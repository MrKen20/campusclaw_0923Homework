# Design: add-auth-rbac-class-knowledge

## Context

仓库根已有 README 三行说明、AGENTS.md 与 OpenSpec 初始化产物，尚无业务应用代码。动机见 `proposal.md`；可验收行为见 `specs/auth-upload/spec.md`。Apply 阶段在本仓库实现 CampusClaw 最小可运行栈：浏览器 → Nginx → Go → MySQL。

## Goals / Non-Goals

**Goals:**

- 登录可用；会话存服务端，客户端只持不透明会话 ID；`role` 与 `class_id` 每请求从服务端读取。
- 权限在服务端可判定：学生上传 403；未登录 401；登出后旧 Cookie 立即失效。
- 班级隔离不可绕过：列表强制班级过滤；按 ID 访问先取行再核对班级，跨班返回与"不存在"同形的 404。
- 教师上传链路：落盘 → 校验 → 解析 → 双表同事务入库 → 本班列表查库可见；失败无任何残留。
- `docker compose up --build` 一键启动（仅 web 映射端口）；`GET /health` 仅存活判定；密钥全部来自环境变量，缺失即启动失败。

**Non-Goals:** 见 `proposal.md` Non-goals（检索问答、对话、作业、超管、SSO、JWT、多副本等）。

## 技术栈

| 层次 | 选择 | 说明 |
| --- | --- | --- |
| 前端 | React 18 + TypeScript + Vite | 主流组合；React Markdown + Remark GFM 渲染材料；原生 CSS |
| 后端 | Go + 标准库 `net/http` | 不引入 Web 框架，把需要评估的变量降到最低 |
| 数据库 | MySQL 8.0 | 用户、班级、会话、材料、知识库正文；为后续检索与多连接做准备 |
| 入口 | Nginx | 托管前端静态资源，同源反向代理 `/api` 与 `/health` |
| 部署 | Docker Compose | web + api + db 三容器；唯一发布形态 |
| 口令 | bcrypt（golang.org/x/crypto） | 加盐慢哈希；不存明文、不用可逆加密 |
| 配置 | 环境变量 / `.env` | 数据库凭据、SESSION_SECRET、端口、预置账号口令 |

**备选与否决（用自己的话记录）：**
- *Flask + SQLite*：本地轻量，但 SQLite 文件锁与 Compose 环境等价性弱，不利于后续检索迭代；否决，选 Go + MySQL。
- *JWT 无状态登录*：登出不能立刻作废、改角色要等过期，且载荷中的 class_id 诱使实现信任客户端数据；否决，选服务端会话。
- *服务端渲染单体*：页面与接口一体更简单，但当前需求明确前后端分离；否决。

## Decisions

### D1 请求链路与信任边界

- 链路唯一：浏览器 → Nginx → Go → MySQL。前端不直连数据库，任何数据访问都必须经过 Go。
- Nginx 只托管前端静态资源并将 `/api`、`/health` 反向代理到 api；**不得**将上传目录作为静态资源暴露。
- 前端通过同源 `/api` 调用后端，规避跨域 Cookie 作用域与 SameSite 问题；开发时 Vite 代理路径与生产 Nginx 保持一致。
- 认证、授权、隔离、上传校验全部发生在 Go（信任边界）；浏览器与静态页一律不可信。

### D2 会话模型

- 服务端会话表（`sessions`）：登录成功生成高熵随机会话 ID，写入会话行；客户端 Cookie 只持不透明 ID。
- Cookie 属性：`HttpOnly`、`SameSite=Lax`；本机 HTTP 环境不加 `Secure`。
- 登录成功 MUST 换发新会话 ID 并作废登录前会话（防会话固定）。
- 会话只存 `user_id`；`role` 与 `class_id` **每请求**从用户表连表读取，不采信请求中任何角色/班级声明。
- 登出删除服务端会话行，旧 Cookie 立即 401。
- 备选：JWT —— 否决（登出语义与改角色生效时间不可判定，见 Non-goals）。

### D3 班级隔离

- 班级是租户边界：单库内以 `class_id` 划分；用户归属固定班级，不得通过请求参数切换。
- **列表**：查询强制 `WHERE class_id = ?`，班级只取自服务端会话；请求参数中的 `class_id` 一律忽略。
- **按 ID 访问**（详情/文件）：先按 ID 取行，再核对行的 `class_id` 与会话班级；越权与"ID 不存在"对外返回**完全同形的 404**（响应体不含标题、正文、路径、存储键），试探原因只进服务端日志。全仓统一用 404，不写 403，并写进 README。
- **写入**：新记录的 `class_id` 只取自会话；表单中的班级字段不改变数据归属。
- 备选：跨班返回 403 —— 否决：403 会区分"存在但越权"与"不存在"，成为探测依据。
- 备选：每班独立库 —— 否决：运维成本高，单库 + class_id 过滤即可满足验收。

### D4 启动与数据库就绪

- api 启动时对数据库连接做**重试等待**（指数退避，有上限），不依赖 Compose `depends_on` 的启动顺序，避免全新启动首轮 502。
- 数据库不可用时业务接口返回 503，不得把已登录用户误判为会话失效（401）。

### D5 口令与密钥

- 口令以 bcrypt 哈希入库（`password_hash`），登录用恒定时间比较；错误密码与"账号不存在"走**等价耗时的哈希比较**。
- 种子账号口令来自环境变量（如 `SEED_TEACHER_PASSWORD`），经 bcrypt 后入库；源码与仓库无任何明文口令。
- `SESSION_SECRET`、数据库凭据等只从环境变量读取；**缺失即启动失败**，禁止内置默认值。
- `.env` 不入库、不进镜像（`.gitignore` + `.dockerignore`）；`.env.example` 列出全部必填项且不含真实值，必须入库。

### D6 上传与入库数据流

```
POST /api/materials (teacher, multipart)
  ├─ 会话校验 + role == teacher        else 401/403（无任何写入）
  ├─ 大小上限校验（MaxBytesReader       else 413，在读完整请求体之前拒绝
  │    MAX_UPLOAD_BYTES 提前截断）
  ├─ 扩展名白名单校验（.txt/.md）       else 400
  ├─ 落盘 uploads/<服务端生成名>         # 客户端文件名只作展示标题，不参与路径构造
  ├─ 解析正文（非空 UTF-8 文本）        失败 → 删文件，返回 400，无 DB 行
  ├─ BEGIN; INSERT materials; INSERT knowledge_entries; COMMIT
  │     任一步失败 → ROLLBACK + 删除已写文件（两表与磁盘均无残留）
  └─ 201 { "id": ... }
```

- 扩展名用**白名单**（黑名单无法穷举）；存储名由服务端生成（如 uuid + 规范扩展名）。
- 大小上限在读完整请求体之前拒绝（`http.MaxBytesReader`），超限返回 413。
- 内容必须是非空 UTF-8 文本，否则 400 且无残留。
- `materials` 与 `knowledge_entries` **同一事务**写入；标题默认取文件名或表单字段。

### D7 材料读取

- 详情 `GET /api/materials/{id}`：会话 + 班级核对，返回标题、班级、时间与知识库正文；**不返回磁盘绝对路径**；跨班 404（同 D3）。
- 文件 `GET /api/materials/{id}/file`：同样鉴权后由 Go 读盘回传；**不**经 Nginx 静态暴露 `/uploads`。
- 列表 `GET /api/materials`：按会话班级过滤的数据库查询，支持本班标题/正文关键词筛选；禁止硬编码演示条目。

### D8 限流与单实例声明

- 登录失败限流（按账号 + IP）在 api 进程内存中实现；会话亦在服务端存储。因此**本迭代明确单实例**：多副本下限流与会话状态不一致，多副本方案留待后续部署迭代。
- 限流触发后的对外响应与"口令错误"**完全同形**（同状态码、同文案、相近耗时），不给账号枚举留依据；真实原因只进服务端日志。备选：限流返回 429 —— 否决：暴露账号锁定状态。

### D9 /health 与探针分离

- `GET /health` 只做进程存活判定（200 `{"status":"ok"}`），**不查数据库**——数据库抖动不应触发编排层重启容器。
- 数据库探活属"就绪"语义：数据库不可用时业务接口返回 503（见 D4）。

## 接口一览

| 方法与路径 | 鉴权 | 说明 |
| --- | --- | --- |
| `POST /api/login` | 无 | 校验口令、限流、换发会话；三类失败响应同形 |
| `POST /api/logout` | 会话 | 删除服务端会话并清 Cookie |
| `GET /api/me` | 会话 | 返回用户标识、角色、班级；前端据此决定是否显示上传入口 |
| `GET /api/materials` | 会话 | 按会话班级过滤的列表与本班搜索 |
| `GET /api/materials/{id}` | 会话 + 班级 | 详情与知识库正文；跨班 404 |
| `GET /api/materials/{id}/file` | 会话 + 班级 | 下载原文件；跨班 404 |
| `POST /api/materials` | 会话 + 教师 | 上传入库（multipart） |
| `GET /health` | 无 | 存活判定，不查库 |

## 目录与模块

```
backend/
  cmd/server/            # main：加载配置（缺失即退出）、连接 db（就绪重试）、建表种子、注册路由
  internal/config/       # 环境变量读取与校验
  internal/db/           # 连接、迁移、种子、带 class_id 的查询 helper
  internal/auth/         # 登录/登出/会话中间件/限流
  internal/materials/    # 列表、详情、文件、上传
  internal/knowledge/    # 解析正文
frontend/                # Vite + React 18 + TypeScript：登录页、材料页
deploy/nginx.conf        # 静态托管 + 同源反代 /api、/health
```

Compose 三服务：`web`（Nginx + frontend 构建产物）、`api`（backend）、`db`（MySQL 8.0）。

## 数据模型（要点）

- `classes(id, name)`；`users(id, username, password_hash, role, class_id NOT NULL, ...)`；`sessions(id, user_id, expires_at, ...)`。
- `materials(id, class_id NOT NULL, title, file_path, uploaded_by, created_at, ...)`；`knowledge_entries(id, material_id, class_id NOT NULL, body_text, ...)`。
- 六类核心结构：班级、用户、讲义（materials）、作业、助手、技能表均建表；作业/助手/技能可各含 0~1 条占位行，不验收业务功能。
- 所有业务表 `class_id` 非空并建索引；种子幂等（先查后插），重复启动不复制数据、不覆盖已上传内容。

## 配置与启动

- `.env.example` 必填项（均不含真实值）：`SESSION_SECRET`、`MYSQL_DATABASE`、`MYSQL_USER`、`MYSQL_PASSWORD`、`SEED_TEACHER_PASSWORD`、`SEED_STUDENT_PASSWORD`、`MAX_UPLOAD_BYTES`、`SESSION_TTL_HOURS`、`LOGIN_FAIL_LIMIT`、`LOGIN_FAIL_WINDOW_SECONDS`、`WEB_PORT`（默认 8080）。
- **api 使用普通数据库账号连接 MySQL**；root 管理员口令仅用于 db 容器自身初始化，不写进 `.env` 下发给 api。
- Compose：web（Nginx，唯一映射宿主端口）、api（Go，不单独暴露）、db（MySQL 8.0，不映射 3306，仅内部网络）。
- 数据持久化用命名 volume；`docker compose down` 再 `up`（不加 `-v`）数据仍在；`-v` 仅用于主动重置。
- README 步骤：`cp .env.example .env` → 填密钥 → `docker compose up --build -d` → 打开 `http://localhost:8080`。

## Risks / Trade-offs

- 会话/限流在进程内存 → 明确声明单实例（D8），写进 README 与 Non-goals。
- 跨班 404 同形 → 排障依赖服务端日志，已在 D3 约定。
- 首版仅解析 `.txt`/`.md` → PDF 等格式后置；失败场景 spec 已要求无脏数据。

## Migration Plan

从零起步：api 首次启动自动建表并执行幂等种子，无旧数据迁移。实现顺序见 `tasks.md`（骨架 → 数据 → 会话 → 隔离 → 上传 → 前端 → Compose → 验收）。

## Open Questions

（无。）
