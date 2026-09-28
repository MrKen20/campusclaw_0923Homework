# knowledge-retrieval 能力规约（delta）

## Purpose

在第 3 课身份、角色与班级隔离的底座之上，为 CampusClaw 增加限定班级范围、可追溯出处的知识库检索：上传的材料被切分为切片并向量化；教师与学生以一句自然语言检索本班材料片段，每条结果可回溯至材料标题、切片序号、字符区间与摘录；问答仅在存在依据切片时生成，并以引用标注回指原文；无依据时明确返回未找到，不得编造。

## ADDED Requirements

### Requirement: 切片入库管线

教师上传材料成功后，系统 MUST 将知识库正文按切分策略切分为切片：切片正文、序号与字符区间 MUST 写入 MySQL `knowledge_chunks` 表；每个切片 MUST 经服务端嵌入网关转为向量并写入 Qdrant 集合 `campusclaw_chunks`。向量主键 MUST 与 `knowledge_chunks.id` 一致；向量 payload MUST 仅含 class_id、material_id、knowledge_entry_id、chunk_id、chunk_index，MUST NOT 包含正文。

#### Scenario: 上传后切片与向量入库

- **WHEN** 教师 A 上传一份合法材料且解析成功
- **THEN** `knowledge_chunks` 表中 MUST 出现该材料的切片记录（含 chunk_index、chunk_text、字符区间、index_status）
- **AND** Qdrant 中 MUST 存在与各切片一一对应的向量点，点主键与切片 id 一致
- **AND** 本班学生 A1 的检索结果中 MUST 能命中该材料的切片

#### Scenario: 嵌入失败不丢原文

- **WHEN** 某切片的向量嵌入调用失败
- **THEN** 该材料记录与 `knowledge_entries` 正文 MUST 仍然保留
- **AND** 对应切片的 `index_status` MUST 标记为 `failed`
- **AND** Qdrant 中 MUST NOT 存在该切片的不完整或占位向量

#### Scenario: 种子材料补齐索引

- **WHEN** 系统启动时存在尚无切片的既有材料（含种子材料）
- **THEN** 系统 MUST 按 `auto` 策略为其补齐切片与向量索引
- **AND** 补齐过程的嵌入失败 MUST 不阻断系统启动

#### Scenario: 向量库不含正文

- **WHEN** 检查 Qdrant 集合中任意向量点的 payload
- **THEN** payload MUST 仅包含 class_id、material_id、knowledge_entry_id、chunk_id、chunk_index 五个标识字段
- **AND** MUST NOT 包含切片正文或其片段

### Requirement: 切分策略

系统 MUST 支持三种切分策略：`auto`（默认，最大 800 字、重叠 80 字，优先在空行、换行、句号处断开）、`custom`（最大长度 100–2000 字、重叠比例 0–50%，可选移除 URL 与邮箱、折叠连续空白）、`hierarchy`（按 Markdown `#`/`##`/`###` 层级分章，标题保留在该章切片内，过长章节再按 auto 规则切分）。预处理 MUST NOT 改写 `knowledge_entries.body_text`；切片偏移相对预处理后的文本计算。

#### Scenario: auto 默认窗口切分

- **WHEN** 上传未指定策略的材料（或种子材料补齐索引）
- **THEN** 切分 MUST 按 auto 策略执行：单切片不超过 800 字，相邻切片重叠约 80 字
- **AND** 请求中另行填写的长度与预处理参数 MUST NOT 生效

#### Scenario: custom 参数校验

- **WHEN** 重建索引请求 custom 策略携带越界参数（如最大长度 50 或 3000 字、重叠比例 60%）
- **THEN** 系统 MUST 返回 400
- **AND** 既有切片与向量 MUST 保持不变

#### Scenario: hierarchy 按标题分章

- **WHEN** 对含 `#`/`##` 标题的 Markdown 材料按 hierarchy 策略切分
- **THEN** 每个章节的切片 MUST 以该章标题开头（标题保留在切片内）
- **AND** 超长章节 MUST 再按 auto 窗口规则切分

#### Scenario: 预处理不改写原文

- **WHEN** custom 策略启用"折叠连续空白"等预处理
- **THEN** 切片与偏移 MUST 基于预处理后的文本计算
- **AND** `knowledge_entries.body_text` MUST 保持上传时的原文不变

#### Scenario: 更换策略不自动重切

- **WHEN** 材料已按某策略入库，未显式重建索引
- **THEN** 既有切片 MUST 保持不变（系统 MUST NOT 自动重新切分）

### Requirement: 三种检索模式

系统 MUST 提供 `keyword`、`vector`、`hybrid`（默认）三种检索模式。`keyword` 仅查询 MySQL ngram 全文索引，MUST NOT 调用嵌入服务或访问 Qdrant；`vector` 将问句嵌入后在 Qdrant 按余弦相似度检索，相似度低于 0.35 的候选 MUST 丢弃，再以向量主键回 MySQL 取回正文；`hybrid` MUST 并发执行两路、各自独立过滤后按名次做 RRF 融合（k = 60），缺席的一路 MUST NOT 贡献分数。

#### Scenario: keyword 命中原词

- **WHEN** 用户以原词（词直接出现在本班某切片正文中）发起 keyword 检索
- **THEN** 该切片 MUST 出现在结果中并按全文相关度降序排列
- **AND** 系统 MUST NOT 为该请求调用嵌入网关或 Qdrant

#### Scenario: vector 命中语义改写

- **WHEN** 用户以同义改写（原词未出现但语义相近）发起 vector 检索
- **THEN** 语义相近的切片 MUST 仍可命中
- **AND** 余弦相似度低于 0.35 的候选 MUST NOT 出现在结果中

#### Scenario: hybrid RRF 融合

- **WHEN** 用户发起 hybrid 检索且两路均有候选
- **THEN** 最终排序 MUST 由两路名次按 `1/(60+rank)` 融合计算
- **AND** 仅一路命中的切片 MUST 按单路名次参与排序
- **AND** 结果 MUST NOT 以两路原始分数直接相加排序

#### Scenario: 检索结果字段完整可溯源

- **WHEN** 检索返回非空结果
- **THEN** 每条结果 MUST 至少包含材料标题、切片序号、字符区间与一段摘录
- **AND** 摘录 MUST 取自 MySQL 的 `chunk_text`
- **AND** 结果 MUST 提供材料标识以便打开对应材料详情

#### Scenario: 空查询与非法模式

- **WHEN** 检索请求的 query 为空或去空白后为空，或 mode 不属于三种模式
- **THEN** 系统 MUST 返回 400

### Requirement: 检索侧班级隔离

检索与问答的班级边界 MUST 与第 3 课一致：班级标识仅从登录会话读取，请求中携带的班级编号 MUST 无效；关键字与向量两条路径 MUST 均带班级条件，回表查询 MUST 再次核对；跨班级检索对外 MUST 表现为无命中。

#### Scenario: 请求中的班级编号被丢弃

- **WHEN** A 班用户在检索或问答请求体/查询串中写入 B 班的 `class_id`
- **THEN** 实际过滤条件 MUST 仍为 A 班
- **AND** 结果 MUST 与不带该参数时完全一致

#### Scenario: 跨班检索表现为无命中

- **WHEN** A 班用户以仅出现在 B 班切片正文中的词发起任一模式检索
- **THEN** 响应 MUST 为 HTTP 200 且 hits 为空
- **AND** MUST NOT 以 403 或 404 暗示该资料属于其他班级

#### Scenario: 向量侧漏过滤时回表兜底

- **WHEN** Qdrant 返回了含其他班级的候选（如过滤条件异常失效）
- **THEN** 回 MySQL 取正文时的班级条件 MUST 将其他班级记录排除
- **AND** 最终结果 MUST NOT 出现其他班级的切片

#### Scenario: 关键字路径同样隔离

- **WHEN** A 班用户以仅出现在 B 班切片正文中的词发起 keyword 检索
- **THEN** MySQL 查询条件 MUST 包含 `class_id = 会话班级`
- **AND** 结果 MUST 为空

### Requirement: 问答与引用标注

`POST /api/ask` MUST 先以混合模式检索本班前 4 条切片；仅当存在命中切片时 MUST 调用对话网关生成简短回答，回答中的 `[n]` 标注 MUST 与 `citations` 列表顺序一致；无命中时 MUST 返回固定文案且 MUST NOT 调用生成模型。客户端注入的 `system` 消息 MUST 被丢弃，system 提示 MUST 由服务端写入。

#### Scenario: 有依据时回答带引用

- **WHEN** 用户提出材料中确有依据的问题且混合检索命中切片
- **THEN** 系统 MUST 将命中切片（材料标题、切片序号、切片正文）与提问交给对话网关
- **AND** 回答中的 `[1]`、`[2]` 标注 MUST 与 `citations` 列表顺序一致
- **AND** citations 每条 MUST 指明材料标题与切片序号（或材料标识）

#### Scenario: 无依据时不调用生成模型

- **WHEN** 用户提出与本班材料无关的问题（如天气、比分）且检索无命中
- **THEN** 接口 MUST 返回 HTTP 200 与固定文案「资料中未找到相关内容」
- **AND** `citations` MUST 为空
- **AND** 对话网关 MUST NOT 被调用

#### Scenario: 客户端 system 注入被丢弃

- **WHEN** 问答请求的 history 或消息中携带客户端构造的 `system` 消息
- **THEN** 该消息 MUST 被丢弃
- **AND** 实际发送给对话网关的 system 提示 MUST 由服务端写入（仅允许依据编号资料作答）

#### Scenario: 问答限定本班

- **WHEN** B 班用户提问仅 A 班材料能回答的问题
- **THEN** 检索 MUST 无命中
- **AND** 响应 MUST 为固定文案，citations 为空

### Requirement: 索引重建

系统 MUST 提供教师专属的索引重建能力：按本次请求的策略，先删除该材料旧切片与旧向量，再重新切分、嵌入、写入；重建 MUST 幂等，学生 MUST 被拒绝，跨班材料 MUST 返回同形 404。

#### Scenario: 教师按新策略重建

- **WHEN** 教师对本班某材料发起 reindex 并指定 hierarchy 策略
- **THEN** 系统 MUST 先删除该材料的旧 `knowledge_chunks` 记录与 Qdrant 旧向量
- **AND** MUST 按新策略重新生成切片与向量
- **AND** 重建后检索结果 MUST 来自新切片

#### Scenario: 学生重建被拒绝

- **WHEN** 学生调用 reindex
- **THEN** 系统 MUST 返回 403
- **AND** 目标材料的切片与向量 MUST 不变

#### Scenario: 重建幂等

- **WHEN** 对同一材料以相同策略连续重建两次
- **THEN** 两次重建后的切片数量、内容与向量主键集合 MUST 一致

### Requirement: 检索容错与降级

Qdrant 不可用时 `keyword` 模式 MUST 仍可返回结果，`vector` 与 `hybrid` 模式 MUST 返回 503 且 MUST NOT 编造相似度分数；嵌入网关不可用时上传 MUST 仍成功（切片标 failed）；问答接口依赖混合检索，降级行为与 hybrid 一致。

#### Scenario: Qdrant 故障时 keyword 可用

- **WHEN** 停止 Qdrant 容器后用户发起 keyword 检索
- **THEN** 系统 MUST 正常返回本班命中结果

#### Scenario: Qdrant 故障时 vector 与 hybrid 降级

- **WHEN** 停止 Qdrant 容器后用户发起 vector 或 hybrid 检索
- **THEN** 系统 MUST 返回 503
- **AND** 响应 MUST NOT 包含任何编造的相似度分数

#### Scenario: 嵌入网关故障不影响上传

- **WHEN** 嵌入网关不可用时教师上传材料
- **THEN** 上传 MUST 成功且原文与材料记录保留
- **AND** 对应切片 `index_status` MUST 为 `failed`

### Requirement: 配置与密钥边界

嵌入与对话网关的地址、密钥、模型名及 Qdrant 地址 MUST 仅经服务端环境变量注入，缺失必需项时启动失败；Qdrant 容器 MUST NOT 映射宿主端口；前端 MUST 只调用本站 `/api` 接口，MUST NOT 直连 Qdrant 或网关。

#### Scenario: 密钥缺失启动失败

- **WHEN** 未提供 `EMBEDDING_API_KEY` 等必需环境变量时启动 api
- **THEN** 启动 MUST 失败并给出明确错误
- **AND** MUST NOT 回落内置默认值

#### Scenario: Qdrant 不对外暴露

- **WHEN** 检查 Compose 配置与本机端口
- **THEN** qdrant 服务 MUST NOT 映射宿主端口
- **AND** 浏览器页面代码中 MUST NOT 出现 Qdrant 或网关地址

#### Scenario: 前端不持有网关密钥

- **WHEN** 检查前端构建产物与网络请求
- **THEN** 嵌入与对话网关的密钥 MUST NOT 出现在前端代码或请求中
- **AND** 检索与问答请求 MUST 均指向本站 `/api`

### Requirement: 检索与问答界面

前端 MUST 提供检索入口（支持模式选择）与问答入口：检索结果 MUST 展示材料标题、切片序号与摘录并可跳转材料详情；问答 MUST 展示回答正文与引用列表，引用顺序与回答标注一致。

#### Scenario: 检索结果展示与跳转

- **WHEN** 用户在本班发起检索并命中切片
- **THEN** 界面 MUST 列出每条命中的材料标题、切片序号与摘录
- **AND** 点击命中条目 MUST 打开对应材料详情（沿用第 3 课鉴权与 404 语义）

#### Scenario: 问答展示引用

- **WHEN** 问答返回带引用的回答
- **THEN** 界面 MUST 同时展示回答正文与 citations 列表
- **AND** 回答中 `[n]` 标注与列表条目的对应关系 MUST 在界面上可辨认

#### Scenario: 无命中提示

- **WHEN** 检索或问答无命中
- **THEN** 界面 MUST 显示「资料中未找到相关内容」类提示
- **AND** MUST NOT 展示任何编造的出处
