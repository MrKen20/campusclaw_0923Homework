# Change: add-traceable-vector-retrieval

## Why

第 3 课（`add-auth-rbac-class-knowledge`，已归档）已实现登录、按班级隔离与材料入库，但材料只能按标题浏览——教师与学生无法用一句自然语言定位"本班材料里讲过什么"，更无法把回答追溯到原文。本变更为同一套系统增加**限定班级范围、可追溯出处**的知识库检索：以一句自然语言在本班材料中定位相关片段并标注其来自哪份材料的哪个切片；语料中不存在依据时明确返回未找到，不得编造出处。

## What Changes

- **切片与入库管线**：上传成功后，正文按切分策略写入新表 `knowledge_chunks`（正文与字符区间留在 MySQL）；每个切片经服务端嵌入网关转为向量，写入 Qdrant 集合 `campusclaw_chunks`（余弦度量）。向量主键与 `knowledge_chunks.id` 一致；payload 仅含 class_id、material_id、knowledge_entry_id、chunk_id、chunk_index，不含正文。嵌入失败时材料记录保留、切片标记 `failed`，不写不完整向量。
- **三种切分策略**：`auto`（默认：最大 800 字、重叠 80 字，优先在空行/换行/句号断开）、`custom`（100–2000 字、重叠 0–50%，可选预处理：移除 URL/邮箱、折叠连续空白）、`hierarchy`（按 Markdown `#`/`##`/`###` 分章，过长章节再按 auto 规则切分）。预处理不改写 `knowledge_entries.body_text`；已入库材料不自动重切，更换策略须显式重建索引。
- **三种检索模式**：`keyword`（仅 MySQL `FULLTEXT WITH PARSER ngram` 全文索引，不调嵌入、不访问 Qdrant）；`vector`（问句经嵌入网关转向量，Qdrant 按余弦相似度检索，低于 0.35 的候选丢弃，回 MySQL 取正文）；`hybrid`（默认；两路分别过滤后按名次做 RRF 融合，`k = 60`，缺席的一路不贡献分数）。
- **检索结果可溯源**：每条命中至少包含材料标题、切片序号、字符区间与一段摘录（摘录取自 MySQL 的 `chunk_text`），并支持打开对应材料详情（沿用第 3 课鉴权规则）。
- **检索侧班级隔离**：班级标识仅从登录会话读取；关键字与向量两条路径都带班级条件，回表查询再次核对；跨班级检索对外表现为 200 + 空 hits，不以 403/404 暗示资料存在。
- **问答接口 `POST /api/ask`**：先以混合模式检索本班前 4 条切片；有命中才调用对话网关生成简短回答，回答中 `[1]`、`[2]` 标注与 `citations` 列表顺序一致；无命中直接返回固定文案「资料中未找到相关内容」，不调用生成模型。客户端注入的 `system` 消息一律丢弃，system 提示由服务端写入。
- **重建索引**：教师可按新策略对既有材料重建索引——先删除旧切片与旧向量记录，再重新切分、嵌入、写入。
- **部署**：Docker Compose 新增 Qdrant 容器（不映射宿主端口，仅内部网络）；嵌入网关与对话网关仅服务端调用，密钥经环境变量注入，不下发浏览器。
- **前端**：材料页增加检索入口与模式选择，结果展示标题/切片序号/摘录并可跳转原文；问答展示回答正文与引用列表。

## Capabilities

### New Capabilities

- `knowledge-retrieval`：切片入库管线、三种切分策略、三种检索模式（keyword / vector / hybrid + RRF）、检索侧班级隔离、问答与引用标注、索引重建、向量库容错与降级等可验收行为。

### Modified Capabilities

（无——`auth-upload` 的上传、鉴权与隔离行为不变；本能力在其上传成功之后追加索引管线，不修改既有 Requirement。）

## Impact

- 新增 MySQL 表 `knowledge_chunks`（含 ngram 全文索引、`index_status`）；既有表结构不变。
- 后端新增模块：切分（chunking）、嵌入网关客户端、Qdrant 客户端、检索（keyword / vector / hybrid + RRF）、问答（ask）。
- Compose 新增 `qdrant` 服务；`.env.example` 新增嵌入/对话网关地址与密钥、Qdrant 地址、向量维度、切分默认参数。
- 前端新增检索与问答界面；`docs/` 追加迭代说明。
- 本 change 以规约四件套为交付物；可运行实现按 `tasks.md` 在 Apply 阶段完成。

## Non-goals（非目标）

- **流式输出与长对话**：问答仅生成简短回答；流式输出、多轮长对话与连续追问记忆安排在第 5 课（历史消息可随请求附带，但不实现流式）。
- **重排序**：不引入交叉编码器（cross-encoder）对候选重新打分——延迟与复杂度不匹配本课目标。
- **编排框架与知识库产品**：不引入 LangChain、LlamaIndex、Dify 等；由 Go 服务自行完成切分、嵌入与检索，便于对照规约逐条验收。
- **正文写入向量库**：Qdrant 只存向量与标识，不存 `chunk_text`，避免双副本；摘录与正文一律回 MySQL 取。
- **按班级拆分 Qdrant 集合**：全部切片共存于 `campusclaw_chunks`，以 payload `class_id` 过滤实现隔离；拆集合的隔离强度收益不抵集合管理成本。
- **客户端直连**：Qdrant 与两类网关不暴露给浏览器；前端只调用本站 `/api` 接口。
- **子切片/父块（small-to-big）检索**、**增量重嵌入**（仅对变更段落重新嵌入）：本课不做，统一走"删旧重建"。
- **检索结果缓存、检索频率限制、检索审计**：留待后续迭代。
