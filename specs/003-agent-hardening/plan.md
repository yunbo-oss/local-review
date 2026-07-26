# Implementation Plan: Agent Hardening + 002 Closure

**Branch**: `003-agent-hardening` | **Date**: 2026-07-26 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/003-agent-hardening/spec.md`

**Note**: 设计细节参考 `docs/plans/2026-07-25-local-review-agent-hardening.md`；验收以本规格 SC-A* / SC-B* 为准。`/speckit-tasks` 必须按 **Phase A → Phase B** 分组，且 Phase B 不得阻塞 Phase A 合并。

## Summary

在 `002` 已交付的有界三工具推荐 Agent 之上，完成**可信证据账本、工具层偏好强制补空、工程外壳（Harness/Context/Trace/MySQL 持久化/分布式限流）、生产路径路由**，再接通**真实 `eval-agent` harness、demo、Hybrid 对照与文档**，并收口 `002` 未完成项。

交付硬约束：

```text
Phase A（US1–US6）代码 + 单测/冒烟 → 可合并（SC-A*）
Phase B（US7）评测 + demo + 文档 → 对外「评测闭环」门槛（SC-B*）
```

**不做**：Mem0/LangGraph/多 Agent、默认 HyDE/Multi-Query、dense vs hybrid 对照、开放式文笔 CI 门禁。

## Technical Context

**Language/Version**: Go 1.24+

**Primary Dependencies**: Gin、GORM、go-redis/v9、现有 `internal/llm.ToolChatClient`、`ShopSearchLogic`（Hybrid）、`RecommendAgentLogic` / `AgentHandler`、OpenTelemetry（补齐接线）、现有 graders + `agent.v1.json`

**Storage**:
- Phase A：MySQL 作为长期 profile + AgentRun/ToolCall 事实源；Redis 作 session 窗口、profile Cache Aside、分布式限流；Redis Stack 向量索引不变
- Phase B：评测报告 JSON 文件（`rag-evals/reports/`）；可选报告元数据

**Testing**:
- Phase A：`go test`（evidence/verifier、loop、tools、router、harness、handler SSE、profile CAS、限流）
- Phase B：`make eval-agent`（非 stub）、`make demo-agent`、`make eval-rag-prod` 对照、quickstart 全场景

**Target Platform**: macOS / Linux + Docker Compose（MySQL + Redis Stack）

**Project Type**: 单体 Go Web + CLI（`cmd/server`、`cmd/eval-agent`）+ demo shell

**Performance Goals**: Agent 默认预算不变（3 steps / 5 tools / 10s tool / 45s run）；路由后简单题走 RAG 降本；评测关键题 ≥3 trial，约 1 小时内可跑完正式集

**Constraints**:
- 记忆非模型工具；profile 补空由代码在搜索路径强制执行
- 成功推荐：引用 ⊆ 证据；强事实可校验；空 blogs 不得洗白 ID
- SSE：status/message/done/error；done 含 `trace_id`、`route`；禁 CoT / tool raw / 完整 profile
- Phase A 不以非 stub eval 报告为合并门槛；对外「闭环完成」以 Phase B 为准
- 对照叙事：**Agent vs Hybrid RAG**；分 tag 报 outcome Δ + 代价

**Scale/Scope**: 复用 10～15 条 `agent.v1`；规则路由 3～4 类；MySQL 4 张核心表（run/tool/profile/event）

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*  
*Source: `.specify/memory/constitution.md` v1.1.1*

- [x] **I. Interview-First**: 可讲 EvidenceLedger、记忆非工具、有界 loop、Router、Agent vs Hybrid RAG（outcome + pass^k + 代价）
- [x] **II. Layered Architecture**: Handler → Logic/Harness → Repo 接口 → MySQL/Redis；agent 包仅窄接口；Redis key 进 `redisx`
- [x] **III. Plan → Build → Test**: quickstart 分 Phase A/B；单测先于接线；eval 在 B
- [x] **IV. Explain-as-You-Build**: 实现期按 AGENTS.md §5.8
- [x] **V. Simplicity**: 自研 ledger/harness/router；不上多 Agent/Mem0；Complexity Tracking 见下
- [x] **VI. Knowledge Capture**: Phase A 末更新 Evidence/Router 面试文；Phase B 补评测对照文并勾选面试点

**Post-design re-check (Phase 1)**: 通过 — 未引入第二向量库/编排框架；合约与 data-model 对齐分层；Phase B 依赖 A 的 harness 钩子与 `force_route`。

## Project Structure

### Documentation (this feature)

```text
specs/003-agent-hardening/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md                 # Phase A 验收 → Phase B 验收
├── contracts/
│   ├── evidence-ledger.md        # Phase A
│   ├── recommend-router.md       # Phase A
│   ├── agent-persistence.md      # Phase A
│   ├── recommend-api.md          # Phase A（SSE + route + trace_id）
│   └── eval-and-demo.md          # Phase B
└── tasks.md                      # /speckit-tasks（按 Phase A/B 分组）
```

### Source Code（增量）

```text
# —— Phase A ——
internal/agent/
├── evidence.go / verifier.go     # EvidenceLedger + Fact/Grounding
├── harness.go                    # RecommendAgentHarness
├── context_builder.go
├── loop.go / tools.go            # attempt 预算、严格 JSON、UTF-8 裁剪、ledger 接线
└── trace.go                      # 实际 Start* 调用

internal/logic/
├── recommend_router.go
├── recommend_agent_logic.go      # 瘦身为门面 / 适配
└── shop_search_logic.go          # degraded 模式标记（如需）

internal/model/
├── AgentRun.go / AgentToolCall.go
├── UserAgentProfile.go / UserAgentProfileEvent.go

internal/repository/
├── interface/agent_run.go / agent_profile.go
├── agent_run_repo.go / agent_profile_repo.go
└── memory_repo.go                # Cache Aside 或委托 profile repo

internal/middleware/agent_rate_limit.go   # Redis 原子限流
internal/handler/agent.go | recommend.go  # route + force_route + done 字段
cmd/server/main.go                        # 注入新依赖 + AutoMigrate

# —— Phase B ——
cmd/eval-agent/main.go            # 真实 harness（替换 stub）
script/agent-demo.sh
Makefile                          # eval-agent / demo-agent
doc/AGENT_AND_EVAL.md
rag-evals/baseline/hybrid_prod_v1.json   # 若缺失则录制
rag-evals/reports/agent_latest.json
```

**Structure Decision**: 延续单体分层；agent 包承载 loop/evidence/harness；持久化走 repository；评测保持独立 CLI，避免污染 HTTP 热路径。

## Delivery Phases（供 `/speckit-tasks` 直接映射）

### Phase A — 代码与单测（合并门槛：SC-A*）

| 顺序 | 能力 | Spec US | 主要产物 |
|------|------|---------|----------|
| A1 | EvidenceLedger + Grounding/Fact Guard | US1 | `evidence.go`/`verifier.go` + loop/tools 接线 |
| A2 | 工具层确定性 profile 补空 | US2 | ToolExecutor / adapter + tests |
| A3 | tool-loop 加固 + SSE 不泄漏单测 | US3 | loop/tools + `handler/agent_test.go` |
| A4 | MySQL profile/run/tool + Cache Aside | US4 | models + repos + 迁移读取路径 |
| A5 | RecommendRouter + `force_route` | US5 | `recommend_router.go` + API 元数据 |
| A6 | Harness + ContextBuilder + Trace 接线 + Redis 限流 + degraded | US6 | harness/context/trace/middleware |
| A7 | 补齐 `recommend_agent_logic_test.go`；Knowledge Capture（Evidence/Router） | — | tests + `docs/solutions/interview/` |

**Phase A DoD**: `go test` 相关包绿；冒烟登录推荐；无正式评测数字要求。

### Phase B — 评测与收口（对外门槛：SC-B*）

| 顺序 | 能力 | Spec / 002 | 主要产物 |
|------|------|------------|----------|
| B1 | 真实 `eval-agent` harness | US7 / T042 | CLI：trial、graders、SHA-256、infra 分列 |
| B2 | 录制 `hybrid_prod`（若缺） | T045 | `rag-evals/baseline/hybrid_prod_v1.json` |
| B3 | Agent vs Hybrid 对照节 | US7 / SC-B05 | 报告 comparison + 分 tag Δ |
| B4 | `demo-agent` + `script/agent-demo.sh` | T044 | 三轮记忆演示 |
| B5 | `doc/AGENT_AND_EVAL.md` + README | T046–T047 | 复现步骤与叙事 |
| B6 | 全量回归 + 面试点勾选 | T049–T053 | quickstart 8 场景；002 Interview Value |

**Phase B DoD**: 报告非 stub；关键题 ≥3 trial；demo 抽检过；文档可独立复现。

## Complexity Tracking

| 项 | Why Needed | Simpler Alternative Rejected Because |
|----|------------|-------------------------------------|
| EvidenceLedger（超 ID 集合） | 防空引用/空 blogs 洗白/事实冲突 | 仅 `observed map` 无法表达 verified 字段与事实 |
| MySQL profile + Run | 缓存丢失可恢复、可审计 | Redis-only 不符多实例与排障 |
| RecommendRouter | 成本与难任务成功率 | 全走 Agent 过贵；全走 RAG 难任务失败 |
| 独立 Harness | Logic 过胖、eval 难钩 | 继续堆在 Logic 会阻碍 Phase B 进程内评测 |

## Artifacts

| 文件 | 说明 |
|------|------|
| [research.md](./research.md) | Phase 0 决策 |
| [data-model.md](./data-model.md) | 实体与状态机 |
| [contracts/](./contracts/) | API / ledger / router / persistence / eval-demo |
| [quickstart.md](./quickstart.md) | Phase A 然后 Phase B 验证步骤 |

下一步：`/speckit-tasks`（强制 Phase A 任务组全部可完成后，再列 Phase B 任务组）。
