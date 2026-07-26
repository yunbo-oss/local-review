# Data Model: Agent Hardening + 002 Closure

**Date**: 2026-07-26 | **Phase**: A 持久化实体 + 运行期结构；B 评测报告结构

## 1. 运行期（Phase A，内存）

### 1.1 EvidenceLedger

| 字段 | 说明 |
|------|------|
| `shops` | map shop_id → ShopEvidence |
| ShopEvidence.discovered_by | `search_shops` / 其他 |
| ShopEvidence.verified | `get_shop` 命中后 true |
| ShopEvidence.fields | 均价/地址/评分/营业时间等 + source |
| ShopEvidence.blog_ids | 有效评价 ID 列表 |

**规则摘要**（详见 [contracts/evidence-ledger.md](./contracts/evidence-ledger.md)）：

- search 成功 → discovered
- get_shop 命中 → verified + fields
- list_blogs 空 → **不**授予可引用；非空可登记 blog_ids（店铺须已 discovered，或策略允许仅 verified）
- 可引用集：至少 discovered（推荐要求 verified 才可陈述强事实）
- 成功推荐：≥1 引用除非 no-result；引用 ⊆ 可引用；强事实 ⊆ fields

### 1.2 RouteDecision

| 字段 | 值 |
|------|-----|
| `route` | `rag_oneshot` \| `agent_multistep` \| `agent_memory` \| `clarify` |
| `reason` | 短枚举：`clear_oneshot` / `needs_detail` / `pref_correct` / `force` / … |
| `confidence` | 规则可为 1.0；后续 embedding 用 |
| `forced` | bool |

### 1.3 RecommendRunOutcome（Harness 返回）

回答、steps、tool_attempts/executed/duplicate、evidence 摘要、cited IDs、profile 变更摘要、tokens、latency、stop_reason、grounding_status、degraded_mode、trace_id、route、非致命后处理错误。

### 1.4 Profile / ProfilePatch / Message

复用 `002` / `internal/memory` 类型；持久化后端改为 MySQL 事实源（见下）。

---

## 2. MySQL 实体（Phase A）

### 2.1 user_agent_profiles

| 列 | 类型 | 说明 |
|----|------|------|
| user_id | PK | |
| profile_json | JSON | areas/types/budget/dislikes/summary |
| version | bigint | 乐观锁，每次成功 merge +1 |
| created_at / updated_at | datetime | |

### 2.2 user_agent_profile_events

| 列 | 说明 |
|----|------|
| id, user_id, run_id | |
| patch_json | 本回合补丁 |
| old_version, new_version | |
| created_at | |

### 2.3 agent_runs

| 列 | 说明 |
|----|------|
| id, trace_id (unique) | |
| user_id, session_id | |
| status | RUNNING / COMPLETED / FAILED / CANCELLED |
| model, policy_version, route, route_reason | |
| steps, tool_attempts, tool_executed, duplicate_rejected | |
| prompt_tokens, completion_tokens, latency_ms | |
| grounding_status, stop_reason, degraded_mode | |
| evidence_summary_json | |
| created_at, completed_at | |

### 2.4 agent_tool_calls

| 列 | 说明 |
|----|------|
| id, run_id, step_no, attempt_no | |
| tool_name, args_hash, args_summary_json | |
| status, error_code, latency_ms, result_count | |
| created_at | |

**不存**：完整 question、完整评价正文、CoT、API key、无界 tool raw。

### 状态机：agent_runs.status

```text
RUNNING → COMPLETED | FAILED | CANCELLED
（超时清理任务可将僵死 RUNNING → FAILED/CANCELLED）
```

**写偏好**：仅 COMPLETED 且 grounding 成功路径；CANCELLED/FAILED 不写。

---

## 3. Redis（Phase A）

| Key | 用途 | TTL |
|-----|------|-----|
| `agent:sess:{uid}:{sid}` | 近期消息 List（可保留） | 7d |
| `agent:profile:cache:{uid}` | profile 缓存（非唯一事实） | ≤90d 或短 TTL |
| 限流 key（如 `agent:rl:{uid}`） | 滑动窗口/令牌 | 窗口对齐 |

遗留 `agent:profile:{uid}` Hash：读取回填 MySQL 后改为缓存或废弃写入。

---

## 4. 评测结构（Phase B）

复用并扩展 `002` AgentCase / AgentReport：

- `summary`: outcome_rate, groundedness_rate, trajectory_pass_rate, trial_consistency (含 pass^k 或等价), p50/p95_latency, avg_tool_calls, avg_tokens
- `comparison`: baseline_path/hash, per-tag Δ outcome, Δ latency/tokens
- `dataset_hash`, `experiment.*`, `n_infra_error`
- 禁止 stub 标记；禁止 dense vs hybrid 字段

## 5. 校验规则汇总

| 规则 | 阶段 |
|------|------|
| remove 优先于同值 add；budget 三态 | A（已有 merge） |
| profile version CAS（MySQL） | A |
| 引用 ⊆ 证据；强事实一致 | A |
| route ∈ 枚举；force 覆盖 | A |
| 关键 case trials≥3 | B |
| infra 不进质量分母 | B |
