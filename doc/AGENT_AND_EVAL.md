# 推荐 Agent、Harness 与可复现评测

更新日期：2026-08-19。当前生产默认运行有界并行 ReAct，业务与检索数据统一存入 PostgreSQL 17 + pgvector；Redis 只保存缓存、会话、运行检查点和秒杀状态。

## 1. 从空环境复现

前置条件：Docker Desktop。只有需要真实模型的 Agent/过滤条件评测才需要 DeepSeek API key，密钥应只注入当前 shell。

```bash
make docker-reset
LLM_API_KEY='你的密钥' make docker-up
LLM_API_KEY='你的密钥' make docker-verify

# PostgreSQL/pgvector 检索、同任务对照与 Agent
LLM_API_KEY='你的密钥' make docker-eval

# 生产路由到最终回答；不强制路由，不运行对照组
LLM_API_KEY='你的密钥' make docker-agent-e2e-v61
```

空卷验收必须同时满足：PostgreSQL 中 200 家店、1000 条评价、200 条检索文档；`vector` 扩展、HNSW 和 GIN 索引存在；Redis 不含店铺向量；App、PostgreSQL、Redis、RocketMQ healthy；迁移与初始化任务退出码为 0。

## 2. 生产链路

```text
请求
  -> 鉴权、输入限制与不可信文本隔离
  -> 大模型理解请求：意图、查询改写、硬条件、证据需求、置信度
  -> 三路路由：rag_oneshot / agent / clarify
  -> agent 路由进入 Harness
       -> 只装配相关会话摘要和结构化画像
       -> 创建有界 AgentState、预算、取消信号与 trace id
       -> 动作控制器输出 act / finish / clarify
       -> search_shops 先建立候选注册表
       -> 候选已知后，详情和评价工具按依赖图并行
       -> 更新候选、证据账本和证据缺口
       -> 证据不足时翻页、改写重搜并重新决策
       -> 生成逐条声明，校验字段、引用、硬条件和评价蕴含
       -> Harness 返回标准结果
  -> 应用层持久化运行审计；仅成功且已校验的结果可更新长期画像
```

生产路由只有三种。历史上下文和长期偏好是完整 Agent 内部的上下文策略，不是独立 Agent，也不是一条“记忆路由”。旧请求值仅在入口兼容，进入系统后立即归一为 `agent`。

## 3. Harness 的职责边界

`RecommendAgentHarness` 是一次 Agent 运行的稳定边界，屏蔽规划基线、旧工具循环和当前 ReAct 运行时的差异。

| Harness 负责 | Harness 不负责 |
|---|---|
| 输入校验、上下文裁剪与结构化装配 | HTTP 路由和鉴权 |
| 运行时选择、预算、超时和客户端取消传递 | 直接查询或写入 PostgreSQL/Redis |
| 工具注册、候选屏障、参数去重和依赖执行 | 决定长期画像的业务事务 |
| 证据账本、逐条声明校验和安全回退 | 保存原始问句、完整评价或思维链 |
| 统一输出步骤、调用、Token、终止原因、trace id | 把运行检查点当作长期记忆 |

这样做有三个直接收益：线上入口与离线评测复用相同执行边界；运行时可替换而不改变业务层；Harness 单测可以注入确定性模型、工具和检查点，复现超时、重试、恢复和校验失败。

## 4. 为什么不能一开始并行查详情和评价

详情与评价工具都需要合法 `shop_id`。在检索完成前，系统不知道候选 ID，也不能允许模型猜 ID。因此第一批动作必须包含 `search_shops`；搜索结果写入候选注册表后，同一批候选的 `get_shop` 与 `list_shop_blogs` 才成为依赖图的就绪节点并行执行。

并行执行器还会：

- 验证 `depends_on`，拒绝环和未知依赖；
- 以 `errgroup` 限制并发数；
- 对单工具设置超时，对瞬时错误做有界重试；
- 以规范化参数哈希拒绝重复调用；
- 跳过依赖失败的动作，并把结果写入下一轮观察。

评价不是无限读取。工具返回服务端签发的游标；证据缺口仍存在且游标非空时，下一轮才允许续查，默认每店最多两页。

## 5. 上下文、偏好和检查点

- PostgreSQL 是长期画像事实源，使用版本号 CAS 和事件表审计变更。
- Redis 保存有界会话、摘要和 30 分钟运行检查点；检查点按 `run_id + revision` CAS，供跨实例恢复。
- 本轮明确条件始终高于长期偏好；被覆盖的旧区域、类别或预算不会注入提示词。
- 显式偏好修改走确定性补丁；模型推断偏好只有在运行完成且回答校验成功后才能提交。
- 运行检查点描述“这次任务做到哪里”，长期画像描述“用户稳定偏好”，两者生命周期和写入条件不同。

## 6. 可观测性与分布式追踪

W3C Trace Context 从 Gin 请求贯穿 `agent.run -> agent.controller / agent.action -> tool.execute`。SSE 的 `run_started`、最终 `done` 和 PostgreSQL `agent_runs` 使用同一 `trace_id`。

- Jaeger：查看跨 Nginx、Go 实例、控制器和工具的 span、父子关系与耗时。
- PostgreSQL：保存运行终态、路由、预算、Token、延迟、证据摘要以及每次工具尝试的参数哈希、错误码、耗时和结果数。
- Redis：保存可恢复状态，不承担长期审计。

审计表不保存原始问句、完整评价和思维链。`trace_id` 用于把一次用户请求的实时事件、链路和有界审计关联起来；`run_id` 用于同一执行的检查点恢复。

## 7. PostgreSQL + pgvector 检索

`shop_search_documents` 同时保存检索文本、结构化过滤字段、全文向量、384 维 embedding、模型名、内容哈希和来源版本。

- 全文路：确定性中英文分词写入 `tsvector`，使用 GIN 索引。
- 语义路：pgvector HNSW + 余弦距离。
- 融合：两路各取 20 个候选，以 RRF 融合后返回前 5；引号内规范化店名完全一致时固定优先。
- 过滤：区域、类型、价格、评分和评论数直接在 PostgreSQL 查询中下推。
- 更新：RocketMQ 消费者异步生成新文档；`source_version` 防止旧消息覆盖较新向量。

与“关系数据在 MySQL、派生向量在 Redis”的旧结构相比，当前方案减少了一个持久化系统，向量可备份、可审计，检索文档可按来源版本重放。Redis 因而可以专注于低延迟、可过期状态。

## 8. 安全与终止条件

- 检索和评价正文标为不可信数据，文本中的系统指令、假工具输出和假引用不能改变控制流。
- 只有本轮搜索注册过的店铺 ID 才能进入详情、评价和最终引用。
- 最终回答先生成结构化声明；字段声明必须等于工具事实，体验声明必须由同店评价支持。
- 默认最多 4 轮、10 次成功工具调用、12 次尝试、2 次搜索、每店 2 页评价和 3 个并行工具。
- 重复调用、无新证据、预算耗尽、超时和客户端取消都有显式终态；证据不足时返回澄清或安全无结果。

## 9. 数据与评测口径

固定 seed `20260729` 生成 200 家店和 1000 条评价，覆盖近名难负样本、无结果、评价冲突、偏好纠正、长上下文、提示注入和伪造工具文本。

- Retrieval v2：8 dev / 60 test；固定过滤条件的 PostgreSQL/pgvector 回归为 60/60 任务成功，HitRate@5 100%，Recall@5 81.6%，MRR/NDCG@5 1.0，P50/P95 2/17 ms，基础设施错误 0。
- Router v1：48 test；三路规则回归 48/48。
- Router v2：12 dev / 52 challenge；三路规则回归 52/52。100% 只说明规则覆盖这组人工意图边界，不代表线上准确率。
- Agent v6.1：8 dev / 40 challenge、56 次真实模型执行；通过生产路由到最终回答，不强制 Agent 路由。本次逐次任务成功率 92.86%、综合通过率 91.07%、场景宏平均任务成功率 90.00%、成功回答有据率 100%，P50/P95 7.208/21.629 秒，基础设施错误 0。
- 分页、重试、超时、取消、检查点恢复和并发上限由确定性 Harness/运行时测试覆盖，不能拿 fake runner 当模型质量结果。

`outcome` 衡量任务约束是否满足；`groundedness` 衡量引用和事实是否有证据；`trajectory` 衡量工具轨迹是否符合预算；三者全部通过才是 `composite pass`。多次运行的场景同时报告 trial-micro 和 scenario-macro，避免重复 trial 放大某类样本权重。

历史 v4 在 28 个复杂场景上得到 Agent 与单轮 Hybrid RAG 的 scenario-macro 任务成功率 96.43% vs 46.43%。这是旧运行时的冻结对照，只能说明复杂任务中多步工具调用的价值，不能作为当前运行时或自然流量的线上效果。

## 10. 生成与检查

```bash
go run ./cmd/generate-eval-data --check
go run ./cmd/generate-challenge-data --check
go run ./cmd/generate-challenge-data --suite=v31 --check
go run ./cmd/generate-challenge-data --suite=v4 --check
go run ./cmd/generate-challenge-data --suite=v5 --check
go run ./cmd/generate-challenge-data --suite=v6 --check
go run ./cmd/generate-challenge-data --suite=v61 --check
```

关键实现：

- Harness：`internal/agent/harness.go`、`internal/agent/react_harness.go`
- 请求理解：`internal/agent/understanding.go`
- 三路路由：`internal/logic/adaptive_recommend_router.go`
- 状态机与并行执行：`internal/agent/react_runtime.go`、`internal/agent/react_executor.go`
- 证据与回答校验：`internal/agent/evidence.go`、`internal/agent/claims.go`
- PostgreSQL/pgvector：`internal/bootstrap/migrate.go`、`internal/repository/vector_repo.go`
- 审计与画像提交：`internal/logic/recommend_agent_logic.go`

## 11. 已知限制

- 数据主要由固定种子合成，没有真实流量频率、人工双标或跨城市长尾。
- 本地特征哈希向量便于复现，但开放域同义表达和错别字能力有限。
- 请求理解、动作控制器和语义蕴含阈值仍缺真实分布上的概率校准与灰度验证。
- MQ 更新检索文档尚未采用事务 Outbox；数据库提交后存在短暂陈旧窗口，后续应增加对账重建任务。
- 当前基础评价每店默认 5 条，真实多页数据需要独立、版本化的运行时数据覆盖层。

历史工程问题与修复记录保留在 `EVAL_PRACTICE_LOG.md`，其中旧数据库和旧路由名称只代表当时版本。
