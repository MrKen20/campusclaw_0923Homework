# Design: add-traceable-vector-retrieval

## Context

`add-auth-rbac-class-knowledge` 已归档：登录、角色授权、班级隔离、上传入库与最小 Web 界面均已在主力规约 `openspec/specs/auth-upload/` 中。动机见 `proposal.md`；可验收行为见 `specs/knowledge-retrieval/spec.md`。本变更在既有链路（浏览器 → Nginx → Go → MySQL）之上追加检索能力，客户端仍只访问本站 Go 接口。

## Goals / Non-Goals

**Goals:**

- 一句自然语言定位本班材料相关片段，每条结果可回溯至材料标题、切片序号、字符区间与摘录。
- 三种检索模式（keyword / vector / hybrid）语义正确：keyword 判词是否出现，vector 判语义是否接近，hybrid 按名次 RRF 融合。
- 检索与问答的班级边界不可绕过：班级只取会话，双侧过滤 + 回表核对。
- 无依据不生成：无命中切片时不调用对话网关，返回固定文案。
- Qdrant 故障时关键字检索仍可用（降级不中断全部能力）。

**Non-Goals:** 见 `proposal.md` Non-goals（流式长对话、重排序、编排框架、正文入向量库、拆集合、客户端直连、small-to-big、增量重嵌入等）。

## 技术栈增量

| 环节 | 选择 | 说明 |
| --- | --- | --- |
| 切分 | 自研（Go） | auto / custom / hierarchy 三策略；不引入框架 |
| 嵌入 | 课程嵌入网关（OpenAI 兼容 `/embeddings`） | 仅服务端调用；向量维度与 Qdrant collection 一致 |
| 向量库 | Qdrant（Compose 容器） | 集合 `campusclaw_chunks`，余弦度量 |
| 关键字 | MySQL `FULLTEXT ... WITH PARSER ngram` | 中文必须 ngram（token 长度 2），否则无法正确切词 |
| 混合排序 | RRF（Reciprocal Rank Fusion，k = 60） | 融合两路名次而非分数相加 |
| 生成 | 课程对话网关（OpenAI 兼容 chat） | 仅有命中切片时调用；不流式 |

**备选与否决（用自己的话记录）：**
- *分数加权求和替代 RRF*：两路分数量纲不同（全文相关度 vs 余弦相似度），直接相加需重标定权重，换嵌入模型就要重调——否决，选名次融合。
- *LangChain/LlamaIndex 编排*：省事但把切分与检索逻辑藏进框架，无法对照规约逐条验收——否决，Go 自研。
- *`chunk_text` 写入 Qdrant payload*：省一次回表，但正文双副本、一致性问题更大——否决，正文只留 MySQL。
- *pgvector/Milvus/Chroma*：均可；选 Qdrant 是课程统一环境，Compose 一容器即起。
- *BM25/分词器（如 jieba）替代 ngram*：效果可能更好，但引入额外组件；ngram 是 MySQL 内建、零依赖——本课选 ngram。

## Decisions

### D1 存储分工：正文在 MySQL，向量在 Qdrant

- 切片正文、字符区间、状态存 `knowledge_chunks`；Qdrant 仅存浮点向量与标识（payload：`class_id`、`material_id`、`knowledge_entry_id`、`chunk_id`、`chunk_index`，无正文）。
- 关联方式：**向量主键 = `knowledge_chunks.id` = `payload.chunk_id`**，三者一致；命中向量后以主键回 MySQL 取 `chunk_text` 与材料信息。
- 摘录、关键字检索、结果展示均不依赖 Qdrant。

### D2 切分策略与预处理

| 策略 | 规则 | 适用 |
| --- | --- | --- |
| `auto`（默认） | 最大 800 字、重叠 80 字；优先在空行、换行、句号处断开 | 常规正文、种子材料补齐索引 |
| `custom` | 最大 100–2000 字、重叠 0–50%；无断点处强制截断；可选预处理（移除 URL/邮箱、折叠连续空白） | 按粒度控制切分 |
| `hierarchy` | 按 `#`/`##`/`###` 分章，标题保留在该章切片内；过长章节再按 auto 规则切 | 带标题的课程资料 |

- 切分模块输入为 `body_text` 与策略，输出切片文本、序号与位置；**不调用嵌入、不写 Qdrant**（由检索模块在切分返回后继续）。
- **预处理不改写原文**：`knowledge_entries.body_text` 保持上传原样；切片偏移量相对预处理后文本计算，不等于原文件下标。
- `auto` 请求中另行填写的长度/预处理参数不生效；custom 参数超出范围（如 50 字、3000 字、重叠 60%）返回 400。
- **切换策略不自动重切**：已入库材料保持旧切片；重建索引按**本次请求**的策略执行，先删旧切片与旧向量，再重新写入。

### D3 嵌入网关

- 仅在服务端调用课程嵌入网关（OpenAI 兼容 `/embeddings`）；向量维度须与 Qdrant collection 维度一致（环境变量 `EMBEDDING_DIM`，创建集合时使用）。
- 网关地址、密钥、模型名走环境变量（`EMBEDDING_API_BASE` / `EMBEDDING_API_KEY` / `EMBEDDING_MODEL`），缺失即启动失败；密钥不下发浏览器。
- 上传时逐切片嵌入；**嵌入失败不阻断材料保存**：`knowledge_entries` 与 `materials` 记录保留，对应切片 `index_status = failed`，不写不完整向量。种子材料启动时按 auto 策略补齐索引，同样容错。

### D4 向量库 Qdrant

- Compose 新增 `qdrant` 服务，**不映射宿主端口**，仅内部网络；Go 经 `QDRANT_URL`（如 `http://qdrant:6333`）访问。
- 集合 `campusclaw_chunks`，度量余弦；启动时若集合不存在则创建（维度取 `EMBEDDING_DIM`）。
- 写入：upsert 以 `chunk_id` 为点主键；删除：按 `material_id` 过滤删除（重建索引用）。

### D5 三种检索模式与 RRF

| 模式 | 路径 | 排序 |
| --- | --- | --- |
| `keyword` | MySQL 全文索引（`MATCH ... AGAINST`，仅 `index_status = ready`） | 全文相关度降序 |
| `vector` | 问句嵌入 → Qdrant 按 `class_id` 过滤 + 余弦检索 → 相似度 **< 0.35 丢弃** → 主键回 MySQL | 余弦相似度降序 |
| `hybrid`（默认） | 并发执行两路，各自独立过滤 | 每片 RRF 分 `1/(60+rank)` 求和，降序 |

- RRF 融合的是**名次**而非分数；缺席的一路不贡献分数；`k = 60` 为常数，非相关性阈值。
- 结果字段：材料标题、切片序号、字符区间（start/end）、摘录（取自 `chunk_text`）、各路 score 与融合名次、材料 id（供跳转详情，沿用第 3 课 404 语义）。

### D6 检索侧班级隔离（沿用第 3 课哲学）

- 班级标识**仅从登录会话读取**：认证模块依 Cookie 写入请求上下文，检索模块只从上下文取班级；query/JSON/header 中的 `class_id` 解析后一律丢弃。
- **双侧过滤 + 回表核对**：关键字路径 SQL 带 `class_id = 会话班级`；向量路径 Qdrant 查询带 `class_id` 过滤，回表时再用同一班级条件核对——即使向量侧漏过滤，回表仍兜底。
- **跨班检索对外表现为无命中**：200 + 空 hits（或问答固定文案），不以 403/404 暗示资料存在；按材料 id 打开详情沿用第 3 课"跨班与不存在同形 404"。

### D7 问答：先检索，再生成

- `POST /api/ask`：仅以**最新一句**提问发起本班混合检索，取前 4 条切片。
- 有命中：对话模块收到材料标题、切片序号、切片正文与用户提问（不含向量分量、不含他班材料）；服务端写入 system 提示（仅允许依据编号资料作答）；回答中 `[n]` 标注与 `citations`（标题+切片序号+材料 id）顺序一致。
- 无命中：接口在检索处直接返回 `{"answer":"资料中未找到相关内容","citations":[]}`，**不调用对话网关**——防止模型凭参数记忆勉强作答（天气、比分类问题）。
- 客户端提交的 `system` 消息丢弃；历史对话可附带但不用于检索；本课不实现流式。
- 对话模块输入由检索模块组装，**不得自行访问数据库或向量库**。

### D8 容错与降级

- Qdrant 不可用：`keyword` 模式仍可返回结果；`vector` / `hybrid` 返回 503，不编造相似度分数；`ask` 同样 503（依赖混合检索）。
- 嵌入网关不可用：上传材料仍成功（切片标 failed）；`vector` / `hybrid` / `ask` 返回 503；`keyword` 正常。
- 空查询（或去空白后为空）返回 400；模式名非法返回 400。
- 单条切片嵌入失败不中断同批其余切片；已失败切片可经重建索引重试。

### D9 重建索引（教师专属）

- `POST /api/materials/{id}/reindex`（教师角色，沿用第 3 课会话与班级校验）：按请求体指定的策略，先删该材料的旧切片（MySQL）与旧向量（Qdrant 按 `material_id` 过滤删除），再走切分 → 嵌入 → 写入全流程。
- 幂等：同一材料重复重建结果一致；学生调用返回 403；跨班材料返回同形 404。

## 接口一览（新增）

| 方法与路径 | 鉴权 | 说明 |
| --- | --- | --- |
| `POST /api/search` | 会话 | 检索本班切片；body：`{query, mode?}`，mode ∈ keyword/vector/hybrid（默认 hybrid） |
| `POST /api/ask` | 会话 | 问答；body：`{question, history?}`；先混合检索前 4 条，再决定是否生成 |
| `POST /api/materials/{id}/reindex` | 会话 + 教师 | 按新策略重建该材料索引（body：`{strategy, ...params}`） |

既有接口不变；检索与问答均在 `dbGuard` 与会话中间件之后。

## 数据模型（新增）

```sql
CREATE TABLE knowledge_chunks (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  knowledge_entry_id BIGINT NOT NULL,
  material_id BIGINT NOT NULL,
  class_id BIGINT NOT NULL,
  chunk_index INT NOT NULL,
  chunk_text MEDIUMTEXT NOT NULL,
  char_start INT NOT NULL,
  char_end INT NOT NULL,
  index_status VARCHAR(16) NOT NULL DEFAULT 'pending',  -- ready | failed | pending
  strategy VARCHAR(16) NOT NULL DEFAULT 'auto',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_chunks_class (class_id),
  INDEX idx_chunks_material (material_id),
  FULLTEXT KEY ft_chunk_text (chunk_text) WITH PARSER ngram
);
```

`class_id` 非空并建索引（沿用第 3 课租户列纪律）；ngram token 默认长度 2。

## 配置与启动（.env.example 新增）

`EMBEDDING_API_BASE`、`EMBEDDING_API_KEY`、`EMBEDDING_MODEL`、`EMBEDDING_DIM`、`CHAT_API_BASE`、`CHAT_API_KEY`、`CHAT_MODEL`、`QDRANT_URL`、`CHUNK_AUTO_MAX_CHARS`（默认 800）、`CHUNK_AUTO_OVERLAP`（默认 80）、`VECTOR_SCORE_THRESHOLD`（默认 0.35）、`RRF_K`（默认 60）、`ASK_TOP_K`（默认 4）。均为必填或带文档化默认值；密钥缺失即启动失败。Compose 新增 qdrant 服务与 `qdrant_data` volume，不映射端口。

## Risks / Trade-offs

- 网关为外部依赖 → 全部降级路径已定义（D8），keyword 模式保证最小可用。
- ngram 全文检索精度一般（无分词语义） → 接受：课程统一环境，且 hybrid 模式由向量路径补语义。
- 重叠 80 字带来的近重复命中 → RRF 融合后同材料相邻切片自然聚合，不额外去重（留待观察）。

## Migration Plan

- `add-traceable-vector-retrieval` Apply 时：新增表与全文索引（幂等 `CREATE TABLE IF NOT EXISTS`）；Qdrant 集合启动时自动创建；种子材料按 auto 策略补齐索引（嵌入失败标 failed 不阻断启动）。
- 既有 `knowledge_entries` 数据不动；首次启动即为"补齐索引"过程，无需单独迁移脚本。

## Open Questions

（无。）
