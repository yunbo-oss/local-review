# Agent vs Hybrid RAG：评测叙事怎么讲

**日期**: 2026-07-26 | **相关**: specs/003 US7、`cmd/eval-agent`

## 一句话

Agent 的价值用「分 tag outcome Δ + pass^k + 延迟/token」相对 **Hybrid RAG oneshot** 讲；不要用 dense vs hybrid，也不要把 groundedness 100% 当唯一英雄数字。

## 为何对照 Hybrid 而不是 dense

- 线上检索默认已是 Hybrid；Agent 的 `search_shops` 仍走同一 `ShopSearchLogic`。
- dense vs hybrid 回答的是「检索器选型」，不是「多步工具 + 记忆是否值得」。

## 报告怎么读

| 字段 | 用途 |
|------|------|
| `tag_outcome_rates` / `comparison.per_tag_outcome_delta` | 主叙事：记忆纠偏好、多步等 tag |
| `pass_at_k_rate` | 跨 trial 稳定性 |
| `p50_latency_ms` / `avg_tokens` | 成本与体验 |
| `groundedness_rate` | 门禁/配角：成功路径有据可查 |
| `n_infra_error` | 与质量分母分离 |

## 面试追问

- **force_route？** 评测可强制 `agent_multistep`，排除路由噪声，专测 Agent 能力。
- **fake 模式？** 只验证 harness/报告契约；简历数字必须来自 `--mode=inprocess` + 真实基线。
