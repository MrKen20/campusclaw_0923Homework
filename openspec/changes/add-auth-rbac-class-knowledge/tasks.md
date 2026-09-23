# Tasks: add-auth-rbac-class-knowledge

按顺序执行，一次一个 task：实现 → 审查 diff → 运行 verify → 勾选 → 提交。所有复选框初稿保持未勾选，勾选发生在 Apply 阶段且以 verify 实际结果为准。

## 1. 项目骨架与配置

- [ ] 1.1 确认仓根 README 含价值/场景/不做三行说明、单实例声明与跨班 404 约定 — verify: 打开 `README.md` 四项齐全且与 proposal Non-goals 一致
- [ ] 1.2 建立 Go 后端（`backend/cmd/server` + `backend/internal/{config,db,auth,materials,knowledge}`）与前端（`frontend/`，Vite + React 18 + TS）目录骨架，导入与构建不报错 — verify: `cd backend && go build ./...` 与 `cd frontend && npm run build` 均成功
- [ ] 1.3 配置全部来自环境变量（SESSION_SECRET、DB 凭据、MAX_UPLOAD_BYTES、SESSION_TTL_HOURS、LOGIN_FAIL_LIMIT、预置账号口令等），缺失必需项时启动失败 — verify: 删除 `.env` 后执行启动命令，进程报错退出而非照常启动
- [ ] 1.4 添加 `.gitignore` 与 `.dockerignore`，排除 `.env` 与 `uploads/`；提交 `.env.example`（列全部必填项、无真实值）— verify: 克隆到干净目录全局搜密钥无结果；镜像内不含 `.env`

## 2. 数据层与种子

- [ ] 2.1 实现 MySQL 建表：classes、users、sessions、materials、knowledge_entries、作业、助手、技能表；业务表 `class_id` NOT NULL 并建索引 — verify: `docker compose exec db mysql ... -e "SHOW TABLES"` 含全部表名；插入空 class_id 被拒绝
- [ ] 2.2 实现幂等种子：班级 A/B、教师 A、学生 A1/B1；两班各至少一条可区分标题的材料；口令来自环境变量并 bcrypt 入库 — verify: 连续启动两次，用户与材料行数不变；`SELECT username, role FROM users` 含 teacher_a、student_a1、student_b1
- [ ] 2.3 实现数据访问层：连接（含就绪重试）、参数化查询、`list_materials(class_id)` 等强制班级过滤的 helper — verify: 临时脚本对班级 A 调用列表函数返回 ≥1 条且不含 B 班标题

## 3. 登录、会话与限流

- [ ] 3.1 实现 `POST /api/login`：bcrypt 校验、写入服务端会话行、换发会话 ID、Set-Cookie（HttpOnly + SameSite=Lax）— verify: 教师 A 登录后 Cookie 为不透明 ID 且登录前后值不同；`GET /api/me` 返回用户/角色/班级三项
- [ ] 3.2 实现 `POST /api/logout` 与鉴权中间件：未登录 API 返回 401 且响应体无材料数据；登出删会话行 — verify: 清 Cookie 调 `/api/materials` 返回 401 且 body 无标题/正文/路径；登出后用旧 Cookie 再请求返回 401
- [ ] 3.3 实现登录限流与失败同形：账号不存在、口令错误、被限流三类响应的状态码/文案/耗时一致 — verify: 三种输入各试一次，对比状态码、响应体与耗时无明显差异

## 4. 班级隔离

- [ ] 4.1 列表接口仅按会话班级过滤，忽略请求参数中的 class_id — verify: 学生 A1 带 `class_id=B` 参数请求列表，结果与不带时一致且无 B 班标题
- [ ] 4.2 详情与文件接口先按 ID 取行再核对班级；跨班与不存在 ID 返回完全同形 404 — verify: 教师 A 请求 B 班材料 id 与请求不存在 id，对比两者状态码与响应体一致
- [ ] 4.3 上传归属只取会话班级 — verify: 教师 A 表单塞入 `class_id=B` 上传，落库记录的 class_id 仍为 A
- [ ] 4.4 绕过前端验证服务端隔离 — verify: 用 curl 携 Cookie 直接构造跨班请求，仍返回同形 404

## 5. 角色授权与上传入库

- [ ] 5.1 学生调用 `POST /api/materials` 返回 403 且无副作用 — verify: 学生 A1 上传前后，`materials`/`knowledge_entries` 行数与上传目录均不变
- [ ] 5.2 教师上传：扩展名白名单（.txt/.md）、大小上限、服务端生成存储名、解析正文、双表同事务写入 — verify: 教师 A 上传后两表各增 1 行且 class_id 为 A；落盘文件名为服务端生成
- [ ] 5.3 失败路径无残留：非法扩展名、超限文件、非法编码文件各返回明确错误且无脏数据 — verify: 分别上传 `.exe`、超限文件、非法编码 `.md`，两表与上传目录均无新增
- [ ] 5.4 下载走鉴权接口，上传目录不静态暴露 — verify: 本班用户 `GET /api/materials/{id}/file` 可取到文件；直接请求 `/uploads/<猜测文件名>` 取不到

## 6. 材料读取 API

- [ ] 6.1 `GET /api/materials` 列表与本班搜索均来自带班级过滤的数据库查询 — verify: 清空材料表后刷新列表为空；用 B 班标题在 A 班搜索无结果
- [ ] 6.2 `GET /api/materials/{id}` 返回元数据与知识库正文 — verify: 教师上传含已知正文的 `.md` 后，详情接口返回该正文

## 7. 前端页面

- [ ] 7.1 登录页：提交登录、失败统一文案提示、成功跳材料页 — verify: 错误口令与不存在账号的界面提示完全一致
- [ ] 7.2 材料页：本班列表、详情查看（Markdown GFM 渲染且转义）、下载 — verify: 上传含 `<script>` 的 `.md`，页面展示转义后内容且脚本未执行
- [ ] 7.3 上传入口与身份以 `/api/me` 为准；刷新/重开标签页身份正确恢复 — verify: 手工改 localStorage 中 role 后入口不变化；刷新后教师仍见上传入口、学生仍不见
- [ ] 7.4 401 清状态回登录页；403/404 按服务端语义提示 — verify: 分别触发三种响应，界面反馈与语义一致
- [ ] 7.5 本班搜索（在本班已查询数据范围内过滤标题或正文）— verify: 用 B 班材料标题在 A 班搜索无结果
- [ ] 7.6 北大红品牌色与深浅色主题切换 — verify: 切换主题后两模式均可用；主色为北大红
- [ ] 7.7 列表/网格双视图切换 — verify: 两种视图切换后均正确渲染同一批本班材料
- [ ] 7.8 上传进度反馈（XHR 进度）— verify: 上传过程中可见进度变化
- [ ] 7.9 操作结果 toast / 页内提示 — verify: 上传成功、失败及登录失败均有反馈；登录失败文案与服务端同形
- [ ] 7.10 ⌘/Ctrl+K 命令面板（搜索与导航）— verify: 快捷键唤起面板，可搜索本班材料并跳转
- [ ] 7.11 响应式布局（桌面与移动端宽度可用）— verify: 两种宽度下登录页与材料页布局可用

## 8. Docker Compose 与文档

- [ ] 8.1 编写 web/api 的 Dockerfile、`deploy/nginx.conf`（静态托管 + 同源反代 `/api`、`/health`）、`docker-compose.yml`（web/api/db；仅 web 映射端口；api 用普通数据库账号连接，root 口令不下发给 api；上传目录只挂载到 api）— verify: 本机尝试连接 3306 失败；扫本机端口仅 web 端口暴露；`.env` 中无 db root 口令
- [ ] 8.2 `GET /health` 仅存活判定不查库；业务接口在 db 不可用时返回 503 — verify: 停掉 db 后 `/health` 仍 200、`/api/materials` 返回 503 而非 401
- [ ] 8.3 api 启动重试等待 db 就绪；首启自动建表与种子 — verify: 全新环境 `docker compose up --build` 无持续 502，预置账号可登录
- [ ] 8.4 数据卷持久化 — verify: 上传一条材料后 `down` 再 `up`（不加 `-v`），该材料仍在列表中
- [ ] 8.5 完善 README：范围/不做项、单实例、跨班 404、Compose 启动步骤、`.env` 说明 — verify: 请同伴只读 README 复述四点并在干净目录从零启动成功

## 9. 发布验收

- [ ] 9.1 主路径证据：登录、教师上传、列表可见、学生只读各留一份可复核输出（命令+输出或截图）— verify: 证据含完整命令与上下文，同伴可按记录复现
- [ ] 9.2 失败路径证据：未登录 401、学生上传 403、跨班 404 三条各留证据 — verify: 三条证据齐全
- [ ] 9.3 同伴交叉验证：跨班 URL、学生上传、按 README 从零启动 — verify: 记录互验结论；被绕过时区分规约缺陷与实现缺陷并先补规约
- [ ] 9.4 迭代说明：写入三项 ADR（会话与租户来源、跨班 404 及备选、上传事务与清理）+ /health 存活语义，逐条关键 Scenario 给出"通过/不通过 + 证据"的判定结论 — verify: 迭代说明与 design.md 决策一致；同伴读完能复述四项决策
- [ ] 9.5 运行 `openspec validate add-auth-rbac-class-knowledge --strict` — verify: exit 0 且无 error，警告逐条读过
- [ ] 9.6 安全自查：仓库无真实密钥、`.env.example` 已提交、库中口令为 bcrypt 哈希 — verify: 克隆到干净目录全局搜密钥无结果；查 users 表口令字段为 `$2` 前缀
- [ ] 9.7 打版本 tag — verify: `git tag v0.1.0-auth-upload` 后从 tag 检出可启动
