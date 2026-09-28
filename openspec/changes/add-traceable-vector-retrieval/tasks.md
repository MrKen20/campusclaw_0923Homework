# Tasks: add-traceable-vector-retrieval

按顺序执行，一次一个 task：实现 → 审查 diff → 运行 verify → 勾选 → 提交。所有复选框初稿保持未勾选，勾选发生在 Apply 阶段且以 verify 实际结果为准。

## 1. 配置与 Compose 骨架

- [ ] 1.1 `.env.example` 新增嵌入/对话网关与 Qdrant 配置项（EMBEDDING_API_BASE/KEY/MODEL/DIM、CHAT_API_BASE/KEY/MODEL、QDRANT_URL、CHUNK_* 默认、VECTOR_SCORE_THRESHOLD、RRF_K、ASK_TOP_K）；密钥缺失启动失败 — verify: 删除任一必需网关变量后 api 启动报错退出
- [ ] 1.2 Compose 新增 qdrant 服务（不映射宿主端口）与 qdrant_data 卷 — verify: `docker compose ps` 显示 qdrant 运行且 `nc -z localhost 6333` 不可达；仅 web 映射 8080
- [ ] 1.3 启动时自动创建 Qdrant 集合 `campusclaw_chunks`（余弦，维度取 EMBEDDING_DIM）— verify: 启动后 `docker compose exec qdrant curl -s localhost:6333/collections/campusclaw_chunks` 返回集合信息

## 2. 数据层

- [ ] 2.1 新增 `knowledge_chunks` 表（class_id NOT NULL + 索引 + `FULLTEXT ... WITH PARSER ngram` + index_status），幂等建表 — verify: `SHOW CREATE TABLE knowledge_chunks` 含 ngram 全文索引；插入空 class_id 被拒
- [ ] 2.2 种子材料启动时按 auto 策略补齐索引（嵌入失败标 failed 不阻断启动）— verify: 全新启动后种子材料有切片记录；断网关启动仍成功且 index_status=failed

## 3. 切分模块

- [ ] 3.1 实现 `auto` 策略（800 字窗口、80 字重叠、空行/换行/句号优先断开），请求中的自定义参数不生效 — verify: 单元测试：>800 字文本切出的每片 ≤800 字且相邻重叠约 80 字；带越界参数请求结果不变
- [ ] 3.2 实现 `custom` 策略（100–2000 字、0–50% 重叠、URL/邮箱移除与空白折叠预处理；参数越界 400）— verify: 单元测试覆盖参数边界与预处理；接口层 400 实测
- [ ] 3.3 实现 `hierarchy` 策略（按 #/##/### 分章、标题保留、超长章节回退 auto）— verify: 单元测试：带标题 Markdown 的首切片以对应标题开头
- [ ] 3.4 预处理不改写 `body_text`，切片偏移相对预处理后文本 — verify: 对比重建索引前后 `knowledge_entries.body_text` 逐字节一致

## 4. 嵌入与向量写入

- [ ] 4.1 实现嵌入网关客户端（服务端调用 /embeddings，维度校验）— verify: 单元测试 mock 网关返回定长向量；维度不符报错
- [ ] 4.2 上传管线追加：切分 → 逐切片嵌入 → upsert Qdrant（点主键=chunk id，payload 仅五标识字段）→ index_status=ready；失败标 failed 且 Qdrant 无残留 — verify: 教师上传后查 Qdrant 点数=切片数；payload 无正文；停网关上传后 failed 切片在 Qdrant 无点
- [ ] 4.3 实现 Qdrant 客户端（创建集合、upsert、按 class 过滤检索、按 material 删除）— verify: 集成测试：写入两点后按 class 过滤只返回本班点；按 material 删除后点消失

## 5. 检索三模式

- [ ] 5.1 `keyword`：MySQL 全文索引（MATCH/AGAINST，仅 ready），不调嵌入与 Qdrant — verify: 原词检索命中；断 Qdrant 与网关后 keyword 仍返回结果
- [ ] 5.2 `vector`：问句嵌入 → Qdrant class 过滤 + 余弦检索 → 阈值 0.35 过滤 → 主键回表取正文（回表再核对班级）— verify: 同义改写命中、低于阈值候选不出现；响应含相似度分数
- [ ] 5.3 `hybrid`：两路并发，RRF（k=60）融合名次，缺席一路不贡献分数；默认模式 — verify: 构造仅单路命中的用例，融合排序正确；两路分数字段与融合名次均在响应中
- [ ] 5.4 空查询/非法 mode 返回 400；结果字段（标题/切片序号/字符区间/摘录/材料 id）完整 — verify: curl 三类非法请求均 400；正常结果字段齐全且摘录与 chunk_text 一致

## 6. 检索班级隔离验收

- [ ] 6.1 请求体/查询串中 class_id 被丢弃 — verify: A 班会话带 `class_id=B` 检索，结果与不带时一致
- [ ] 6.2 跨班检索对外无命中（200 + 空 hits，非 403/404）— verify: A 班用户用仅存在于 B 班正文的词检索，三种模式均 200 空 hits
- [ ] 6.3 双侧过滤 + 回表兜底 — verify: 代码审查确认 Qdrant 查询带 class 过滤且回表 SQL 带 class 条件；人为构造越界候选时回表仍排除

## 7. 问答接口

- [ ] 7.1 `POST /api/ask`：混合检索前 4 条；有命中才调对话网关，system 由服务端写入，[n] 与 citations 顺序一致 — verify: 有依据问题返回回答+citations 且顺序对应；检查对话网关请求不含向量分量与他班材料
- [ ] 7.2 无命中返回固定文案、citations 为空、不调网关 — verify: 问"今天天气"返回固定文案；网关日志无该请求
- [ ] 7.3 客户端 system 注入被丢弃；历史消息仅附带不参与检索 — verify: 请求携带 system 消息后实际网关请求中无该内容
- [ ] 7.4 问答班级隔离 — verify: B 班用户问仅 A 班材料能答的问题，返回固定文案

## 8. 索引重建

- [ ] 8.1 `POST /api/materials/{id}/reindex`（教师专属）：删旧切片与旧向量 → 按新策略重建；幂等 — verify: 教师以 hierarchy 重建后切片按章节重排且 Qdrant 旧点删除；连建两次结果一致
- [ ] 8.2 学生 403；跨班材料同形 404 — verify: 学生调用 403 且数据不变；A 班教师对 B 班材料 reindex 得 404

## 9. 前端界面

- [ ] 9.1 检索入口与模式选择（keyword/vector/hybrid，默认 hybrid），结果展示标题/切片序号/摘录并可跳转详情 — verify: 浏览器走查：三模式可切换、命中条目点击进入材料详情
- [ ] 9.2 问答入口：展示回答正文与引用列表，[n] 与 citations 对应可辨认 — verify: 浏览器走查：回答中标注与引用列表对应；无命中时显示固定文案
- [ ] 9.3 前端不直连 Qdrant/网关 — verify: 构建产物与网络面板中无 Qdrant/网关地址与密钥

## 10. 发布验收

- [ ] 10.1 主路径证据：上传→切片入库→三模式检索命中→问答带引用，各留可复核输出 — verify: 命令+输出或截图留档，同伴可复现
- [ ] 10.2 失败路径证据：跨班检索无命中、请求篡改 class_id 无效、无依据不调模型、Qdrant 故障降级（keyword 可用/vector 503）— verify: 四条证据齐全
- [ ] 10.3 同伴交叉验证：对方按 README 从零启动并实测一条跨班检索 — verify: 记录互验结论
- [ ] 10.4 迭代说明：追加第 4 课决策（存储分工/RRF/降级/无依据不生成）与 Scenario 判定表 — verify: 与 design.md 一致，同伴能复述
- [ ] 10.5 `openspec validate add-traceable-vector-retrieval --strict` — verify: exit 0 且无 error
- [ ] 10.6 打版本 tag — verify: `git tag v0.2.0-vector-retrieval` 后从 tag 检出可启动
