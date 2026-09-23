# 迭代 1 验收记录（add-auth-rbac-class-knowledge）

验收环境：macOS (Apple Silicon) + Docker 29.8.0 / Compose v5.5.1；`cp .env.example .env` → 填密钥 → `docker compose up --build -d`，访问 `http://localhost:8080`。
验收日期：2026-09-23。以下"证据"列为实际执行的命令与关键输出（可复制复现）。

## 一、设计决策（ADR，用自己的话写）

**ADR-1 会话与租户来源（design.md D2/D3）**
上下文：本迭代为同源浏览器站点，班级是租户边界，验收含"登出后旧 Cookie 立即 401"。
决策：登录态用服务端会话表 + HttpOnly/SameSite=Lax Cookie，库中只存会话 ID 的 HMAC；`role` 与 `class_id` 每次请求从用户表连表读取；列表的班级条件、上传的数据归属一律取自会话，query/表单里的 `class_id` 全部忽略。
备选与否决：JWT——登出不能立刻作废、改角色要等 token 过期，且载荷里的 class_id 会诱使实现信任客户端数据；localStorage 存 Bearer token——XSS 下可被脚本读走。均否决。

**ADR-2 跨班返回 404（design.md D3）**
上下文：按 ID 访问单条资源时，"资源存在但越权"与"资源不存在"需要统一对外表现，否则可枚举资源 ID。
决策：对象路径采用"先取行、再核对班级"；越权与不存在返回**完全同形**的 404（同一 JSON 错误体），跨班试探只写服务端日志；全仓不用 403 表达跨班，并写进 README。
备选与否决：403——明确告知资源存在，利于排障也利于枚举扫描；401——用户已认证，问题属于对象级授权而非未登录。均否决。

**ADR-3 上传事务与清理（design.md D6）**
上下文：材料行与知识库行必须同生共死，否则第 4 课检索会遇到"有文件无正文"或孤儿记录。
决策：校验（扩展名白名单 .txt/.md → MaxBytesReader 超限 413 提前拒绝 → 非空 UTF-8 内容校验）尽量在读盘前完成；落盘用服务端生成的随机存储名（客户端文件名仅作标题）；两表写入放在同一事务，任一步失败整体回滚并删除已写文件。
备选与否决：异步队列入库——失败路径难验证；对象存储——多副本场景才需要，本迭代单实例本地目录即可；两表合一——检索迭代需要独立的 knowledge_entries。均否决。

**ADR-4 /health 存活语义（design.md D9）**
上下文：Compose healthcheck 依赖 /health，若混入数据库探活，数据库抖动会被误判为容器故障触发重启。
决策：/health 只做进程存活判定（200 `{"status":"ok"}`），不查库；数据库不可用时业务接口返回 503（已登录用户不会被误判 401 会话失效）。
备选与否决：/health 内一并检查数据库——编排层一次探活省事，但故障会互相放大。否决。

## 二、关键 Scenario 判定结论

| Scenario（spec.md 原文） | 判定 | 证据（命令 → 关键输出） |
| --- | --- | --- |
| R1 未登录访问受保护 API | 通过 | `curl http://localhost:8080/api/materials` → `401 {"error":"未登录"}`，响应体无标题/正文/路径 |
| R1 三类登录失败同形 | 通过 | 错误口令、不存在账号、限流后正确口令三种输入均 → `401 {"error":"用户名或密码错误"}`，文案一致 |
| R1 登录限流生效 | 通过 | 连续 5 次失败后，第 6 次用**正确口令**登录仍 `401` 同文案 |
| R1 登录成功换发会话 ID | 通过 | 两次登录的 `Set-Cookie: campusclaw_session=` 值不同；属性含 `HttpOnly; SameSite=Lax` |
| R1 登出后旧 Cookie 失效 | 通过 | logout 200 后，携旧 Cookie 请求 `/api/me` → `401` |
| R2 学生上传被拒绝且无副作用 | 通过 | 学生 Cookie POST /api/materials → `403`；查库 `materials=3, knowledge_entries=3` 与上传前一致；uploads 目录仍只有 1 个文件 |
| R3 跨班按 ID 访问同形 404 | 通过 | 教师 A 请求 B 班 id=2 与不存在 id=99999：均 `404 {"error":"材料不存在"}`，响应体逐字一致；`/2/file` 同样 404 |
| R3 列表班级条件只取自会话 | 通过 | `GET /api/materials?class_id=2`（教师 A）与不带参数结果完全一致，仅含 A 班标题 |
| R3 上传归属会话班级 | 通过 | 查库 `SELECT class_id FROM materials WHERE id=3` → `1`（A 班），`file_path` 为服务端生成的随机名 `3f731ba5….md` |
| R4 上传成功两表同事务入库 | 通过 | 教师上传 demo.md → `201 {"id":3}`；查库 materials 与 knowledge_entries 各 +1 且互相关联、class_id=1 |
| R4 扩展名/大小/内容三类失败 | 通过 | `.exe` → 400「仅支持 .txt / .md」；2MB 超限 → 413；非法编码 → 400；三例后两表行数不变、uploads 无新文件 |
| R5 上传目录不可直接访问 | 通过 | `curl http://localhost:8080/uploads/anything.md` → 200 但返回的是 SPA 的 index.html（text/html），非文件内容 |
| R5 详情不含磁盘路径 | 通过 | `/api/materials/3` 响应只含 id/title/class_name/created_at/body，无 file_path |
| R6 种子口令哈希入库 | 通过 | 查库 `LEFT(password_hash,7)` → `$2a$10$`，三账号均无明文 |
| R6 种子幂等 | 通过 | `docker compose restart api` 后查库 users=3 / materials=3 / knowledge=3，无重复、未覆盖上传内容 |
| R6 class_id 非空约束 | 通过 | `INSERT INTO materials (class_id=NULL,…)` → `Column 'class_id' cannot be null` |
| R7 缺环境变量启动失败 | 通过 | `env -i ./campusclaw-api` → `配置错误：缺少必需环境变量 SESSION_SECRET` 退出码 1；仓库 `git grep` 无密钥 |
| R8 按 README 从零启动 | 通过 | 干净 volume 上 `docker compose up --build -d` → 登录页 200，预置账号可登录；api 日志显示就绪重试 4 次后监听 |
| R8 仅 web 映射端口 | 通过 | `nc -z localhost 3306` 连不上；`docker compose port web 80` → `0.0.0.0:8080`，唯一映射 |
| R8 /health 不查库 | 通过 | 停 db 后 `/health` 仍 200；`/api/materials`（携有效 Cookie）→ 503 而非 401 |
| R8 down 再 up 数据仍在 | 通过 | `docker compose down && up -d`（不加 -v）→ 重新登录成功，列表仍含 demo.md 与 A 班种子讲义 |
| R9 刷新后身份恢复 | 通过 | 浏览器刷新/重开由 `/api/me` 恢复身份（见截图 student.png / teacher-light.png） |
| R9 Markdown 渲染转义 | 通过 | 上传含 `<script>alert(1)</script>` 的 .md，详情页该标签以纯文本呈现（见 detail-gfm.png），无弹窗、页面正常响应 |
| R10 上传入口随角色 | 通过 | 教师见"上传材料"按钮（teacher-light.png）；学生界面无该按钮（student.png），且服务端仍 403 |
| R10 主题/视图/命令面板/进度/toast | 通过 | 深色截图 teacher-dark.png；网格视图按钮可切换；⌘K 按钮打开命令面板（palette-open2.png）；登录成功 toast「欢迎，teacher_a（班级 A）」；XHR 上传进度条代码在位 |

截图证据：`docs/evidence/`（teacher-light / teacher-dark / detail-gfm / student / palette-open2）。

## 三、遗留项

- **9.3 同伴交叉验证**：需同伴在本人环境实测一条跨班 URL 与学生上传路径——待课后互验后勾选。
- 归档（Archive）：待 9.3 完成后执行 `openspec archive`，将 delta 并入主力规约。
