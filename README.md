# CampusClaw

> 价值：面向中小学的教研智能体，以班级为数据边界管理教研材料，知识库是后续智能能力的底座。
> 场景：教师登录后上传、查看、下载本班教研材料；学生登录后查看、下载本班材料；跨班访问在服务端被拒绝。
> 不做：本迭代不做检索问答（RAG）、对话助手、作业流程、平台超级管理员、SSO/多租户、注册改密、PDF/Word 解析、生产高可用；不用前端隐藏按钮替代服务端鉴权。

本仓库为迭代 1「认证授权与知识库入库」交付：可登录、可隔离、可上传入库、第三方可按本 README 从零复现。

## 快速启动（Docker Compose，唯一发布形态）

前置：已安装 Docker Desktop（含 `docker compose`）。

```bash
cp .env.example .env
# 编辑 .env，至少填写：SESSION_SECRET、MYSQL_PASSWORD、SEED_TEACHER_PASSWORD、SEED_STUDENT_PASSWORD
# SESSION_SECRET 可用 openssl rand -hex 32 生成
docker compose up --build -d
docker compose ps
```

- 登录页：<http://localhost:8080>（端口冲突时改 `.env` 的 `WEB_PORT`）
- 健康检查：`curl http://localhost:8080/health` → `{"status":"ok"}`（未登录也可访问；只表示进程存活，不查数据库）
- 首次启动 MySQL 初始化需几十秒，期间入口可能短暂 502，属正常现象（api 会重试等待 db 就绪）

### 预置账号（口令来自 .env 的 SEED_* 变量）

| 账号 | 角色 | 班级 |
| --- | --- | --- |
| `teacher_a` | 教师（可上传） | 班级 A |
| `student_a1` | 学生（只读） | 班级 A |
| `student_b1` | 学生（只读） | 班级 B |

两班各有一条标题可区分的种子材料（"A 班种子讲义…" / "B 班种子讲义…"），用于验证班级隔离。

### 数据持久化

数据库与上传文件保存在命名 volume 中：`docker compose down` 后再 `up`（不加 `-v`）数据仍在；`down -v` 会清空数据卷，仅用于主动重置。

## 关键约定

- **单实例部署**：登录限流状态保存在 api 进程内存中，多副本会状态不一致；多副本方案留待后续迭代。
- **跨班返回 404**：访问他班材料详情/文件与"记录不存在"返回完全相同的 404（不用 403，避免暴露资源存在性）；跨班探测只记录到服务端日志。
- **租户标识只取自会话**：请求参数、请求头、表单中的 `class_id` 一律被忽略；上传的数据归属会话所在班级。
- **失败响应同形**：账号不存在、口令错误、登录限流三种情况返回完全一致的响应，不给账号枚举留依据。
- **数据库不出内网**：只有 web 映射宿主端口；db 不映射 3306，api 不单独暴露；api 使用普通数据库账号连接（root 口令由 db 容器随机生成，不下发）。
- **上传目录不静态暴露**：文件只经 `GET /api/materials/{id}/file` 鉴权下载；直接猜 `/uploads/...` 路径取不到文件。

## 技术栈与架构

浏览器 → Nginx（静态资源 + 同源反代 `/api`、`/health`）→ Go（`net/http`，认证/授权/隔离/上传校验）→ MySQL 8.0

- 前端：React 18 + TypeScript + Vite（`frontend/`），React Markdown + Remark GFM
- 后端：Go + 标准库 `net/http`（`backend/cmd/server` + `backend/internal/{config,db,auth,materials,knowledge}`），bcrypt 口令哈希，服务端会话表 + HttpOnly Cookie（不用 JWT）
- 部署：Docker Compose（web/api/db），环境变量经 `.env` 注入（`.env.example` 入库，`.env` 不入库）

## 接口一览

| 方法与路径 | 鉴权 | 说明 |
| --- | --- | --- |
| `POST /api/login` | 无 | 校验口令、限流、换发会话；三类失败响应同形 |
| `POST /api/logout` | 会话 | 删除服务端会话并清 Cookie |
| `GET /api/me` | 会话 | 返回用户、角色、班级 |
| `GET /api/materials` | 会话 | 本班列表与搜索（`?q=`） |
| `GET /api/materials/{id}` | 会话 + 本班 | 详情与知识库正文；跨班 404 |
| `GET /api/materials/{id}/file` | 会话 + 本班 | 下载原文件；跨班 404 |
| `POST /api/materials` | 会话 + 教师 | 上传 `.txt`/`.md`（multipart，字段名 `file`） |
| `GET /health` | 无 | 存活判定，不查库 |

## 规约

本仓库采用 OpenSpec 规约驱动开发（SDD）：活动变更见 `openspec/changes/`，已归档能力规约见 `openspec/specs/`。无规约不写代码，详见 `AGENTS.md`。
