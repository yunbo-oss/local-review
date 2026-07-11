# 带记忆的推荐 Agent + 可评测智能搜索 — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在现有点评后端与 Naive RAG 之上，落地「可评测的智能搜索」与「带结构化记忆的推荐 Agent」，用 golden set + baseline 量化优化收益，形成秋招可讲的工程闭环。

**Architecture:** 不引入 Mem0 / Eino / Deep Agents / Milvus。Agent 为自研 tool-loop；短时会话与长期 profile 落 Redis（与现有缓存/向量同基础设施、不同 key 空间）；检索侧先扩评测台再做一次可 A/B 的优化（Hybrid 或 Rerank 二选一）；Trace 复用已有 OpenTelemetry。事实向量记忆标为 Phase 2，本计划 Phase 1 只做结构化 Hash profile。

**Tech Stack:** Go 1.24+、Gin、Redis Stack（Hash + 已有 HNSW）、go-openai、现有 `internal/logic/rag_logic.go` / `VectorRepo`、OpenTelemetry、JSON golden set + `cmd/eval-rag` 扩展。

**相关 Skills（执行时参考）：**
- `@superpowers:executing-plans` / `@superpowers:subagent-driven-development`（按任务落地）
- `@skill-creator`（若要把评测流程沉淀为个人 Cursor Skill，可选、非本计划阻塞项）
- 社区参考（指导评测设计，不必整仓接入）：[RAG Evaluation Metrics](https://qaskills.sh/skills/thetestingacademy/rag-evaluation-metrics)、[RAG Regression Testing](https://qaskills.sh/skills/thetestingacademy/rag-regression-testing)

---

## 0. 背景与面试叙事（写进 README / 简历前先对齐）

### 0.1 项目定位（一句话）

> Go 分布式点评（秒杀 / RocketMQ / 多实例）之上的 **可评测店铺推荐 Agent**：结构化用户记忆注入检索、tool 编排多步决策、golden set 量化检索优化。

### 0.2 面试亮点优先级（勿颠倒）

1. **可评测智能搜索**（数字：Recall@5 / MRR，baseline vs 优化）
2. **领域 Agent 编排**（tools + maxSteps + rewrite + OTel span）
3. **记忆服务化**（session + profile Hash，注入 filter）
4. **已有底座**（秒杀事务消息、布隆、MQ 异步更新向量）— 主菜仍是后端

### 0.3 明确不做（YAGNI）

| 不做 | 原因 |
|------|------|
| 换 Milvus / Neo4j / 接入 BreakTheWaves 整仓 | 题材迁移成本高，叙事易变成「抄开源」 |
| Mem0 / LangGraph / Deep Agents / 先上 Eino | 秋招主项目收益低，依赖叙事弱于自研闭环 |
| 完整 Reco 微服务（独立召回池/曝光） | 与点评域重复；用 Agent 工作流表达推荐即可 |
| Phase 1 用户事实向量记忆 | 先 Hash；不够再 Phase 2 |
| 生成侧完整 Ragas CI（可后置） | Phase 1 以检索指标为主；Faithfulness 作可选脚本 |

### 0.4 现状基线（改之前先跑一次）

```bash
# 依赖：MySQL/Redis 已起，seed + seed-vector 完成，LLM_API_KEY 已设
make seed && make seed-redis && make seed-vector
make eval-rag
```

记录当前输出到 `rag-evals/baseline/baseline_metrics.json`（本计划 Task 2 会规范格式）。现有测试集仅约 7 条（`script/rag-eval.json`），**不足以谈提升百分比**。

### 0.5 目标目录结构（完成后）

```text
rag-evals/
  golden/
    retrieval.v1.json          # 检索评测集（≥50）
    agent_memory.v1.json       # Agent/记忆场景（≥15，可后补）
  baseline/
    baseline_metrics.json      # 提交的已知好分数
  reports/                     # gitignore，本地/CI 产物
    latest.json
cmd/
  eval-rag/                    # 扩展：多策略、写 report、对比 baseline
  eval-rag-gate/               # 可选：阈值 + 漂移门禁
internal/
  agent/                       # 新建：tool-loop、skills 路由、trace helper
  memory/                      # 新建：session + profile
  logic/
    rag_logic.go               # 保留；智能搜索入口可委托 agent 或并行存在
    recommend_agent_logic.go   # 新建：推荐 Agent 门面
  handler/
    rag.go                     # 保留 /api/rag/chat（智能搜索）
    agent.go                   # 新建 /api/agent/recommend（SSE）
pkg/utils/redisx/keys.go       # 新增 agent/memory key 常量
```

---

## 1. 架构设计

### 1.1 智能搜索（强化现有 RAG）

```text
POST /api/rag/chat
  question (+ optional filter)
       │
       ├─ [可选] 若已登录：LoadProfile → 合并默认 filter
       ├─ LLM 抽 filter（现有 extractFilterFromQuestion）
       ├─ Embed(question)
       ├─ SearchShops (现有；后续可切 Hybrid/Rerank 策略开关)
       ├─ buildShopContext + ChatStream
       └─ OTel：embed / search / generate spans
```

### 1.2 推荐 Agent（新建）

```text
POST /api/agent/recommend  (AuthRequired)
  { "question": "...", "session_id": "..." }
       │
       ├─ LoadSession + LoadProfile
       ├─ for step < maxSteps (默认 5):
       │     LLM(messages, tools)
       │     tools:
       │       - search_shops(query, filter?)
       │       - get_shop(shop_id)
       │       - list_blogs(shop_id)
       │       - read_memory()
       │       - update_memory(patch)   # 或由服务端在回合结束自动抽取
       │     无 tool_call → 最终回答，break
       ├─ PersistSession
       ├─ 异步/回合末：ExtractAndMergeProfile
       └─ SSE + OTel per tool
```

### 1.3 记忆模型（Phase 1：仅结构化）

**短期会话**

| Key | 类型 | 内容 | TTL |
|-----|------|------|-----|
| `agent:sess:{userId}:{sessionId}` | Redis List 或 String(JSON) | 近 N 轮 `{role,content,ts}`，N=20 | 24h～7d |

**长期 Profile**

| Key | 类型 | 字段 |
|-----|------|------|
| `agent:profile:{userId}` | Hash | `preferred_areas` (JSON array)、`preferred_types`、`budget_max`、`dislike`、`summary`、`updated_at` |

**注入规则**

- `search_shops` 若调用方未传 area/type/maxPrice，用 profile 填默认值（用户本轮明确说出的条件优先覆盖）。
- System prompt 附带一行：`用户偏好摘要：{summary}`。

**抽取规则（回合结束）**

- 小 prompt 输出 JSON patch；空字段不覆盖；数组字段做 union；`budget_max` 取最新非零值。
- 失败则打 Warn，不影响主回答。

### 1.4 Phase 2（本计划不实现，仅预留）

```text
硬偏好 → Hash（始终）
软事实 → agent:mem:{userId}:{memId} + 小向量索引
检索：读 Hash 必做；需要氛围/忌口等再 KNN 记忆（可并行，非「Hash 失败才向量」）
```

### 1.5 评测与量化

**检索集字段（retrieval.v1.json）**

```json
{
  "version": "retrieval.v1",
  "cases": [
    {
      "id": "r001",
      "question": "朝阳区有什么适合约会的浪漫餐厅？",
      "expected_shop_ids": [5, 7, 15],
      "tags": ["semantic", "area"],
      "optional_filter": { "area": "朝阳区", "typeName": "美食" }
    }
  ]
}
```

**指标**

| 指标 | 含义 | 用途 |
|------|------|------|
| Recall@5 | Top5 是否命中任一 expected | 主指标 |
| MRR | 首个命中排名的倒数平均 | 排序质量 |
| FilterHitRate（可选） | 结果是否满足硬过滤 | 防 filter 回归 |

**对比协议（谈「提升 X%」必须遵守）**

- 同一 `retrieval.v1.json`、同一 embedding 模型、同一 TopK=5
- 只改 `retriever_version`（如 `dense-v1` → `dense+rerank-v1`）
- 报告：绝对百分点（pp）+ 相对提升 + n=样本数
- 例：`Recall@5 42% → 61%（+19pp，相对 +45%，n=50）`

**Agent/记忆集（可后做）**

```json
{
  "id": "a001",
  "setup_profile": { "preferred_areas": ["海淀区"], "budget_max": 100 },
  "question": "推荐个咖啡",
  "expect_filter_contains": { "area": "海淀区", "maxPrice": 100 },
  "expected_shop_ids": [8, 12]
}
```

---

## 2. 实现任务（按顺序执行）

> 每个 Task 尽量小步：测 → 实现 → 再测 → commit（仅在你明确要求提交时由执行者 commit）。

---

### Task 1: 评测目录与 golden set v1

**Files:**
- Create: `rag-evals/golden/retrieval.v1.json`
- Create: `rag-evals/baseline/.gitkeep`
- Create: `rag-evals/reports/.gitignore`（内容：`*` + `!.gitignore`）
- Modify: `script/rag-eval.json`（保留兼容，或改为指向 v1 的说明注释于 Makefile）
- Modify: `.gitignore`（忽略 `rag-evals/reports/*`）

**Step 1: 从现有店铺种子归纳题型模板**

题型覆盖（每类至少 5～8 条，总计 ≥50）：

1. 纯区域+类型（「朝阳区火锅」）
2. 价格约束（「海淀人均100以内」）
3. 语义（「适合约会」「安静」「有包间」）— expected 需人工/半自动标注
4. 评分/热度（「西城评分高的咖啡」）
5. 改写扩增：对种子题用 LLM 生成 2～3 条同义问，**expected_shop_ids 不变**，再人工抽检 20%

**Step 2: 写 `retrieval.v1.json`**

- 迁移现有 7 条进 v1，并扩到 ≥50
- 每条带 `id`、`tags`

**Step 3: 抽检**

- 随机 10 条：人工确认 expected 店铺在语义上合理（对照 DB/前端店铺名）

**Step 4: 验证 JSON 可解析**

```bash
python3 -c "import json; json.load(open('rag-evals/golden/retrieval.v1.json')); print('ok')"
```

Expected: `ok`

**Step 5: Commit（若用户要求）**

```bash
git add rag-evals script/rag-eval.json .gitignore
git commit -m "chore(eval): add retrieval golden set v1 (≥50 cases)"
```

---

### Task 2: 扩展 `cmd/eval-rag` — 多策略、写 report、对比 baseline

**Files:**
- Modify: `cmd/eval-rag/main.go`
- Create: `rag-evals/baseline/baseline_metrics.json`（首次跑完后写入）
- Modify: `Makefile`（`eval-rag`、`eval-rag-baseline` 目标）

**Step 1: 定义 report 结构**

```go
type EvalReport struct {
    DatasetVersion   string             `json:"dataset_version"`
    RetrieverVersion string             `json:"retriever_version"`
    EmbeddingModel   string             `json:"embedding_model"`
    TopK             int                `json:"top_k"`
    N                int                `json:"n"`
    RecallAtK        float64            `json:"recall_at_k"`
    MRR              float64            `json:"mrr"`
    PerCase          []CaseResult       `json:"per_case,omitempty"`
    JudgeNote        string             `json:"judge_note"` // 检索评测无 LLM judge
}
```

**Step 2: CLI flags**

```text
--test-set=rag-evals/golden/retrieval.v1.json
--strategy=dense          # dense | dense_profile（后续）| 预留 hybrid | rerank
--out=rag-evals/reports/latest.json
--baseline=rag-evals/baseline/baseline_metrics.json
--write-baseline=false    # true 时覆盖 baseline（仅故意提升质量时）
```

**Step 3: 实现对比输出**

跑完后打印：

```text
Recall@5: 0.420 → 0.610  (+0.190 pp, +45.2% rel)
MRR:      0.310 → 0.480
n=50 strategy=dense vs baseline=dense-v1
```

若 `--baseline` 存在且未 `--write-baseline`，计算 delta；若指定阈值（后续 gate）可非零退出。

**Step 4: 跑 baseline 并落盘**

```bash
go run ./cmd/eval-rag \
  --test-set=rag-evals/golden/retrieval.v1.json \
  --strategy=dense \
  --write-baseline \
  --out=rag-evals/reports/latest.json
```

Expected: 生成 `baseline_metrics.json`，Recall/MRR 为当前真实分数（无论高低都先记下）。

**Step 5: Makefile**

```makefile
eval-rag:
	go run ./cmd/eval-rag --test-set=rag-evals/golden/retrieval.v1.json --strategy=dense

eval-rag-baseline:
	go run ./cmd/eval-rag --test-set=rag-evals/golden/retrieval.v1.json --strategy=dense --write-baseline
```

**Step 6: Commit（若用户要求）**

```bash
git add cmd/eval-rag Makefile rag-evals/baseline/baseline_metrics.json
git commit -m "feat(eval): extend eval-rag with report and baseline compare"
```

---

### Task 3: Redis key 与 Memory 包（Session + Profile）

**Files:**
- Modify: `pkg/utils/redisx/keys.go`
- Create: `internal/memory/types.go`
- Create: `internal/memory/profile.go`
- Create: `internal/memory/session.go`
- Create: `internal/memory/profile_test.go`
- Create: `internal/repository/interface/memory.go`（可选，若希望走 Repo 层）
- Create: `internal/repository/memory_repo.go`（推荐：Logic 不直接碰 Redis）

**Step 1: 常量**

```go
AGENT_SESS_KEY_PREFIX    = "agent:sess:"    // + userId + ":" + sessionId
AGENT_PROFILE_KEY_PREFIX = "agent:profile:" // + userId
AGENT_SESS_TTL           = 7 * 24 * 3600    // 秒
AGENT_SESS_MAX_TURNS     = 20
```

**Step 2: 写失败测试（合并逻辑）**

```go
func TestMergeProfilePatch(t *testing.T) {
    base := Profile{PreferredAreas: []string{"朝阳区"}, BudgetMax: 100}
    patch := ProfilePatch{PreferredAreas: []string{"海淀区"}, BudgetMax: 150}
    got := MergeProfile(base, patch)
    // areas 含朝阳+海淀，budget=150
}
```

**Step 3: 实现 Merge + Redis Load/Save**

- `LoadProfile` / `SaveProfile`
- `AppendSession` / `LoadSession`（超长截断保留最近 N 轮）

**Step 4: 单测**

```bash
go test ./internal/memory/... -count=1
```

Expected: PASS

**Step 5: Commit（若用户要求）**

```bash
git add pkg/utils/redisx/keys.go internal/memory internal/repository/memory_repo.go internal/repository/interface/memory.go
git commit -m "feat(memory): add session and structured user profile on Redis"
```

---

### Task 4: Profile 抽取（LLM）与注入检索

**Files:**
- Create: `internal/memory/extract.go`
- Modify: `internal/logic/rag_logic.go`（或新建 wrapper）：登录用户合并 profile → filter
- Modify: `internal/handler/rag.go`：从 JWT 取 userId（若有）传入 logic
- Test: `internal/memory/extract_test.go`（对 `parseProfilePatchJSON` 做表驱动，不打真 LLM）

**Step 1: 抽取 prompt**

仅输出 JSON：

```json
{"preferred_areas":[],"preferred_types":[],"budget_max":0,"dislike":[],"summary":""}
```

未提及字段空/0；summary ≤ 80 字。

**Step 2: `MergeFilterWithProfile(filter, profile)`**

- filter 已有 area 则不覆盖
- filter 空则用 profile

**Step 3: 单测 merge / parse**

```bash
go test ./internal/memory/... -count=1
```

**Step 4: 接线 ChatWithFilter**

- 签名增加可选 `userID int64`
- userID>0 时 LoadProfile 再 merge

**Step 5: Commit（若用户要求）**

```bash
git add internal/memory internal/logic/rag_logic.go internal/handler/rag.go
git commit -m "feat(rag): inject structured profile into search filters"
```

---

### Task 5: Agent 核心 — tools + loop + OTel

**Files:**
- Create: `internal/agent/tools.go`（tool schema + 执行）
- Create: `internal/agent/loop.go`
- Create: `internal/agent/trace.go`
- Create: `internal/logic/recommend_agent_logic.go`
- Create: `internal/handler/agent.go`
- Modify: `internal/handler/router.go`
- Modify: `cmd/server/main.go`（DI）

**Step 1: Tool 定义（OpenAI function calling）**

| Tool | 参数 | 实现 |
|------|------|------|
| `search_shops` | query, area?, type_name?, max_price? | Embed + VectorRepo.SearchShops + profile 默认 |
| `get_shop` | shop_id | ShopRepo |
| `list_blogs` | shop_id, limit? | BlogRepo |
| `read_memory` | — | LoadProfile + 最近 session 摘要 |
| `update_memory` | patch fields | MergeProfile + Save |

**Step 2: Loop**

```go
const maxSteps = 5
// messages = system(推荐助手+profile summary) + session history + user question
// for i := 0; i < maxSteps; i++ {
//   resp := chat.ChatWithTools(ctx, messages, toolDefs)
//   if len(toolCalls)==0 { stream final; break }
//   for each call: span + execute + append tool result
// }
```

若现有 `ChatClient` 无 tools API：扩展 `internal/llm/client.go` 增加 `ChatWithTools` / 非流式 tool 回合 + 最终 `ChatStream`。

**Step 3: System prompt 要点**

- 先 read_memory 或直接 search（模型自选）
- 推荐必须基于 tool 返回的店铺，禁止瞎编店名
- 信息不足再 search（可改写 query）
- 简洁中文，列出 1～3 家店及理由

**Step 4: 路由**

```go
authGroup.POST("/agent/recommend", h.Agent.Recommend) // SSE，需登录
```

**Step 5: 手工冒烟**

```bash
# 登录拿 token 后
curl -N -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"question":"海淀安静一点的咖啡","session_id":"s1"}' \
  http://localhost:8088/api/agent/recommend
```

Expected: SSE 有 message 事件，回答含真实店名；Redis 出现 `agent:sess:*` / 可能更新 `agent:profile:*`。

**Step 6: Commit（若用户要求）**

```bash
git add internal/agent internal/logic/recommend_agent_logic.go internal/handler/agent.go internal/handler/router.go internal/llm/client.go cmd/server/main.go
git commit -m "feat(agent): add recommend agent tool-loop with memory tools"
```

---

### Task 6: 一次可量化检索优化（二选一，先 Rerank 推荐）

> 业界共识：Rerank 常是最高 ROI 的单点优化。Hybrid（BM25+dense）若 RediSearch 全文已易开也可做；**本计划默认先做 Rerank**，Hybrid 作备选 Task 6b。

**Files:**
- Create: `internal/rag/rerank.go`（或 `internal/logic/rerank.go`）
- Modify: `internal/repository/vector_repo.go` 或 logic：先 KNN 取 Top20，再压到 Top5
- Modify: `cmd/eval-rag/main.go`：`--strategy=rerank`
- Modify: `internal/config` / env：`RAG_RERANK_ENABLED`、`RAG_RERANK_CANDIDATES=20`

**Step 1: Rerank 策略**

- **实现 A（推荐，零新依赖）：** LLM 打分 — 把 query + 候选店 text 交给小模型，返回排序 ID 列表（注意限流与费用，eval 可 sleep）
- **实现 B：** 调用兼容 Rerank API（若已有 DashScope/Cohere key）
- 个人项目选 A 即可讲清原理

**Step 2: 固定对照实验**

```bash
# baseline 已存在
go run ./cmd/eval-rag --strategy=dense --out=rag-evals/reports/dense.json
go run ./cmd/eval-rag --strategy=rerank --out=rag-evals/reports/rerank.json
```

将对比表写入 `rag-evals/reports/COMPARE.md`（可 gitignore 或提交一版面试用）：

| strategy | Recall@5 | MRR | n |
|----------|----------|-----|---|
| dense (baseline) | ? | ? | 50 |
| rerank | ? | ? | 50 |
| delta | +? pp | +? | |

**Step 3: 线上默认**

- env `RAG_RETRIEVER=dense|rerank`，Agent 的 `search_shops` 与 `/api/rag/chat` 共用同一检索函数。

**Step 4: Commit（若用户要求）**

```bash
git add internal/rag/rerank.go internal/logic cmd/eval-rag rag-evals/reports/COMPARE.md
git commit -m "feat(rag): add rerank strategy and eval comparison vs dense baseline"
```

---

### Task 6b（可选）: Hybrid 检索

**仅当 Task 6 的 Rerank 提升不明显（Recall 相对提升 <10%）时做。**

- RediSearch 对 `text_content`/`name` 建 TEXT 字段 + FT.SEARCH 关键词
- 与 KNN 结果 RRF 融合（k=60）
- `--strategy=hybrid` 再跑 eval

---

### Task 7: Agent 记忆评测小集 + Trace 文档化

**Files:**
- Create: `rag-evals/golden/agent_memory.v1.json`（≥15 条）
- Create: `cmd/eval-agent-memory/main.go`（或并入 eval-rag 子命令）
- Create: `doc/AGENT_AND_EVAL.md`（面试讲义：架构、指标、如何复现数字）
- Modify: `memory-bank/activeContext.md`、`memory-bank/systemPatterns.md`

**Step 1: 记忆评测逻辑**

- 写入 setup_profile → 调 search 或 agent → 断言 filter 合并正确 / Recall

**Step 2: 文档必须包含**

- 如何 `make eval-rag` 复现表格
- Agent 序列图
- 为什么不用 Mem0/Milvus（一段 tradeoff）
- 简历 bullet 示例 3 条

**Step 3: 更新 memory-bank**

- activeContext：第四阶段扩展为「可评测 RAG + 推荐 Agent + Profile 记忆」
- systemPatterns：补充 Agent tool-loop、评测基线

**Step 4: Commit（若用户要求）**

```bash
git add rag-evals/golden/agent_memory.v1.json cmd/eval-agent-memory doc/AGENT_AND_EVAL.md memory-bank
git commit -m "docs: agent/eval guide and memory eval set"
```

---

### Task 8: 回归与面试演示脚本

**Files:**
- Modify: `script/rag.sh` 或 Create: `script/agent-demo.sh`
- Modify: `Makefile`：`demo-agent`、`eval-rag-compare`

**Step 1: demo 脚本**

1. 登录  
2. 第一轮：「我在海淀，人均100以内，喜欢安静咖啡」  
3. 第二轮（同 session）：「那就推荐两家」— 应体现记忆/filter  
4. 打印 Redis profile key  

**Step 2: 全量检查**

```bash
go test ./internal/memory/... ./internal/agent/... ./internal/logic/... -count=1
make eval-rag
```

**Step 3: Commit（若用户要求）**

```bash
git add script Makefile
git commit -m "chore: add agent demo and eval-compare make targets"
```

---

## 3. 验收标准（Definition of Done）

- [ ] `retrieval.v1.json` ≥ 50 条，抽检通过  
- [ ] `baseline_metrics.json` 已提交，`make eval-rag` 可对比 delta  
- [ ] 至少一种优化策略（默认 rerank）相对 baseline 有记录在案的数字（提升或持平都要诚实写进 COMPARE.md）  
- [ ] `/api/agent/recommend` SSE 可用；Redis 有 session/profile  
- [ ] Profile 能注入 search filter（单测 + 手工 demo）  
- [ ] OTel 下能看到 search / tool spans（有 collector 时）  
- [ ] `doc/AGENT_AND_EVAL.md` 可独立照着跑出指标  
- [ ] **未**引入 Mem0 / Eino / Milvus / BreakTheWaves 合并  

---

## 4. 建议排期（有 AI 辅助编码时）

| 阶段 | Tasks | 预估（人日，含想清楚+抽检） |
|------|-------|---------------------------|
| P0 评测台 | 1–2 | 1～2 |
| P1 记忆 | 3–4 | 1 |
| P2 Agent | 5 | 1～2 |
| P3 量化优化 | 6 | 1 |
| P4 文档演示 | 7–8 | 0.5～1 |

合计约 **5～7 人日** 可闭环；不要并行开 Hybrid+Mem0+换库。

---

## 5. 简历 / 面试话术草稿

**简历一条：**

> 设计店铺推荐 Agent（tool-loop + Redis 会话/偏好记忆）与可回归 RAG 评测（golden set n≥50，dense vs rerank，Recall@5/MRR 对比）；检索与秒杀链路共用 Redis，店铺变更经 MQ 异步更新向量索引。

**追问「为什么组件不多」：**

> 缓存、向量、会话、profile 同 Redis 不同 key 空间与 TTL；缺的是评测闭环不是中间件。Mem0 适合多 Agent 共享记忆服务，本项目单推荐 Agent 用结构化 profile 更可解释、更好测。

**追问「提升多少」：**

> 打开 COMPARE.md，报 pp 与相对提升、n、embedding/topK 固定条件。

---

## 6. 执行方式选择

Plan 已保存到 `docs/plans/2026-07-11-recommend-agent-eval.md`。

**可选执行方式：**

1. **Subagent-Driven（本会话）** — 按 Task 派生子代理，任务间复查  
2. **Parallel Session（新会话）** — 用 executing-plans 按文档批量推进  

从 **Task 1（golden set）** 开始最稳：先有评测数字，再写 Agent，避免无对照的优化。
