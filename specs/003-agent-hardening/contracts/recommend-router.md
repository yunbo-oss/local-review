# Contract: RecommendRouter

**Phase**: A | **Consumers**: 统一推荐入口 / Agent 入口前、eval `force_route`

## Purpose

路径/复杂度分流：简单 → Hybrid RAG oneshot；复杂/记忆 → Agent。非通用 NLU。

## Routes

| route | 下游 |
|-------|------|
| `rag_oneshot` | 现有 RAG 推荐流水线（Hybrid + 生成） |
| `agent_multistep` | RecommendAgentHarness |
| `agent_memory` | RecommendAgentHarness（记忆相关） |
| `clarify` | 可选短澄清；A 期可映射为 `rag_oneshot` 或 agent |

## Input

- `user_id`, `session_id`, `question`
- 可选：已加载 profile 摘要特征、是否同 session 多轮
- 可选：`force_route`（请求字段或 Header）

## Output

```json
{
  "route": "agent_multistep",
  "reason": "needs_detail",
  "confidence": 1.0,
  "forced": false
}
```

## Phase A 规则（最小集）

1. `force_route` 合法 → 直接采用，`forced=true`
2. 纠正/清空偏好话术 → `agent_memory`
3. 评价/对比/详情/营业时间等信号 → `agent_multistep`
4. 否则 → `rag_oneshot`

## Guarantees

- 路由在主 LLM Agent loop 之前完成（RAG 路径仍可有自己的 filter 抽取调用）
- 决策写入 Run Trace 与 SSE `done`
- 表驱动单测覆盖：单轮清晰 / 纠偏好 / 要评价 / force 覆盖
