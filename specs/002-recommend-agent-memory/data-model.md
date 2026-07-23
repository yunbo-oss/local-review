# Data Model: Recommend Agent with Correctable Memory

**Feature**: `002-recommend-agent-memory`  
**Date**: 2026-07-23

本规格新增**记忆、Agent 运行、Agent 评测**数据；店铺/点评/向量文档沿用 001 与现有 MySQL/Redis 模型。

## 1. UserProfile（长期偏好）

Redis Hash：`agent:profile:{userId}`（key 常量见 `pkg/utils/redisx`）。

| 字段 | 类型 | 约束 |
|------|------|------|
| `preferred_areas` | []string | 去重、稳定排序存储（如 JSON 或分隔符，实现期统一） |
| `preferred_types` | []string | 同上 |
| `budget_max` | *int64 | nil/缺失表示未设；0 表示用户清空上限 |
| `dislikes` | []string | 如「太辣」 |
| `summary` | string | 自然语言摘要，≤80 字（超长拒绝或截断） |
| `version` | int64 | CAS；每次成功 merge +1 |
| `updated_at` | int64 | Unix 秒 |

**TTL**: 90 天，读写后刷新。

**State transitions**:
- 空用户 → 首次 merge 创建 version=1
- 正常 patch merge → version++
- CAS 冲突 → 重试（≤3）→ 仍失败返回 error，旧 profile 保留
- 解析失败 → 不写入

**Relationships**: 1 User → 1 Profile；被 `MergeFilterWithProfile` 只读消费。

---

## 2. SessionMessage / SessionTranscript（短期会话）

Redis List：`agent:sess:{userId}:{sessionId}`。

| 字段（Message） | 类型 | 约束 |
|-----------------|------|------|
| `role` | string | `user` \| `assistant` |
| `content` | string | 非空 |
| `ts` | int64 | Unix 秒 |

**Retention**: 最多 20 条（`LTRIM`）；TTL 7 天；加载后按上下文预算再裁剪。

**Write policy**: 推荐**成功**结束后 append 本轮 user+assistant；断连/失败/grounding 失败不 append（与 profile 写入策略一致）。

---

## 3. ProfilePatch（偏好补丁）

由 `ExtractProfilePatch` 产出，**非**持久化实体（传输/合并中间态）。

| 字段 | 语义 |
|------|------|
| `preferred_areas_add` / `_remove` | 区域增删 |
| `preferred_types_add` / `_remove` | 品类增删 |
| `dislikes_add` / `_remove` | 忌口增删 |
| `budget_max` | *int64：nil=不变，0=清空，>0=覆盖 |
| `summary` | *string：nil=不变 |

**Merge rules**（`MergeProfile`）:
1. 对每集合字段：先 apply remove，再 apply add，去重排序
2. 同值同时 add+remove → remove 胜出
3. `budget_max` 指针三态

---

## 4. FilterConditions + Profile（检索解析）

沿用 001 `FilterConditions` / `VectorSearchFilter`；002 增加 **profile 补空**：

**优先级**: 本轮显式 > LLM 抽取 > profile 默认（仅补空字段）。

Profile 字段映射示例：
- `preferred_areas[0]` → 默认 `area`（若未显式/抽取）
- `preferred_types[0]` → 默认 `typeName`
- `budget_max` → 默认 `maxPrice`

---

## 5. DomainTool / ToolCall（领域工具）

逻辑实体，不持久化。

| Tool | 参数 | 校验 |
|------|------|------|
| `search_shops` | `query`, `area?`, `type_name?`, `max_price?` | query 非空；价格≥0 |
| `get_shop` | `shop_id` | int64 > 0 |
| `list_shop_blogs` | `shop_id`, `limit?` | shop_id > 0；limit 默认与上限（如 10） |

**Dedupe key**: `toolName + canonicalJSON(args)` — 同 run 内唯一。

**Tool result**: 字符串 JSON/文本，≤6000 字符（超出截断并标记）。

---

## 6. RecommendRun（推荐运行）

单次 `POST /api/agent/recommend` 的运行记录（报告/ trace 用，可选落盘到 eval report）。

| 字段 | 说明 |
|------|------|
| `user_id`, `session_id` | 标识 |
| `question` | 用户问题 |
| `steps`, `tool_calls` | 计数 |
| `tool_trace` | [{name, args, status, latency_ms}] |
| `observed_shop_ids` | 本轮工具见过的 shop id 集合 |
| `answer` | 最终回答（含 `[shop:{id}]` 引用） |
| `groundedness_ok` | bool |
| `profile_before` / `profile_after` | eval 用 |
| `usage` | tokens, latency_ms |
| `error` | infra / budget / grounding |

**Budget fields**: `max_steps`, `max_tool_calls`, `run_timeout`, `tool_timeout`.

---

## 7. AgentCase（Agent 评测场景）

文件：`rag-evals/golden/agent.v1.json`。

| 字段 | 类型 | 约束 |
|------|------|------|
| `id` | string | 唯一，如 `a001` |
| `split` | string | 可选 `dev` \| `test` |
| `setup_profile` | object | 初始 Profile 快照（可空） |
| `turns` | [{user}] | 至少 1 轮 user |
| `expected` | object | 见下 |
| `tags` | []string | memory / multi_step / no_result / tool_error / anti_loop |
| `trials` | int | 默认 1；关键 case ≥3 |
| `evidence` | string | 非空，人工核验依据 |

**expected 子对象**:

| 字段 | 说明 |
|------|------|
| `filter_contains` | 最终检索/filter 应包含的硬约束子集 |
| `allowed_shop_ids` | outcome 允许出现的 shop（可空表示仅检查不编造） |
| `forbidden_shop_ids` | 不得引用 |
| `profile_after` | 回合后 profile 期望（部分字段即可） |
| `max_steps`, `max_tool_calls` | trajectory 上限 |
| `expect_no_results` | bool：应明确无合适店 |
| `expect_groundedness` | bool，默认 true |

**文件级**: `version`（如 `agent.v1`）、`cases`（10～15）。

---

## 8. AgentReport（Agent 评测报告）

| 字段 | 说明 |
|------|------|
| `version`, `dataset_hash` | 场景集 |
| `experiment` | 模型、budget 参数、timestamp |
| `summary` | outcome_rate, groundedness_rate, trajectory_pass_rate, trial_consistency, p50/p95_latency, avg_tool_calls, avg_tokens |
| `n_total`, `n_evaluated`, `n_infra_error` | 分母分离 |
| `cases` | 逐题逐 trial 明细 |
| `comparison` | 可选：vs hybrid_prod 引用（baseline path + fingerprint） |

---

## 9. Redis Key Registry（新增）

| Key pattern | 类型 | TTL |
|-------------|------|-----|
| `agent:sess:{userId}:{sessionId}` | List | 7d |
| `agent:profile:{userId}` | Hash | 90d |

MUST 经 `pkg/utils/redisx/keys.go` 生成，禁止散落字符串。

---

## 10. Relationships (overview)

```text
User ──1:1── UserProfile
User ──1:N── SessionTranscript (per sessionId)

RecommendRun ──uses── ShopSearchLogic / ShopRepo / BlogRepo
RecommendRun ──reads/writes── UserProfile, SessionTranscript

AgentCase ──exercises── RecommendRun + UserProfile
AgentReport ──aggregates── RecommendRun results
```

## Validation rules (cross-cutting)

- AgentCase 加载：`evidence` 非空；`turns` 非空
- 引用 shop_id 必须存在于 seed catalog（outcome 校验）
- 标注错误：升级 `agent.v2` + 变更说明，禁止为刷分改 expected
- Profile extract 失败不得 clobber 旧 profile
- 未完成回合不得 persist profile patch
