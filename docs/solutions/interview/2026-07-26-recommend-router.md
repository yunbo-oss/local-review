# RecommendRouter：生产路径分流（简单→RAG / 复杂→Agent）

**日期**: 2026-07-26 | **相关**: specs/003-agent-hardening US5

## 问题

推荐既有 Hybrid RAG oneshot，也有多步 Agent。若全部走 Agent，延迟与 token 成本高；若全部走 RAG，记忆纠偏好与评价对比做不好。需要**路径/复杂度路由**，而不是完整 NLU。

## 方案

规则路由器 `RecommendRouter`（`internal/logic/recommend_router.go`）：

| 信号 | 路由 |
|------|------|
| `force_route` 合法 | 强制覆盖 |
| 纠正/清空偏好话术 | `agent_memory` |
| 评价/对比/详情 | `agent_multistep` |
| 过短/模糊 | `clarify`（A 期映射 `rag_oneshot`） |
| 否则 | `rag_oneshot` |

统一入口 `POST /api/recommend` 先 Route 再分流；Agent 路径仍写 `agent_runs.route` 与 SSE `done.route`。

## 面试要点

1. **为什么不用大模型做路由？** A 期可解释、可测、延迟稳定；embedding 级联可后置。
2. **force_route 的作用？** 评测对照与排障：同一题强制走 RAG vs Agent。
3. **和意图分类的区别？** 目标是选执行路径（成本/能力），不是细粒度 slot filling。
