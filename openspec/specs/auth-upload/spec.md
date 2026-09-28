# auth-upload Specification

## Purpose
让教师与学生以各自角色登录 CampusClaw，以班级作为数据边界隔离教研材料；教师上传的材料解析后写入知识库，本班成员经鉴权 API 查看与下载；未登录、越权、跨班访问在服务端被拒绝，且对外响应不泄露任何可用于探测的信息。

## Requirements

### Requirement: 用户登录与会话

系统 MUST 提供教师（teacher）、学生（student）两类角色的账号密码登录；登录成功 MUST 建立服务端会话并通过 Cookie 下发不透明会话 ID；登出 MUST 立即使会话失效。

#### Scenario: 登录成功建立服务端会话

- **WHEN** 用户使用有效账号与密码发起登录（预置账号含教师 A、学生 A1、学生 B1）
- **THEN** 登录 MUST 成功并在服务端建立会话行
- **AND** 响应 MUST 通过 `Set-Cookie` 下发 `HttpOnly`、`SameSite=Lax` 的会话 Cookie，Cookie 值 MUST 为不透明随机 ID（不含角色、班级等可读信息）
- **AND** `GET /api/me` 携带该 Cookie 时 MUST 返回用户标识、角色与所属班级三项

#### Scenario: 三类登录失败响应同形

- **WHEN** 用户分别以「账号不存在」「口令错误」「已被登录限流」三种情形发起登录
- **THEN** 三种响应的状态码、响应体文案 MUST 完全一致
- **AND** 响应 MUST NOT 暗示账号是否存在、是否被锁定
- **AND** 三种情形的响应耗时 MUST 相近（对不存在账号也执行等价耗时的哈希比较）

#### Scenario: 登录限流生效

- **WHEN** 同一账号或来源在短时间内连续多次登录失败
- **THEN** 后续登录尝试 MUST 被限流
- **AND** 限流期间的对外响应 MUST 与「口令错误」同形（不返回 429 或差异化文案）

#### Scenario: 登录成功换发会话 ID

- **WHEN** 用户登录成功
- **THEN** 系统 MUST 换发新的会话 ID
- **AND** 登录前持有的匿名/旧会话 MUST 作废

#### Scenario: 未登录访问受保护 API

- **WHEN** 未携带有效会话的请求访问受保护 API（如 `GET /api/materials`）
- **THEN** 系统 MUST 返回 HTTP 401
- **AND** 响应体 MUST NOT 包含任何本班或他班材料的标题、正文、文件路径或存储键

#### Scenario: 前端收到 401 回登录页

- **WHEN** 前端任一 API 请求收到 401
- **THEN** 前端 MUST 清除本地身份状态并跳转登录页

#### Scenario: 登出后旧 Cookie 立即失效

- **WHEN** 已登录用户调用 `POST /api/logout`
- **THEN** 服务端 MUST 删除对应会话行并清除 Cookie
- **AND** 此后携带旧 Cookie 请求受保护 API MUST 返回 401

### Requirement: 角色权限

系统 MUST 按服务端会话中的角色授权：教师可上传、查看、下载本班材料；学生只能查看、下载；角色 MUST 每次请求从服务端读取，MUST NOT 采信客户端声明。

#### Scenario: 学生上传被拒绝且无副作用

- **WHEN** 以学生（如学生 A1）有效会话向 `POST /api/materials` 提交文件
- **THEN** 系统 MUST 返回 HTTP 403
- **AND** `materials` 与 `knowledge_entries` 表行数 MUST 与请求前相同
- **AND** 上传目录 MUST 无因该请求产生的新文件

#### Scenario: 教师上传被允许

- **WHEN** 以教师（如教师 A）有效会话向 `POST /api/materials` 提交合法文件
- **THEN** 系统 MUST 接受请求并完成入库（见「材料上传与知识库入库」）
- **AND** MUST 返回成功（如 HTTP 201）并含新材料标识

#### Scenario: 上传入口依据 /api/me 显示

- **WHEN** 前端渲染材料页
- **THEN** 上传入口 MUST 依据 `GET /api/me` 返回的角色决定是否显示
- **AND** MUST NOT 依据 localStorage 等客户端存储中的角色

#### Scenario: 客户端角色声明无效

- **WHEN** 请求在请求体或请求头中携带与会话不一致的 `role` 字段
- **THEN** 系统 MUST 忽略该字段
- **AND** 授权结果 MUST 与不带该字段时一致

#### Scenario: 学生只读本班材料

- **WHEN** 学生 A1 登录后访问本班材料列表与详情
- **THEN** 系统 MUST 正常返回本班可读材料
- **AND** 学生对本班材料的一切写操作（上传、修改、删除）在服务端 MUST 被拒绝

### Requirement: 班级隔离

班级是数据边界。所有材料、知识库查询 MUST 在服务端按会话班级过滤；跨班按 ID 访问 MUST 返回与「记录不存在」完全同形的 404；前端隐藏入口 MUST NOT 作为满足本要求的手段。

#### Scenario: 跨班按 ID 访问返回同形 404

- **WHEN** 班级 A 用户（教师 A 或学生 A1）通过路径参数访问 B 班某材料的详情（`GET /api/materials/{id}`）或文件（`GET /api/materials/{id}/file`）
- **THEN** 系统 MUST 返回 HTTP 404
- **AND** 该响应 MUST 与访问「不存在的 ID」时的响应在状态码与响应体上完全一致
- **AND** 响应 MUST NOT 包含 B 班材料的标题、正文片段、文件路径或存储键

#### Scenario: 列表班级条件只取自会话

- **WHEN** 已登录用户请求 `GET /api/materials` 并在查询参数中携带其他班级的 `class_id`
- **THEN** 系统 MUST 忽略请求中的班级参数
- **AND** 返回结果 MUST 与不带该参数时完全一致

#### Scenario: 列表数据来自按班级过滤的数据库查询

- **WHEN** 教师 A 登录后请求材料列表（含合法搜索关键词）
- **THEN** 返回集合中每条记录 MUST 归属班级 A
- **AND** 数据 MUST 来自带班级过滤条件的数据库查询，MUST NOT 为硬编码演示条目
- **AND** 清空材料表后列表 MUST 为空

#### Scenario: 上传归属会话班级

- **WHEN** 教师 A 上传文件时在表单中携带其他班级字段（如 `class_id=B`）
- **THEN** 新记录 MUST 归属会话所在班级 A
- **AND** 表单中的班级字段 MUST NOT 改变数据归属

#### Scenario: 绕过前端仍被服务端拒绝

- **WHEN** 使用 curl 等工具携带有效 Cookie 直接构造跨班或越权请求（不经前端页面）
- **THEN** 服务端 MUST 按上述各 Scenario 拒绝
- **AND** 拒绝行为 MUST NOT 依赖前端路由守卫或入口隐藏

### Requirement: 材料上传与知识库入库

教师上传材料后，系统 MUST 校验、解析正文，并在同一事务写入 `materials` 与 `knowledge_entries`；任何失败 MUST 使数据库与磁盘均无残留。

#### Scenario: 上传成功两表同事务入库

- **WHEN** 教师 A 提交合法的 `.txt` 或 `.md` 文件且解析成功
- **THEN** `materials` 与 `knowledge_entries` MUST 在同一事务各新增一条关联记录
- **AND** 两条记录的 `class_id` MUST 为班级 A
- **AND** 知识库记录 MUST 含解析出的正文文本

#### Scenario: 上传后本班列表与详情可查

- **WHEN** 教师 A 上传成功后，教师 A 与学生 A1 分别刷新本班列表
- **THEN** 两人 MUST 都能在列表中看到新记录标题
- **AND** 详情接口 MUST 返回该材料的正文
- **AND** 学生 A1 对该材料 MUST 仍为只读

#### Scenario: 扩展名白名单校验

- **WHEN** 教师上传 `.exe`、`.md.exe` 等白名单（`.txt`/`.md`）之外的文件
- **THEN** 系统 MUST 返回明确错误（如 HTTP 400）
- **AND** 数据库与上传目录 MUST 无任何新增

#### Scenario: 大小上限校验

- **WHEN** 教师上传超过大小上限（`MAX_UPLOAD_BYTES`，值在 `.env.example` 与 README 中明确）的文件
- **THEN** 系统 MUST 返回 HTTP 413
- **AND** MUST 在读取完整请求体之前拒绝
- **AND** 数据库与上传目录 MUST 无任何新增

#### Scenario: 解析失败无残留

- **WHEN** 文件扩展名合法但内容无法解析（如非法编码、空文件）
- **THEN** 系统 MUST 返回明确错误（如 HTTP 400）
- **AND** `materials` 与 `knowledge_entries` MUST 无不完整记录
- **AND** 已写入磁盘的临时文件 MUST 被删除

#### Scenario: 入库失败整体回滚

- **WHEN** 双表写入过程中任一语句失败
- **THEN** 事务 MUST 整体回滚
- **AND** 两表 MUST 均无新增行，磁盘 MUST 无孤儿文件

### Requirement: 材料详情与文件下载

材料详情与原始文件 MUST 经鉴权 API 提供；上传目录 MUST NOT 作为静态资源暴露；存储文件名 MUST 由服务端生成。

#### Scenario: 详情经鉴权返回正文

- **WHEN** 本班用户请求 `GET /api/materials/{id}`
- **THEN** 系统 MUST 校验会话与班级后返回标题、班级、时间与知识库正文
- **AND** 响应 MUST NOT 包含磁盘绝对路径或存储键

#### Scenario: 文件经鉴权下载

- **WHEN** 本班用户请求 `GET /api/materials/{id}/file`
- **THEN** 系统 MUST 校验会话与班级后返回原始文件
- **AND** 跨班或不存在时 MUST 返回同形 404

#### Scenario: 上传目录不可直接猜测访问

- **WHEN** 直接请求 `/uploads/<猜测文件名>` 或等价的静态路径
- **THEN** 系统 MUST NOT 返回文件内容

#### Scenario: 存储名由服务端生成

- **WHEN** 上传文件的原始文件名含路径穿越字符（如 `../`）或特殊字符
- **THEN** 实际落盘路径 MUST 使用服务端生成的文件名
- **AND** 原始文件名 MUST 仅作为展示标题保存，不参与路径构造

### Requirement: 预置核心数据

系统 MUST 建立班级、用户、会话、材料（讲义）、知识库条目、作业、助手、技能等核心数据结构，并预置可验收的幂等种子数据。

#### Scenario: 种子口令来自环境变量且哈希入库

- **WHEN** 种子脚本执行
- **THEN** 预置账号口令 MUST 来自环境变量并以 bcrypt 哈希写入
- **AND** 库中 MUST NOT 出现明文口令

#### Scenario: 两班材料标题可区分

- **WHEN** 种子执行完成
- **THEN** MUST 存在至少一条明确归属 A 班的材料标题与至少一条明确归属 B 班的材料标题，且两班标题可区分
- **AND** 用 B 班标题在 A 班列表搜索 MUST 无结果

#### Scenario: 种子幂等

- **WHEN** 连续两次执行初始化/启动（含 Compose 重启）
- **THEN** 用户、班级、种子材料 MUST 不被重复插入
- **AND** 已上传的材料 MUST 不被覆盖或清除

#### Scenario: 班级字段非空并索引

- **WHEN** 查看建表语句
- **THEN** 业务表的 `class_id` MUST 为 NOT NULL 并建有索引
- **AND** 尝试插入空 `class_id` 记录 MUST 被拒绝

#### Scenario: 双班三账号可登录

- **WHEN** 首次初始化完成
- **THEN** 库中 MUST 存在班级 A、班级 B
- **AND** MUST 存在教师 A（A 班）、学生 A1（A 班）、学生 B1（B 班）三个可登录账号
- **AND** 作业、助手、技能表 MUST 已创建（可含占位行，不验收业务功能）

### Requirement: 密码哈希与会话密钥

口令 MUST 以 bcrypt 加盐慢哈希存储；会话密钥与数据库凭据 MUST 仅来自环境变量，缺失 MUST 启动失败；密钥 MUST NOT 进入仓库或镜像。

#### Scenario: 库中口令为哈希形态

- **WHEN** 直接查询 `users` 表口令字段
- **THEN** 字段值 MUST 为 bcrypt 哈希形态（如 `$2` 前缀）
- **AND** MUST NOT 等于任何已知预置账号的明文口令

#### Scenario: 登录使用哈希校验

- **WHEN** 用户使用正确明文口令登录
- **THEN** 系统 MUST 通过哈希校验并建立会话
- **WHEN** 同一用户使用错误口令
- **THEN** 校验 MUST 失败且不建立会话

#### Scenario: 缺失环境变量启动失败

- **WHEN** 未提供 `SESSION_SECRET` 或数据库凭据等必需环境变量时启动应用
- **THEN** 应用 MUST 启动失败并给出明确错误
- **AND** MUST NOT 回落到任何内置默认值

#### Scenario: 密钥不入库不进镜像

- **WHEN** 检查仓库历史与构建产物
- **THEN** 仓库 MUST NOT 含真实密钥；`.env` MUST 被 `.gitignore` 与 `.dockerignore` 排除
- **AND** `.env.example` MUST 列出全部必填项且不含真实值
- **AND** 若密钥曾进入历史，MUST 轮换而非仅删除提交

### Requirement: Docker Compose 部署与健康检查

Docker Compose MUST 为唯一标准启动方式；第三方按 README 从零可复现；`GET /health` MUST 只做存活判定；数据库 MUST NOT 映射宿主端口。

#### Scenario: 按 README 从零启动可访问

- **WHEN** 第三方在干净环境按 README 执行 `cp .env.example .env`、填写必填项并 `docker compose up --build`
- **THEN** 浏览器 MUST 可访问登录页
- **AND** `GET /health` MUST 返回 200 且 body 表明存活（如 `{"status":"ok"}`）

#### Scenario: 仅 web 映射宿主端口

- **WHEN** 检查 Compose 配置与本机监听端口
- **THEN** MUST 只有 web 服务映射宿主端口
- **AND** db MUST NOT 映射 3306，api MUST NOT 单独暴露

#### Scenario: /health 不查数据库

- **WHEN** 停掉 db 容器后请求 `GET /health`
- **THEN** `/health` MUST 仍按进程存活返回
- **AND** 业务接口（如 `/api/materials`）MUST 返回 503
- **AND** 已登录用户 MUST NOT 被误判为 401 会话失效

#### Scenario: down 再 up 数据仍在

- **WHEN** 上传材料后执行 `docker compose down` 再 `docker compose up`（不加 `-v`）
- **THEN** 预置账号 MUST 仍可登录
- **AND** 已上传材料与知识库记录 MUST 仍可查询

#### Scenario: README 写明关键约定

- **WHEN** 阅读 README
- **THEN** README MUST 写明范围与不做项、单实例声明、跨班返回 404 的约定、Compose 启动步骤
- **AND** 仅读 README 的第三方 MUST 能复述这四点

#### Scenario: 数据库就绪重试

- **WHEN** 在全新环境执行 `docker compose up --build`（db 尚未就绪时 api 已启动）
- **THEN** api MUST 重试等待数据库可连接
- **AND** 首次启动 MUST NOT 因启动顺序出现持续 502

### Requirement: 最小 Web 界面

系统 MUST 提供登录页与材料页两个页面；身份以 `/api/me` 为唯一依据；Markdown 渲染 MUST 转义防脚本。

#### Scenario: 登录页与材料页可达

- **WHEN** 用户访问应用
- **THEN** 未登录 MUST 落在登录页；登录成功 MUST 进入材料页
- **AND** 材料页 MUST 展示本班材料列表

#### Scenario: 刷新后以 /api/me 恢复身份

- **WHEN** 已登录用户刷新页面或重开标签页
- **THEN** 前端 MUST 通过 `GET /api/me` 重新确认身份
- **AND** 上传入口、班级信息 MUST 以该响应为准

#### Scenario: 上传入口随角色变化

- **WHEN** 学生与教师分别登录查看材料页
- **THEN** 教师 MUST 看到上传入口
- **AND** 学生 MUST NOT 看到上传入口（但服务端拒绝不依赖此隐藏）

#### Scenario: 材料正文安全渲染

- **WHEN** 材料 Markdown 正文含 `<script>` 等注入内容
- **THEN** 前端 MUST 按渲染库默认转义策略展示
- **AND** 材料中的脚本 MUST NOT 被执行

#### Scenario: 本班搜索范围受限

- **WHEN** 用户在材料页使用搜索
- **THEN** 搜索 MUST 仅在本班材料范围内过滤标题或正文
- **AND** 用 B 班材料标题在 A 班搜索 MUST 无结果

### Requirement: 界面体验

系统 MUST 提供与 R9 行为一致的界面体验：北大红品牌、深浅色主题、响应式布局、列表/网格双视图、本班搜索、命令面板、上传进度、toast 反馈与 Markdown GFM 渲染。体验项 MUST NOT 替代或改变任何服务端鉴权语义（401/403/404）。

#### Scenario: 品牌与主题

- **WHEN** 用户打开应用任一页面
- **THEN** 界面 MUST 采用北大红品牌色
- **AND** MUST 支持深色与浅色模式切换，且在桌面与移动端宽度下布局可用

#### Scenario: 列表视图与操作反馈

- **WHEN** 用户在材料页浏览与操作
- **THEN** 材料列表 MUST 支持列表/网格双视图切换
- **AND** 上传 MUST 有进度反馈（如 XHR 进度条）
- **AND** 操作结果 MUST 以 toast 或页内提示反馈
- **AND** 登录失败提示 MUST 与服务端同形文案一致（不区分账号不存在与口令错误）

#### Scenario: 命令面板与 Markdown 渲染

- **WHEN** 用户使用快捷键（⌘/Ctrl+K）与查看材料详情
- **THEN** MUST 提供命令面板承载搜索与导航
- **AND** 材料正文 MUST 以 GFM（GitHub Flavored Markdown）格式渲染
