# Research: Agent Hardening + 002 Closure

**Date**: 2026-07-26 | **Spec**: [spec.md](./spec.md)

## 1. EvidenceLedger vs 仅 observed ID 集合

**Decision**: 引入本轮 `EvidenceLedger`（discovered / verified / fields / blog IDs）；成功推荐要求引用 ⊆ 可引用集，且强事实与 ledger 一致；空 `list_shop_blogs` **不得**单独授予可引用身份；成功推荐默认至少一处引用，除非明确 no-result。

**Rationale**: 002 的 ID ⊆ observed 仍可被「任意 ID + 空评价」污染；业界 research/agent eval 将 groundedness 视为证据支撑，而非仅字符串格式。

**Alternatives**: (a) 仅收紧 observed 写入规则——不够校验价格/地址；(b) LLM-as-judge——非确定、不宜作硬门禁。

## 2. Profile 强制点：prompt vs 工具层

**Decision**: 保留 system 摘要；**搜索执行前**代码强制 `MergeFilterWithProfile`（显式 > 抽取 > profile 仅补空）。

**Rationale**: 模型可忽略摘要；002 已在 RAG 路径验证该优先级，Agent `search_shops` 必须对齐。

**Alternatives**: 记忆工具——002 已否决；仅靠更大 prompt——不可测。

## 3. 持久化：MySQL 事实源 + Redis 缓存

**Decision**: `user_agent_profiles`（version 乐观锁）+ `agent_runs` / `agent_tool_calls` / `profile_events` 落 MySQL；Redis session 窗口与 profile Cache Aside；迁移：MySQL 空则读旧 Redis 并回填。

**Rationale**: 加固计划 ADR；多实例 CAS 与审计需要；不新增 Mongo/图库。

**Alternatives**: 继续 Redis-only——丢失与审计风险；上 Mongo——运维面扩大、YAGNI。

## 4. Harness 与 Logic 边界

**Decision**: `RecommendAgentHarness` 编排 Validate→Context→Loop→Verify→Persist→Trace；Logic/Handler 变薄；eval 可进程内调 Harness。

**Rationale**: Phase B harness 需要稳定 Outcome 钩子；避免 Handler 膨胀。

**Alternatives**: 全堆 Logic——评测与 HTTP 耦合；上 LangGraph——叙事弱、违宪章 V。

## 5. 生产路由

**Decision**: Phase A 规则路由（`rag_oneshot` / `agent_multistep` / `agent_memory`）+ `force_route`；Trace/done 记录 route；embedding 级联留 Phase B 校准后可选。

**Rationale**: Anthropic Routing workflow；工具少无需意图平台；误路由代价不对称，低置信度策略可配置。

**Alternatives**: 客户端自选——现状，成本失控；一上来 LLM 路由——贵且慢。

## 6. Query 改写

**Decision**: Phase A/B **默认不做** HyDE/Multi-Query；会话指代改写仅当 `eval-rag` 证明需要再开。

**Rationale**: 已有 filter 抽取 + Hybrid；改写与路由正交，先证伪再加。

## 7. 评测指标与简历叙事（Phase B）

**Decision**:
- 硬门禁：outcome、groundedness（成功口径）、trajectory；infra 单列
- 对外主数字：分 tag **outcome Δ vs Hybrid RAG** + **pass^k** + P50/token（或 cost per success）
- groundedness 100% 作门禁一句，不作「能力提升」主指标

**Rationale**: Anthropic Demystifying evals；τ-bench pass^k；Agentic RAG 论文均报成功+代价。全局 HitRate 不宜当 Agent 提升。

**Alternatives**: 只报 groundedness 100%——像自证门禁；只报全局成功率——被简单题稀释。

## 8. Phase A / B 分界

**Decision**: A 合并不要求非 stub 评测报告；B 完成前不得宣称评测闭环；`force_route` 与 Harness Outcome 在 A 就绪，供 B 复用。

**Rationale**: 用户策略与规格 Delivery order；避免评测环境阻塞可信代码。

## 9. 限流与 Trace

**Decision**: 用户级限流迁 Redis（Lua/原子）；OTel helpers **必须接线**；SSE `done` 增 `trace_id`；禁止写完整 question/profile/评价正文。

**Rationale**: 002 T041 未接线；多实例进程内 map 可绕过。

## Resolved clarifications

无残留 NEEDS CLARIFICATION。统一入口可采用「保留双路径 + 新增/网关路由」或「`/api/recommend` 统一」——默认 **保留 `/api/rag/chat` 与 `/api/agent/recommend`，新增 Router 用于统一入口或 agent 入口前分流**；`force_route` 必达。
