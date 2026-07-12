# 带记忆的推荐 Agent + 可评测智能搜索 — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在现有 Go 分布式点评与 Naive RAG 之上，构建「可复现评测的智能搜索 + 有界工具调用 + 可纠正结构化记忆」闭环，并用可信数字说明优化收益。

**Architecture:** 不引入 Mem0 / Eino / LangGraph / Deep Agents / Milvus。先修正现有 RAG 的配置与评测口径，再建立与线上路径一致的 retrieval eval；检索优化优先采用现有 Redis TEXT + HNSW 的 Hybrid RRF。Agent 为 3 个只读领域工具的自研 bounded tool-loop；会话与 profile 由服务端加载和持久化，模型不直接读写记忆。Trace 复用 OpenTelemetry，但补充 search / LLM / tool 自定义 span。Phase 1 仅做 Redis 结构化记忆，事实向量记忆继续后置。

**Tech Stack:** Go 1.24+、Gin、Redis Stack（Hash + TEXT + HNSW）、go-openai、现有 `RAGLogic` / `VectorRepo`、OpenTelemetry、JSON golden set、`cmd/eval-rag`、`cmd/eval-agent`。

**执行时使用：**
- `@superpowers:executing-plans` / `@superpowers:subagent-driven-development`
- `@superpowers:test-driven-development`
- `@superpowers:verification-before-completion`

**设计依据（2026-07-12 核对）：**
- [Anthropic — Building effective agents](https://www.anthropic.com/engineering/building-effective-agents)：从最简单可测方案开始，仅在收益可证明时增加 Agent 复杂度；重点设计和测试工具接口。
- [Anthropic — Demystifying evals for AI agents](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents)：20～50 条高质量任务可作为早期起点；稳定环境、多 trial、代码/模型/人工 grader 组合优先于盲目扩大数据集。
- [OpenAI — Evaluate agent workflows](https://developers.openai.com/api/docs/guides/agent-evals)：先利用 trace 定位失败，再用 dataset 与 eval run 做可重复回归。
- [Redis — FT.HYBRID](https://redis.io/docs/latest/commands/ft.hybrid/)：文本与向量结果可通过 RRF 融合；本项目先实现客户端 RRF，避免绑定特定 Redis 新版本。

---

## 0. 背景与面试叙事（写进 README / 简历前先对齐）

### 0.1 项目定位（一句话）

> Go 分布式点评（秒杀 / RocketMQ / 多实例）之上的 **可评测店铺推荐 Agent**：Hybrid 检索、结构化偏好记忆和有界 tool-loop 共用现有 Redis / OTel 底座，并由 golden set 与 Agent eval 量化验证。

### 0.2 面试亮点优先级（勿颠倒）

1. **已有后端底座**：秒杀事务消息、布隆过滤器、MQ 异步更新、Nginx 多实例——后端岗位主菜。
2. **可信评测闭环**：线上路径一致、正确指标、稳定环境、baseline 与失败样本。
3. **领域 Agent 编排**：3 个边界清晰的只读工具、预算/超时/停止条件、SSE 与 OTel。
4. **可纠正记忆**：session + profile Hash，明确覆盖、删除、并发更新和注入优先级。

不要表述为「所有后端岗位都必须会 Agent」。面试话术应是：**后端基本盘决定下限，Agent / RAG / eval 能力扩大 AI 应用与智能业务后端岗位的适配面。**

### 0.3 明确不做（YAGNI）

| 不做 | 原因 |
|------|------|
| 换 Milvus / Neo4j / 接入 BreakTheWaves 整仓 | 题材迁移成本高，叙事易变成「抄开源」 |
| Mem0 / LangGraph / Deep Agents / 先上 Eino | 秋招主项目收益低，依赖叙事弱于自研闭环 |
| 完整 Reco 微服务（独立召回池/曝光） | 与点评域重复；用 Agent 工作流表达推荐即可 |
| Phase 1 用户事实向量记忆 | 先 Hash；不够再 Phase 2 |
| 模型侧 `read_memory` / `update_memory` | profile 已由服务端注入；让模型任意读写会增加调用噪声、污染与安全风险 |
| 默认使用生成式 LLM Rerank | 非确定、慢且贵；先做可复现的 Hybrid RRF，候选召回足够后再考虑专用 reranker |
| 外部 LLM eval 直接做 PR 强门禁 | API 与模型存在抖动；Phase 1 仅提供本地/手工回归命令 |
| 生成侧完整 Ragas CI | 先保证检索、工具轨迹、记忆状态与 groundedness；开放式回答质量后置 |
| 为达到 n≥50 批量同义改写 | 当前 catalog 约 25 家店，相关样本会制造统计虚胖；质量优先于数量 |

### 0.4 已发现的前置问题（先修再记录 baseline）

1. `cmd/eval-rag/main.go` 当前把「TopK 命中任一相关项」称为 Recall@5，实际是 **HitRate@5**。
2. 当前 eval 使用 `SearchShops(..., nil, k)`，而线上 `/api/rag/chat` 会先抽取 filter；两者路径不一致。
3. embedding 配置默认维度与 Redis 索引 fallback 不一致；必须以 `LLM_EMBEDDING_DIM` 为唯一真相并校验返回向量长度。
4. 未被调用的 `RAGLogic.IngestShop` 会漏写价格、评分等 filter 元数据，应删除，避免形成错误旁路。
5. `docker-compose.yml` 使用 `redis/redis-stack-server:latest`，不利于复现实验，应锁定已验证 tag 或 digest。
6. 现有 OTel 只有 HTTP 层 `otelgin`，RAG 内部尚无 embed / search / generate span。
7. `script/rag-eval.json` 仅 7 条，只保留为 smoke set，不作为可对外宣称的 baseline。

### 0.5 目标目录结构（完成后）

```text
rag-evals/
  golden/
    retrieval.v1.json          # 25～35 条全量人工核验，含 dev/test 标记
    agent.v1.json              # 10～15 条多轮 Agent/记忆场景
  baseline/
    dense_prod_v1.json         # 线上 filter + dense 的已知基线
    hybrid_prod_v1.json        # Hybrid 对照结果（完成后）
  reports/                     # gitignore，本地/CI 产物
    retrieval_latest.json
    agent_latest.json
cmd/
  eval-rag/                    # filter mode × retriever strategy + report
  eval-agent/                  # outcome / groundedness / trajectory
internal/
  agent/                       # 新建：tool schema、bounded loop、预算
  memory/                      # 新建：纯类型、merge、extract
  rag/                         # 现有文本构建 + 新增 RRF 纯函数
  logic/
    shop_search_logic.go       # 新建：RAG / Agent / eval 共享检索入口
    rag_logic.go               # 保留：检索后生成回答
    recommend_agent_logic.go   # 新建：推荐 Agent 门面
  repository/
    interface/memory.go        # MemoryRepo 接口（必选）
    memory_repo.go             # Redis session/profile 实现
  handler/
    agent.go                   # `/api/agent/recommend` SSE
```

---

## 1. 架构设计

### 1.1 共享检索入口

`/api/rag/chat`、Agent 的 `search_shops` 与 `cmd/eval-rag` 必须调用同一个 `ShopSearchLogic`，防止「线上一套、评测一套」：

```text
question + explicit filter + optional profile
       │
       ├─ ResolveFilter
       │    ├─ 显式请求条件（最高优先级）
       │    ├─ LLM filter extractor
       │    └─ profile 默认值（仅补空字段）
       ├─ ShopSearchLogic.Search(strategy=dense|hybrid)
       │    ├─ Embed(question)
       │    ├─ VectorRepo.SearchShops
       │    └─ [hybrid] Text Search + RRF
       └─ []ShopSearchResult
              ├─ RAG：buildShopContext → ChatStream
              └─ Agent：作为 tool result 返回
```

检索评测把两个变量正交拆开：

- `--filter-mode=none|oracle|llm`
- `--retriever=dense|hybrid`

其中：

- `oracle + dense/hybrid` 用于隔离比较 retriever。
- `llm + dense/hybrid` 用于代表线上真实路径。
- `none + dense` 只作为旧版诊断，不作为生产 baseline。

### 1.2 推荐 Agent（新建）

```text
POST /api/agent/recommend  (AuthRequired)
  { "question": "...", "session_id": "..." }
       │
       ├─ 服务端 LoadSession + LoadProfile
       ├─ system prompt 注入 profile summary
       ├─ bounded loop：maxSteps=3 / maxToolCalls=5 / runTimeout=45s
       │     tools:
       │       - search_shops(query, area?, type_name?, max_price?)
       │       - get_shop(shop_id)
       │       - list_shop_blogs(shop_id, limit?)
       │
       ├─ 非流式生成完整回答 → 校验引用的 shop_id → 再发送 SSE message
       ├─ 成功结束后 PersistSession
       ├─ 回合末 ExtractProfilePatch → CAS MergeProfile
       └─ SSE status/message/done/error + OTel spans
```

为什么保留 3 个工具：

- `search_shops`：发现候选。
- `get_shop`：读取权威详情；避免将所有字段塞进搜索结果。
- `list_shop_blogs`：仅在用户关心真实评价、环境、口味时按需调用，体现动态工具选择。

为什么没有 memory tool：profile 与 session 是应用状态，不是领域知识工具。服务端控制读写，更容易校验、测试、限权与处理用户纠正。

### 1.3 Agent 可靠性边界

| 边界 | 默认值 / 行为 |
|------|---------------|
| 最大推理步数 | `3` |
| 最大工具调用数 | `5` |
| 单工具超时 | `10s` |
| 单次 Agent 总超时 | `45s` |
| 工具结果上限 | `6000` 字符，服务端截断 |
| 重复调用 | 相同 tool + canonical args 只允许一次 |
| 参数安全 | JSON Schema + Go 本地校验；shop_id/limit/价格范围校验 |
| 记忆写入 | 仅服务端回合后执行；失败不影响主回答 |
| SSE 断连 | 取消下游 context；未完成回合不写 profile |
| 幻觉约束 | 回答中的 `[shop:{id}]` 必须属于本轮工具结果集合 |
| 成本防护 | 登录态 + 用户级限流 + trace 中记录 steps/tokens/latency |

SSE 只展示「正在检索/读取详情」等状态，**不输出模型隐藏思维链**。

### 1.4 记忆模型（Phase 1：结构化、可纠正）

**短期会话**

| Key | 类型 | 内容 | TTL |
|-----|------|------|-----|
| `agent:sess:{userId}:{sessionId}` | Redis List | 最近 20 条 `{role,content,ts}` | 7d，访问后刷新 |

写入使用事务 pipeline：`RPUSH → LTRIM → EXPIRE`。加载后再按上下文预算裁剪，不能只依赖消息条数。

**长期 Profile**

| Key | 类型 | 字段 | TTL |
|-----|------|------|-----|
| `agent:profile:{userId}` | Redis Hash | `preferred_areas`、`preferred_types`、`budget_max`、`dislikes`、`summary`、`version`、`updated_at` | 90d，更新后刷新 |

**优先级与纠正规则**

1. 本轮用户显式条件 > profile > 无条件。
2. `ProfilePatch` 使用 `*_add` / `*_remove`；同一值同时出现时 remove 胜出。
3. `budget_max` 使用指针语义：未出现 = 不变，`0` = 清空，正数 = 覆盖。
4. 支持「忘掉预算」「我现在也吃辣」等删除/纠正表达。
5. Repository 使用 `version` + Redis WATCH/CAS，避免多 session 并发覆盖。
6. LLM patch 解析失败只记录 Warn；不得覆盖旧 profile。

`MemoryRepo` 是必选依赖，遵守项目 `Handler → Logic → Repository → Redis` 分层。

### 1.5 Phase 2（本计划不实现，仅预留）

```text
硬偏好 → Hash（每次读取）
软事实 → agent:mem:{userId}:{memId} + 小向量索引
检索 → Hash 与事实 KNN 可并行；不是「Hash 查不到才搜向量」
```

只有 Phase 1 的真实失败案例证明 Hash 无法表达软事实时才进入 Phase 2。

### 1.6 Retrieval eval

**`retrieval.v1.json`：25～35 条，全量人工核验**

```json
{
  "version": "retrieval.v1",
  "dataset_hash": "运行时计算",
  "cases": [
    {
      "id": "r001",
      "split": "test",
      "question": "朝阳区有什么适合约会的浪漫餐厅？",
      "relevant_shop_ids": [5, 7, 15],
      "oracle_filter": {"area": "朝阳区", "typeName": "美食"},
      "tags": ["semantic", "area"],
      "evidence": "seed.sql 中对应店铺及点评包含浪漫/约会语义"
    }
  ]
}
```

要求：

- 题目只能使用种子店铺和点评中存在的事实；无「包间」证据就不标「有包间」。
- LLM 同义改写只能进入 `dev`，不能用来膨胀 `test`。
- 每条 `relevant_shop_ids`、`oracle_filter`、`evidence` 都人工核验。
- `test` 一旦冻结，不因某次策略失败而改标签；发现标注错误需记录变更原因并升级 dataset version。

**指标**

| 指标 | 正确定义 | 用途 |
|------|----------|------|
| HitRate@5 | Top5 是否至少命中一个 relevant | 查询级成功率 |
| Recall@5 | Top5 命中 relevant 数 / relevant 总数 | 多相关项召回 |
| MRR | 第一个 relevant 排名的倒数 | 首个好结果位置 |
| FilterFieldAccuracy | LLM filter 各字段与 oracle 一致率 | filter extractor |
| FilterCompliance@5 | 返回结果满足硬 filter 的比例 | 防过滤回归 |
| InfraErrorRate | embedding / Redis / LLM 基础设施失败率 | 与质量分分离 |

### 1.7 Agent / 记忆 eval（Phase 1 强制）

`agent.v1.json` 在 Agent 实现前创建 10～15 条场景，覆盖：

1. profile 补全区域/预算；
2. 本轮显式条件覆盖 profile；
3. 偏好纠正和清空；
4. 需要 search → detail 或 blogs 的多步问题；
5. 无结果时不编造；
6. 工具失败时降级；
7. 同一工具参数不得循环调用。

主要 grader：

- **Outcome**：结果是否满足约束，最终 profile 是否正确。
- **Groundedness**：回答引用的 shop_id 是否全部来自本轮工具结果。
- **Trajectory**：步数、工具数、重复调用、错误率；不强制唯一工具顺序。
- **Reliability**：5 条关键 case 各跑 3 trials，报告成功率，不用一次运行代表稳定性。
- **成本**：TTFT、总延迟、tokens、tool calls；开放式回答质量仅做人工抽检或可选 LLM rubric。

### 1.8 实验与报告协议

对外谈「提升 X%」必须同时固定并报告：

- dataset version + SHA-256；
- seed 数据版本 / 索引 schema 版本 / Redis image digest；
- embedding 模型、维度、chat/filter 模型；
- retriever、filter mode、TopK、RRF 参数；
- prompt version、temperature、重试策略；
- `n_total`、`n_evaluated`、`n_infra_error`。

质量指标分母使用 `n_evaluated`；基础设施失败单列并使正式 eval 命令非零退出。

百分点示例必须写成：

```text
HitRate@5 42.0% → 61.0%：+19.0pp，相对 +45.2%，n=30
```

禁止写成 `+0.190pp`。

---

## 2. 实现任务（按顺序执行）

> 每个 Task 尽量小步：测 → 实现 → 再测 → commit（仅在你明确要求提交时由执行者 commit）。

---

### Task 0: 修复评测前置正确性问题

**Files:**
- Modify: `internal/config/redis/vector.go`
- Test: `internal/config/redis/vector_test.go`
- Modify: `internal/llm/client.go`
- Test: `internal/llm/client_test.go`
- Modify: `internal/logic/rag_logic.go`
- Modify: `docker-compose.yml`

**Step 1: 写 embedding 维度失败测试**

- `InitShopVectorIndex(..., dim<=0)` 必须返回错误，不能静默回退到另一维度。
- `validateEmbeddingDimension(got, expected)` 对 1024/1536 不一致返回可读错误。

**Step 2: 运行测试确认失败**

```bash
go test ./internal/config/redis/... ./internal/llm/... -count=1
```

Expected: 新测试 FAIL。

**Step 3: 统一维度契约**

- `LLM_EMBEDDING_DIM` / `llm.Config.EmbeddingDim` 是唯一维度来源。
- 删除 Redis 索引层的 1536 fallback；非法维度立即报错。
- `EmbedBatch` 返回后校验每条向量维度，不一致立即失败，禁止写入索引。

**Step 4: 删除错误旁路**

`RAGLogic.IngestShop` 当前无调用方且无法携带 `AvgPrice/Score/Comments/Sold`，从接口与实现中删除。向量写入继续统一走 seed-vector 与 MQ handler 的完整 `ShopVectorDoc`。

**Step 5: 锁定 Redis image**

执行时先查看当前已验证镜像：

```bash
docker image inspect redis/redis-stack-server:latest --format '{{index .RepoDigests 0}}'
```

将 `docker-compose.yml` 的 `latest` 替换为该 tag 或 digest；文档记录选择值。不得猜测一个未验证版本。

**Step 6: 验证**

```bash
go test ./internal/config/redis/... ./internal/llm/... ./internal/logic/... -count=1
go test ./...
```

Expected: PASS。

**Step 7: Commit（若用户要求）**

```bash
git add internal/config/redis/vector.go internal/config/redis/vector_test.go internal/llm/client.go internal/llm/client_test.go internal/logic/rag_logic.go docker-compose.yml
git commit -m "fix(rag): enforce embedding and index consistency"
```

---

### Task 1: 评测目录与 golden set v1

**Files:**
- Create: `rag-evals/golden/retrieval.v1.json`
- Create: `rag-evals/baseline/.gitkeep`
- Create: `rag-evals/reports/.gitignore`（内容：`*` + `!.gitignore`）
- Keep: `script/rag-eval.json`（7 条 smoke set，不作为正式指标）
- Modify: `.gitignore`（忽略 `rag-evals/reports/*`）

**Step 1: 盘点可标注事实**

对照 `script/seed.sql`、店铺字段和 blog 内容建立标注清单。问题中的每个语义条件都必须在数据中有证据。

覆盖：

1. 区域 + 类型；
2. 价格、评分、热度等硬条件；
3. 「约会」「安静」「氛围」等有点评证据的语义；
4. 名称、菜系等 lexical 查询；
5. 无结果 / 条件冲突边界。

**Step 2: 写 `retrieval.v1.json`**

- 总计 25～35 条；`dev` 与冻结的 `test` 都要覆盖主要 tag。
- 每条包含 `id`、`split`、`question`、`relevant_shop_ids`、`oracle_filter`、`tags`、`evidence`。
- LLM 生成的同义改写只能进入 `dev`。

**Step 3: 全量人工核验**

逐条确认 relevant、filter 与 evidence。删除无法由当前 catalog 判断对错的题目，不允许只抽检 20%。

**Step 4: 验证 JSON 可解析**

```bash
python3 -c "import json; json.load(open('rag-evals/golden/retrieval.v1.json')); print('ok')"
```

Expected: `ok`

**Step 5: 记录 dataset hash**

```bash
shasum -a 256 rag-evals/golden/retrieval.v1.json
```

把 hash 写入首次正式 report，不手工写回 golden 文件。

**Step 6: Commit（若用户要求）**

```bash
git add rag-evals/golden/retrieval.v1.json rag-evals/baseline/.gitkeep rag-evals/reports/.gitignore .gitignore
git commit -m "test(eval): add verified retrieval golden set"
```

---

### Task 2: 重构 `cmd/eval-rag` — 正确指标、路径对齐、可复现 report

**Files:**
- Modify: `cmd/eval-rag/main.go`
- Create: `cmd/eval-rag/types.go`
- Create: `cmd/eval-rag/metrics.go`
- Test: `cmd/eval-rag/metrics_test.go`
- Create: `internal/logic/shop_search_logic.go`
- Test: `internal/logic/shop_search_logic_test.go`
- Modify: `internal/repository/interface/vector.go`
- Modify: `internal/repository/vector_repo.go`
- Modify: `internal/logic/rag_logic.go`
- Modify: `cmd/server/main.go`
- Modify: `Makefile`

**Step 1: 先写指标失败测试**

表驱动覆盖：

- Top5 命中 1/3 个 relevant：HitRate=1，Recall=1/3。
- 第 2 位首次命中：MRR=0.5。
- 空 relevant 应拒绝加载为非法 case。
- infra error 不进入质量指标分母。

```bash
go test ./cmd/eval-rag -run TestMetrics -count=1
```

Expected: FAIL。

**Step 2: 实现指标纯函数与 report**

```go
type EvalReport struct {
    DatasetVersion      string       `json:"dataset_version"`
    DatasetSHA256       string       `json:"dataset_sha256"`
    SeedVersion         string       `json:"seed_version"`
    RedisImage          string       `json:"redis_image"`
    IndexSchemaVersion  string       `json:"index_schema_version"`
    Retriever           string       `json:"retriever"`
    FilterMode          string       `json:"filter_mode"`
    EmbeddingModel      string       `json:"embedding_model"`
    EmbeddingDim        int          `json:"embedding_dim"`
    FilterModel         string       `json:"filter_model,omitempty"`
    TopK                int          `json:"top_k"`
    NTotal              int          `json:"n_total"`
    NEvaluated          int          `json:"n_evaluated"`
    NInfraError         int          `json:"n_infra_error"`
    HitRateAtK          float64      `json:"hit_rate_at_k"`
    RecallAtK           float64      `json:"recall_at_k"`
    MRR                 float64      `json:"mrr"`
    FilterFieldAccuracy float64      `json:"filter_field_accuracy,omitempty"`
    FilterComplianceAtK float64      `json:"filter_compliance_at_k,omitempty"`
    PerCase             []CaseResult `json:"per_case"`
}
```

Loader 同时兼容正式 `{version,cases}` schema 与旧 `script/rag-eval.json` 数组；旧格式只允许 `filter-mode=none`，并在报告标记为 smoke，不得写 baseline。

**Step 3: 建立共享 `ShopSearchLogic`**

- 注入 `EmbeddingClient` 与 `VectorRepo`。
- 扩展 `ShopSearchResult` 和 RediSearch `RETURN`，带回 `avg_price/score/comments/sold`，否则无法计算 FilterCompliance 或让 Agent 展示权威元数据。
- `RAGLogic` 改为依赖该接口，不再自行重复 Embed + Search。
- 后续 Agent tool 与 eval 复用同一入口。
- `filter-mode=llm` 必须复用生产 filter extractor；若当前私有方法阻碍复用，抽成可注入的 `FilterExtractor` 接口。

**Step 4: CLI flags**

```text
--test-set=rag-evals/golden/retrieval.v1.json
--split=test
--filter-mode=none        # none | oracle | llm
--retriever=dense         # dense | hybrid
--out=rag-evals/reports/retrieval_latest.json
--baseline=rag-evals/baseline/dense_prod_v1.json
--write-baseline=false
```

**Step 5: 失败口径**

- embedding / Redis / filter LLM 调用失败：记录 per-case error 与 `n_infra_error`。
- 正式 test 只要 `n_infra_error > 0` 就非零退出，防止把服务故障误写成检索变差。
- 仍生成 report，便于排障。

**Step 6: 输出比较**

```text
HitRate@5: 42.0% → 61.0%  (+19.0pp, +45.2% rel)
Recall@5:  35.0% → 52.0%  (+17.0pp, +48.6% rel)
MRR:       0.310 → 0.480
n_total=30 n_evaluated=30 infra_errors=0
filter_mode=llm retriever=dense
```

**Step 7: 运行指标单测**

```bash
go test ./cmd/eval-rag ./internal/logic/... -count=1
```

Expected: PASS。

**Step 8: 写 Makefile 目标**

```makefile
eval-rag-smoke:
	go run ./cmd/eval-rag --test-set=script/rag-eval.json --filter-mode=none --retriever=dense

eval-rag-oracle:
	go run ./cmd/eval-rag --filter-mode=oracle --retriever=dense

eval-rag-prod:
	go run ./cmd/eval-rag --filter-mode=llm --retriever=dense
```

**Step 9: 记录两份 dense baseline**

```bash
go run ./cmd/eval-rag --filter-mode=oracle --retriever=dense \
  --out=rag-evals/reports/dense_oracle.json
go run ./cmd/eval-rag --filter-mode=llm --retriever=dense \
  --write-baseline --out=rag-evals/reports/dense_prod.json
```

Expected: `n_infra_error=0`，并生成 `rag-evals/baseline/dense_prod_v1.json`。

**Step 10: Commit（若用户要求）**

```bash
git add cmd/eval-rag internal/logic/shop_search_logic.go internal/logic/shop_search_logic_test.go internal/repository/interface/vector.go internal/repository/vector_repo.go internal/logic/rag_logic.go cmd/server/main.go Makefile rag-evals/baseline/dense_prod_v1.json
git commit -m "feat(eval): align retrieval evaluation with production"
```

---

### Task 3: Hybrid RRF 检索与固定 A/B

**Files:**
- Create: `internal/rag/rrf.go`
- Test: `internal/rag/rrf_test.go`
- Modify: `internal/repository/interface/vector.go`
- Modify: `internal/repository/vector_repo.go`
- Test: `internal/repository/vector_repo_test.go`
- Modify: `internal/logic/shop_search_logic.go`
- Test: `internal/logic/shop_search_logic_test.go`
- Modify: `cmd/eval-rag/main.go`
- Modify: `Makefile`

**Step 1: 写 RRF 失败测试**

覆盖：

- dense 与 text 同时高排的文档应排第一；
- 只在一侧出现的文档仍可进入结果；
- 相同输入稳定输出；
- 相同分数按 shop_id 固定打破平局；
- 去重后最多返回 TopK。

```go
func TestFuseRRF(t *testing.T) {
    dense := []int64{1, 2, 3}
    text := []int64{2, 4, 1}
    got := FuseRRF(dense, text, 60, 4)
    // shop 2 同时在两路高排，应排第一
}
```

**Step 2: 运行测试确认失败**

```bash
go test ./internal/rag/... -run TestFuseRRF -count=1
```

Expected: FAIL。

**Step 3: 实现客户端 Hybrid**

- 复用现有 `name` / `text_content` TEXT 字段做关键词检索 Top20。
- Dense KNN 取 Top20。
- 两路查询可用 `errgroup.WithContext` 并行。
- `FuseRRF(k=60)` 融合并截断 Top5。
- 不依赖 Redis 8.4 `FT.HYBRID`，保持与当前 Redis Stack 兼容，且 RRF 纯函数可单测。

**Step 4: 接入共享检索策略**

- `ShopSearchLogic` 根据 `RAG_RETRIEVER=dense|hybrid` 选择策略。
- `/api/rag/chat`、Agent `search_shops`、eval 共用该实现。
- 文本查询失败时整次请求报错，不静默退化，避免实验口径漂移；线上是否降级可在 Phase 2 单独设计。

**Step 5: 运行测试**

```bash
go test ./internal/rag/... ./internal/repository/... ./internal/logic/... -count=1
```

Expected: PASS。

**Step 6: 固定变量做 A/B**

```bash
go run ./cmd/eval-rag --filter-mode=oracle --retriever=dense \
  --out=rag-evals/reports/dense_oracle.json
go run ./cmd/eval-rag --filter-mode=oracle --retriever=hybrid \
  --out=rag-evals/reports/hybrid_oracle.json

go run ./cmd/eval-rag --filter-mode=llm --retriever=hybrid \
  --baseline=rag-evals/baseline/dense_prod_v1.json \
  --out=rag-evals/reports/hybrid_prod.json
```

Expected: 三次运行均 `n_infra_error=0`。提升、持平或下降都如实保留，并按 tag 分析失败。

**Step 7: 写已知结果**

将真实 `hybrid_prod.json` 复制为 `rag-evals/baseline/hybrid_prod_v1.json`；README/文档只引用可复现数字。

**Step 8: Commit（若用户要求）**

```bash
git add internal/rag/rrf.go internal/rag/rrf_test.go internal/repository/interface/vector.go internal/repository/vector_repo.go internal/repository/vector_repo_test.go internal/logic/shop_search_logic.go internal/logic/shop_search_logic_test.go cmd/eval-rag Makefile rag-evals/baseline/hybrid_prod_v1.json
git commit -m "feat(rag): add evaluated hybrid retrieval with RRF"
```

---

### Task 4: Redis MemoryRepo + 可纠正 Profile

**Files:**
- Modify: `pkg/utils/redisx/keys.go`
- Create: `internal/memory/types.go`
- Create: `internal/memory/profile.go`
- Create: `internal/memory/extract.go`
- Test: `internal/memory/profile_test.go`
- Test: `internal/memory/extract_test.go`
- Create: `internal/repository/interface/memory.go`
- Create: `internal/repository/memory_repo.go`
- Test: `internal/repository/memory_repo_test.go`
- Modify: `internal/logic/shop_search_logic.go`
- Modify: `internal/handler/rag.go`
- Modify: `cmd/server/main.go`

**Step 1: 写 Profile merge 失败测试**

表驱动覆盖：

- add 新区域但保留旧区域；
- remove 已有 dislike；
- add/remove 冲突时 remove 胜出；
- `BudgetMax=nil` 保持、`0` 清空、正数覆盖；
- 显式请求 filter 不被 profile 覆盖；
- 空请求 filter 由 profile 补齐；
- summary 超长拒绝或截断到 80 字。

**Step 2: 实现纯函数**

```go
type ProfilePatch struct {
    PreferredAreasAdd    []string `json:"preferred_areas_add"`
    PreferredAreasRemove []string `json:"preferred_areas_remove"`
    PreferredTypesAdd    []string `json:"preferred_types_add"`
    PreferredTypesRemove []string `json:"preferred_types_remove"`
    DislikesAdd          []string `json:"dislikes_add"`
    DislikesRemove       []string `json:"dislikes_remove"`
    BudgetMax            *int64   `json:"budget_max"`
    Summary              *string  `json:"summary"`
}
```

实现 `MergeProfile` 与 `MergeFilterWithProfile`；所有数组去重并稳定排序，便于测试与存储。

**Step 3: 写 MemoryRepo 接口**

```go
type MemoryRepo interface {
    LoadProfile(ctx context.Context, userID int64) (memory.Profile, error)
    MergeProfile(ctx context.Context, userID int64, patch memory.ProfilePatch) (memory.Profile, error)
    LoadSession(ctx context.Context, userID int64, sessionID string, limit int) ([]memory.Message, error)
    AppendSession(ctx context.Context, userID int64, sessionID string, messages ...memory.Message) error
}
```

**Step 4: 实现 Redis 原子操作**

- Session：事务 pipeline 执行 `RPUSH / LTRIM / EXPIRE`。
- Profile：`WATCH version`，读取 → Merge → HSET → version+1，冲突最多重试 3 次。
- session TTL=7d；profile TTL=90d。
- 所有 key 通过 `pkg/utils/redisx/keys.go` 生成。

**Step 5: 实现 profile extractor**

- 输入：旧 profile + 本轮用户原话；assistant 输出不得作为偏好事实来源，避免把模型幻觉写回长期记忆。
- 输出：严格 JSON patch，不允许模型直接访问 Redis。
- JSON parse、范围校验、未知字段拒绝做表驱动测试；测试不调用真实 LLM。

**Step 6: 注入现有 RAG**

- `RAGHandler` 从 `middleware.GetUserInfo` 取得 userID。
- `ShopSearchLogic` 先取得本轮 filter，再仅用 profile 补空字段。
- profile 加载失败记录 Warn，继续使用本轮条件；不能使搜索不可用。

**Step 7: 运行测试**

```bash
go test ./internal/memory/... ./internal/repository/... ./internal/logic/... ./internal/handler/... -count=1
```

Expected: PASS。

**Step 8: Commit（若用户要求）**

```bash
git add pkg/utils/redisx/keys.go internal/memory internal/repository/interface/memory.go internal/repository/memory_repo.go internal/repository/memory_repo_test.go internal/logic/shop_search_logic.go internal/handler/rag.go cmd/server/main.go
git commit -m "feat(memory): add correctable profile and session memory"
```

---

### Task 5: 先定义 Agent / 记忆 eval

**Files:**
- Create: `rag-evals/golden/agent.v1.json`
- Create: `cmd/eval-agent/types.go`
- Create: `cmd/eval-agent/graders.go`
- Test: `cmd/eval-agent/graders_test.go`

**Step 1: 写 10～15 条场景**

至少覆盖：

- 无 profile 的普通推荐；
- profile 补全区域/预算；
- 本轮条件覆盖 profile；
- 「忘掉预算」「现在也吃辣」；
- 需要 shop detail；
- 需要 blog 评价；
- 无结果时明确说明；
- 工具错误时不编造；
- 循环调用检测。

**Step 2: 使用 outcome schema**

```json
{
  "id": "a001",
  "setup_profile": {"preferred_areas": ["海淀区"], "budget_max": 100},
  "turns": [
    {"user": "推荐两家安静的咖啡"}
  ],
  "expected": {
    "filter_contains": {"area": "海淀区", "maxPrice": 100},
    "allowed_shop_ids": [8, 12],
    "profile_after": {"preferred_areas": ["海淀区"], "budget_max": 100},
    "max_steps": 3,
    "max_tool_calls": 5
  },
  "tags": ["memory", "search"]
}
```

**Step 3: 写 grader 失败测试**

实现前先测试：

- 回答引用未知 shop_id → groundedness FAIL；
- 超过 max_steps → trajectory FAIL；
- 最终 profile 不符 → outcome FAIL；
- 不同但有效的工具顺序不应被误判。

**Step 4: 实现确定性 grader**

先实现纯函数 grader，不调用 Agent：

- `GradeGroundedness`
- `GradeOutcome`
- `GradeTrajectory`
- `AggregateTrials`

开放式语言质量不做硬门禁。

**Step 5: 运行测试**

```bash
go test ./cmd/eval-agent -count=1
python3 -c "import json; json.load(open('rag-evals/golden/agent.v1.json')); print('ok')"
```

Expected: PASS / `ok`。

**Step 6: Commit（若用户要求）**

```bash
git add rag-evals/golden/agent.v1.json cmd/eval-agent
git commit -m "test(agent): define recommendation agent evaluation"
```

---

### Task 6: Agent 核心 — Tools + Bounded Loop

**Files:**
- Modify: `internal/llm/client.go`
- Test: `internal/llm/client_test.go`
- Create: `internal/agent/types.go`
- Create: `internal/agent/tools.go`
- Test: `internal/agent/tools_test.go`
- Create: `internal/agent/loop.go`
- Test: `internal/agent/loop_test.go`
- Create: `internal/logic/recommend_agent_logic.go`
- Test: `internal/logic/recommend_agent_logic_test.go`

**Step 1: 扩展 LLM tool-call 接口**

避免让 Agent 核心直接依赖 `go-openai` 类型：

```go
type ToolChatClient interface {
    ChatWithTools(ctx context.Context, messages []Message, tools []ToolDefinition) (AssistantTurn, error)
    ChatComplete(ctx context.Context, messages []Message) (AssistantTurn, error)
}
```

`AssistantTurn` 包含 assistant message、tool calls、usage。Agent Phase 1 的最终回答先完整生成并校验 groundedness，再通过 SSE 发出；否则 token 已发送后无法撤回幻觉内容。现有 `/api/rag/chat` 仍保留 `ChatStream`。对所选默认模型做一次 function calling 兼容性 smoke test；不兼容时明确启动失败，不静默当普通聊天。

**Step 2: 写工具 schema 与 executor 测试**

| Tool | 参数 | 实现 |
|------|------|------|
| `search_shops` | `query`, `area?`, `type_name?`, `max_price?` | `ShopSearchLogic` |
| `get_shop` | `shop_id` | `ShopRepo` |
| `list_shop_blogs` | `shop_id`, `limit?` | `BlogRepo` |

测试非法 JSON、负数 shop_id、超大 limit、未知字段、结果截断和 context timeout。

**Step 3: 写 loop 失败测试（fake LLM + fake tools）**

覆盖：

- 一次 search 后生成最终回答；
- search → get_shop 两步；
- 相同 tool+args 重复调用被拒绝；
- 第 4 步被 maxSteps 截断；
- tool error 作为结构化结果反馈模型；
- context cancel 立即停止；
- 最终引用不在 observed shop IDs 中时返回 grounding error。

**Step 4: 实现 bounded loop**

默认配置：

```text
AGENT_MAX_STEPS=3
AGENT_MAX_TOOL_CALLS=5
AGENT_RUN_TIMEOUT=45s
AGENT_TOOL_TIMEOUT=10s
AGENT_MAX_TOOL_RESULT_CHARS=6000
```

每次调用以 `toolName + canonicalJSON(args)` 去重。达到预算后终止，不让模型自行决定继续无限循环。

**Step 5: 实现 RecommendAgentLogic**

调用顺序：

1. MemoryRepo 加载 session/profile；
2. 组装 system + history + current question；
3. 执行 bounded loop；
4. 最终回答通过 groundedness 校验；
5. 成功后写 session；
6. profile extractor 生成 patch 并调用 CAS merge；失败仅 Warn。

**Step 6: 运行单测**

```bash
go test ./internal/llm/... ./internal/agent/... ./internal/logic/... -count=1
```

Expected: PASS，单测不访问真实 LLM/Redis。

**Step 7: Commit（若用户要求）**

```bash
git add internal/llm internal/agent internal/logic/recommend_agent_logic.go internal/logic/recommend_agent_logic_test.go
git commit -m "feat(agent): add bounded recommendation tool loop"
```

---

### Task 7: HTTP/SSE、限流与 OTel 接线

**Files:**
- Create: `internal/handler/agent.go`
- Test: `internal/handler/agent_test.go`
- Modify: `internal/handler/router.go`
- Modify: `cmd/server/main.go`
- Create or Modify: `internal/middleware/agent_rate_limit.go`
- Create: `internal/agent/trace.go`

**Step 1: 写 Handler 失败测试**

覆盖：

- 未登录返回 401；
- 空 question / session_id 返回 400；
- 正常请求返回 `text/event-stream`；
- logic error 发送 `error` 事件；
- 完成发送 `done`；
- 客户端取消后 context 传到 logic。

**Step 2: 实现 SSE 协议**

事件仅包含：

```text
status: searching | reading_shop | reading_blogs
message: answer chunk
done: trace_id + usage summary
error: public error message
```

禁止向客户端透出原始 tool result、profile、prompt 或隐藏思维链。

**Step 3: 注册登录路由与依赖注入**

```go
authGroup.POST("/agent/recommend", middleware.AgentRateLimit(), handlers.Agent.Recommend)
```

`cmd/server/main.go` 按 `Repo → Logic → Handler` 创建依赖。任一必要依赖缺失时启动失败，不在构造函数中偷偷创建全局默认实例。

**Step 4: 添加自定义 OTel spans**

至少记录：

- `agent.run`
- `llm.tool_turn`
- `tool.execute`
- `rag.embed`
- `rag.search`
- `llm.generate`

属性只记录 model、retriever、filter_mode、tool_name、status、candidate_count、steps、tokens、latency；不记录原始问题、完整 profile 或 blog 内容。

**Step 5: 用户级限流**

按 JWT userID 限制 Agent 请求频率；默认值低于普通读接口。返回 429，不消耗 LLM 调用。

**Step 6: 冒烟**

```bash
curl -N -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"question":"海淀安静一点的咖啡","session_id":"s1"}' \
  http://localhost:8088/api/agent/recommend
```

Expected: 有 status/message/done，回答只引用真实 shop ID，Jaeger 可见 agent/tool/search spans。

**Step 7: 运行测试**

```bash
go test ./internal/handler/... ./internal/middleware/... ./internal/agent/... ./internal/logic/... -count=1
```

Expected: PASS。

**Step 8: Commit（若用户要求）**

```bash
git add internal/handler/agent.go internal/handler/agent_test.go internal/handler/router.go internal/middleware/agent_rate_limit.go internal/agent/trace.go cmd/server/main.go
git commit -m "feat(agent): expose traced recommendation SSE endpoint"
```

---

### Task 8: Agent eval、回归、文档与演示

**Files:**
- Modify: `cmd/eval-agent/main.go`
- Test: `cmd/eval-agent/main_test.go`
- Create: `script/agent-demo.sh`
- Create: `doc/AGENT_AND_EVAL.md`
- Modify: `README.md`
- Modify: `Makefile`
- Modify: `memory-bank/activeContext.md`

**Step 1: 接通 Agent eval harness**

- 每个 trial 使用独立 session_id，并先清理/写入 setup profile。
- 捕获完整 transcript、tool calls、observed shop IDs、最终 profile、usage 和 latency。
- 运行确定性 graders。
- 5 条关键 case 各跑 3 trials；其余先跑 1 trial。

**Step 2: 运行 Agent eval**

```bash
go run ./cmd/eval-agent \
  --test-set=rag-evals/golden/agent.v1.json \
  --out=rag-evals/reports/agent_latest.json
```

Expected: report 含 task success、groundedness、trajectory、trial consistency、P50/P95 latency、tokens/tool calls；infra error 单列。

**Step 3: demo 脚本**

1. 登录
2. 第一轮：「我在海淀，人均100以内，喜欢安静咖啡」
3. 第二轮（同 session）：「那就推荐两家」— 应体现记忆/filter
4. 第三轮：「忘掉预算，我现在也可以吃辣」— 应体现纠正与删除
5. 打印脱敏后的 profile 与 trace ID

**Step 4: Makefile**

```makefile
eval-rag-compare:
	go run ./cmd/eval-rag --filter-mode=llm --retriever=hybrid --baseline=rag-evals/baseline/dense_prod_v1.json

eval-agent:
	go run ./cmd/eval-agent --test-set=rag-evals/golden/agent.v1.json

demo-agent:
	chmod +x script/agent-demo.sh && ./script/agent-demo.sh
```

**Step 5: 文档必须包含**

- 线上 RAG 与 Agent 数据流；
- dense vs hybrid 的真实数字、固定条件和失败案例；
- Agent outcome / groundedness / trajectory / 成本指标；
- 为什么不用 Mem0 / Milvus / Eino；
- 为什么 memory 不暴露成模型工具；
- 一个成功 trace 和一个失败 trace 的分析；
- 一条命令如何复现实验；
- 简历 bullet 只填真实结果，不预写提升百分比。

**Step 6: 全量检查**

```bash
go test ./... -count=1
make eval-rag-compare
make eval-agent
make demo-agent
```

Expected: 单测通过；两份 eval 均无 infra error；demo 三轮可复现 profile 继承与纠正。

**Step 7: Commit（若用户要求）**

```bash
git add cmd/eval-agent script/agent-demo.sh doc/AGENT_AND_EVAL.md README.md Makefile memory-bank
git commit -m "docs(agent): add reproducible eval and interview demo"
```

---

## 3. 验收标准（Definition of Done）

- [ ] embedding 维度在配置、API 返回与 Redis 索引之间一致；`latest` 镜像已锁定。
- [ ] `retrieval.v1.json` 有 25～35 条全量人工核验 case，冻结 test split 并记录 SHA-256。
- [ ] eval 同时输出 HitRate@5、真正 Recall@5、MRR、filter 指标和 infra error。
- [ ] `oracle+dense/hybrid` 与 `llm+dense/hybrid` 路径均可运行，报告包含完整实验元数据。
- [ ] Hybrid RRF 与 dense 的真实对比已保存；提升、持平或下降均有失败分析。
- [ ] `agent.v1.json` 有 10～15 条场景，确定性 graders 与关键 case 多 trial 可运行。
- [ ] `/api/agent/recommend` 只有 3 个只读领域工具，并具有 maxSteps、tool budget、timeout、去重与限流。
- [ ] 回答引用的 shop_id 全部来自本轮工具结果。
- [ ] session/profile 有 TTL；偏好支持添加、删除、清空与 CAS 并发合并。
- [ ] OTel 可看到 agent / LLM / tool / embed / search spans，且不记录原始用户隐私内容。
- [ ] `go test ./... -count=1`、`make eval-rag-compare`、`make eval-agent`、`make demo-agent` 均通过。
- [ ] `doc/AGENT_AND_EVAL.md` 可独立复现实验与 demo。
- [ ] **未**引入 Mem0 / Eino / LangGraph / Milvus / BreakTheWaves 整仓。

---

## 4. 建议排期（有 AI 辅助编码时）

| 阶段 | Tasks | 预估（人日，含想清楚+抽检） |
|------|-------|---------------------------|
| P0 前置正确性 | 0 | 0.5～1 |
| P1 Golden + 评测台 | 1–2 | 2～3 |
| P2 Hybrid RRF | 3 | 1～2 |
| P3 MemoryRepo + Profile | 4 | 1.5～2 |
| P4 Agent eval + 核心 | 5–6 | 2～3 |
| P5 接口、Trace、文档 | 7–8 | 1～2 |

合计约 **8～12 人日**。AI 可以缩短编码时间，但人工标注、真实 API 联调、失败分析和把原理讲清楚不能省。

停止条件：上述 DoD 完成后停止扩功能，转入后端主线与面试复盘；不要继续加多 Agent、MCP、向量记忆或新数据库。

---

## 5. 简历 / 面试话术草稿

**后端主线一条：**

> 将点评系统改造为 Nginx 多实例部署，秒杀链路采用 Redis Lua + RocketMQ 事务消息削峰，结合布隆过滤器、限流、延迟关单和唯一索引保证高并发可靠性。

**AI 加分一条（完成后再填真实数字）：**

> 设计可评测店铺推荐 Agent：基于 Redis Stack 实现 dense + lexical 的 Hybrid RRF 检索，以结构化 profile 注入个性化条件，并通过 3-tool bounded loop 完成多步推荐；在人工标注 golden set（n=`真实值`）上将 HitRate@5 从 `A` 提升至 `B`（`+Xpp`），同时评测 groundedness、工具轨迹、延迟与 token 成本。

**追问「为什么组件不多」：**

> 该项目是单 Agent、单领域、硬偏好为主。缓存、检索、会话和 profile 使用同一 Redis 的不同 key/index/TTL，减少运维面；结构化 profile 比 Mem0 更可解释、可纠正、可做确定性测试。只有真实失败证明需要跨 Agent 共享或软事实语义召回时才升级组件。

**追问「提升多少」：**

> 展示 report，先说明 dataset hash、seed/index、模型、filter mode、TopK 和样本数，再报绝对百分点与相对提升；同时展示失败 case。若没有提升，就说明哪类 query 下降以及下一步假设，不编数字。

**追问「为什么一定要 Agent」：**

> 普通推荐仍可走单次 RAG；当问题需要先找候选、再读权威详情或点评，并根据中间结果决定是否继续时才进入 Agent。项目用 Agent eval 对比单次 RAG 的任务成功率、groundedness、工具数和成本；如果收益不成立，就把它降级为固定 workflow。

---

## 6. 执行方式选择

Plan 已保存到 `docs/plans/2026-07-11-recommend-agent-eval.md`。

**可选执行方式：**

1. **Subagent-Driven（本会话）** — 按 Task 派生子代理，任务间复查  
2. **Parallel Session（新会话）** — 用 executing-plans 按文档批量推进  

从 **Task 0（前置正确性）** 开始；随后严格按 1→8 执行，不跳过 Task 5 的 Agent eval 定义。
