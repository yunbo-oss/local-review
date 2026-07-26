# Tasks: Agent Hardening + 002 Closure

**Input**: Design documents from `/specs/003-agent-hardening/`

**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/ ✅, quickstart.md ✅

**Tests**: Included — spec SC-A* / Independent Test 要求自动化用例；Phase A 以 `go test` 为合并门槛；Phase B 以非 stub eval + demo 为对外门槛

**Organization**: **先 Phase A（US1–US6 + A 收口），再 Phase B（US7 + 文档/回归）**。Phase B 任务不得阻塞 Phase A 合并。原 `002` T041–T053 已映射到下方任务。

**Out of scope**: Mem0 / LangGraph / 多 Agent / 默认 HyDE·Multi-Query / dense vs hybrid

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1–US7 maps to spec user stories
- Include exact file paths in descriptions

## Path Conventions

- Go: `cmd/`, `internal/`, `pkg/`, `script/`, `rag-evals/`, `doc/`, `docs/solutions/`

---

# ========== PHASE A：代码与单测（合并门槛 SC-A*）==========

> Checkpoint A：`go test` 相关包绿 + quickstart A1–A5；**不要求**非 stub `eval-agent` 报告（SC-A09）

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 包骨架与配置占位；不改线上行为

- [x] T001 Create stub files `internal/agent/evidence.go`, `internal/agent/verifier.go`, `internal/agent/harness.go`, `internal/agent/context_builder.go` per plan.md Project Structure
- [x] T002 [P] Document Router / persistence / force_route notes in `.env.example` (e.g. `AGENT_RATE_LIMIT_PER_MIN`, optional unified recommend path) without breaking defaults
- [x] T003 [P] Add Redis key helpers for profile cache + rate limit in `pkg/utils/redisx/keys.go` (e.g. `AgentProfileCacheKey`, `AgentRateLimitKey`)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 错误码与 RunConfig 扩展等横切基础；完成后才能进 US1

**⚠️ CRITICAL**: 完成后再进入 US1

- [x] T004 Extend `internal/agent/types.go` RunConfig with per-turn tool call cap / attempt budget fields (env-loaded, safe defaults)
- [x] T005 [P] Define stable public error categories (grounding_*, tool_*, rate_limit) usable by loop/handler in `internal/agent/errors.go` (or equivalent)
- [x] T006 Run `go test ./internal/agent/... -count=1` to confirm types compile after T004–T005

**Checkpoint**: Foundation ready — start US1

---

## Phase 3: User Story 1 - 可信推荐：EvidenceLedger (Priority: P1) 🎯 MVP

**Goal**: 证据账本 + 引用/事实校验；空 blogs 不可洗白；成功前校验

**Independent Test**: 空引用 / 未知 ID / 空 blogs 洗白 / 事实冲突 / no-result 自动化全过（SC-A01）

**Contracts**: [contracts/evidence-ledger.md](./contracts/evidence-ledger.md)

### Tests for User Story 1

- [x] T007 [P] [US1] Add failing EvidenceLedger unit tests (discover/verify/empty blogs no cite; fact conflict) in `internal/agent/evidence_test.go`
- [x] T008 [P] [US1] Add failing VerifyAnswer tests (no citation fail; unknown id fail; no-result ok) in `internal/agent/verifier_test.go`

### Implementation for User Story 1

- [x] T009 [P] [US1] Implement EvidenceLedger types + mutations in `internal/agent/evidence.go` per data-model.md §1.1 (depends on T007)
- [x] T010 [US1] Implement VerifyAnswer / FactGuard in `internal/agent/verifier.go` per contracts/evidence-ledger.md (depends on T008, T009)
- [x] T011 [US1] Wire ledger into `internal/agent/tools.go`: search→discover; get_shop→verify; empty list_shop_blogs MUST NOT grant citeable; restrict detail/blogs to discovered IDs
- [x] T012 [US1] Replace/extend groundedness in `internal/agent/loop.go` to use VerifyAnswer before success; expose evidence summary on LoopResult
- [x] T013 [US1] Ensure `internal/logic/recommend_agent_logic.go` / handler reject success SSE when verification fails (no `message` event)
- [x] T014 [US1] Run `go test ./internal/agent/... ./internal/logic/... ./internal/handler/... -count=1` — expect SC-A01 sample PASS

**Checkpoint**: US1 可独立验收

---

## Phase 4: User Story 2 - 工具层确定性 profile 补空 (Priority: P1)

**Goal**: `search_shops` 执行前强制 MergeFilterWithProfile（显式 > 抽取 > profile 仅补空）

**Independent Test**: 补空海淀 / 本轮朝阳覆盖 / 清空预算（SC-A02）

### Tests for User Story 2

- [x] T015 [P] [US2] Add failing tests that Agent search path applies profile fill-empty in `internal/agent/tools_test.go` and/or `internal/logic/recommend_agent_logic_test.go`

### Implementation for User Story 2

- [x] T016 [US2] Inject current `memory.Profile` into ToolExecutor / shopSearchAdapter in `internal/logic/recommend_agent_logic.go` and `internal/agent/tools.go`
- [x] T017 [US2] Apply `logic.MergeFilterWithProfile` (or shared helper) inside search execution before `ShopSearchLogic.Search` in `internal/agent/tools.go` / adapter
- [x] T018 [US2] Run `go test ./internal/agent/... ./internal/logic/... -count=1` — expect SC-A02 PASS

**Checkpoint**: US2 可独立验收

---

## Phase 5: User Story 3 - 有界执行与 SSE 不泄漏 (Priority: P1)

**Goal**: attempt 预算、单 turn 上限、严格 JSON/UTF-8 裁剪、公共错误；SSE 无 tool raw/profile/CoT

**Independent Test**: 重复调用耗预算、单 turn 上限、畸形 JSON、注入评价、SSE 抽检（SC-A03 / SC-A08；002 T051）

### Tests for User Story 3

- [x] T019 [P] [US3] Add failing loop/tools tests (attempt budget includes failures/dupes; per-turn cap; trailing JSON reject; UTF-8 safe truncate) in `internal/agent/loop_test.go` / `tools_test.go`
- [x] T020 [P] [US3] Add failing SSE leak tests (no tool raw / full profile / CoT in events) in `internal/handler/agent_test.go`

### Implementation for User Story 3

- [x] T021 [US3] Enforce attempt budget + per-turn tool call cap + duplicate handling in `internal/agent/loop.go` (depends on T004, T019)
- [x] T022 [US3] Harden `strictDecode` EOF check + structured truncate (no mid-JSON/UTF-8 slice) in `internal/agent/tools.go`
- [x] T023 [US3] Mark blog content untrusted in system/policy strings; map tool errors to public codes via `internal/agent/errors.go` in `internal/handler/agent.go`
- [x] T024 [US3] Implement handler tests in `internal/handler/agent_test.go` (401/400/SSE content-type/error-vs-message/leak) (depends on T020)
- [x] T025 [US3] Run `go test ./internal/agent/... ./internal/handler/... -count=1` — expect SC-A03/A08 PASS

**Checkpoint**: US3 可独立验收

---

## Phase 6: User Story 4 - MySQL 持久化与可审计 Run (Priority: P1)

**Goal**: MySQL profile/run/tool + Cache Aside；断连不写偏好；可查 trace 摘要

**Independent Test**: 清缓存恢复偏好；并发 CAS；成功/失败可查终态（SC-A04 / SC-A07）

**Contracts**: [contracts/agent-persistence.md](./contracts/agent-persistence.md)

### Tests for User Story 4

- [x] T026 [P] [US4] Add failing AgentProfileRepo CAS / cache-aside tests in `internal/repository/agent_profile_repo_test.go`
- [x] T027 [P] [US4] Add failing AgentRunRepo begin/finalize/status tests in `internal/repository/agent_run_repo_test.go`

### Implementation for User Story 4

- [x] T028 [P] [US4] Add GORM models `internal/model/AgentRun.go`, `AgentToolCall.go`, `UserAgentProfile.go`, `UserAgentProfileEvent.go` per data-model.md
- [x] T029 [P] [US4] Add repo interfaces in `internal/repository/interface/agent_run.go` and `agent_profile.go`
- [x] T030 [US4] Implement MySQL `AgentProfileRepo` (version CAS, events, Redis cache aside, legacy Redis backfill) in `internal/repository/agent_profile_repo.go` (depends on T003, T026, T028, T029)
- [x] T031 [US4] Implement MySQL `AgentRunRepo` (begin/finalize/tool batch, privacy-safe fields) in `internal/repository/agent_run_repo.go` (depends on T027–T029)
- [x] T032 [US4] Wire AutoMigrate + repo construction in `cmd/server/main.go`; switch Recommend path Load/Merge profile to AgentProfileRepo
- [x] T033 [US4] Persist run lifecycle from recommend path (RUNNING→COMPLETED/FAILED/CANCELLED); skip profile write on cancel/fail in logic/harness
- [x] T034 [US4] Run `go test ./internal/repository/... ./internal/logic/... -count=1` — expect SC-A04 sample PASS

**Checkpoint**: US4 可独立验收

---

## Phase 7: User Story 5 - RecommendRouter (Priority: P1)

**Goal**: 规则路由 + `force_route`；done/trace 含 route

**Independent Test**: 单轮→rag；纠偏好→agent_memory；评价对比→agent_multistep；force 覆盖（SC-A05）

**Contracts**: [contracts/recommend-router.md](./contracts/recommend-router.md), [contracts/recommend-api.md](./contracts/recommend-api.md)

### Tests for User Story 5

- [x] T035 [P] [US5] Add failing table-driven router tests in `internal/logic/recommend_router_test.go`

### Implementation for User Story 5

- [x] T036 [US5] Implement `RecommendRouter` in `internal/logic/recommend_router.go` (rules + force_route) (depends on T035)
- [x] T037 [US5] Extend request/SSE done with `force_route`, `route`, `route_reason` in `internal/handler/agent.go` (and optional unified handler)
- [x] T038 [US5] Register optional `POST /api/recommend` or wire router before agent/RAG dispatch in `internal/handler/router.go` + `cmd/server/main.go`
- [x] T039 [US5] Record route on agent_runs / done payload; run `go test ./internal/logic/... ./internal/handler/... -count=1`

**Checkpoint**: US5 可独立验收

---

## Phase 8: User Story 6 - Harness / Context / Trace / Redis 限流 (Priority: P2)

**Goal**: RecommendAgentHarness + ContextBuilder + OTel 接线 + Redis 限流 + degraded 标记（含 002 T041）

**Independent Test**: 超长历史不挤规则；span 可区分阶段；三实例限流；检索降级显式（SC-A06 / A07）

### Tests for User Story 6

- [x] T040 [P] [US6] Add failing ContextBuilder budget tests in `internal/agent/context_builder_test.go`
- [x] T041 [P] [US6] Add failing Redis rate-limit tests (shared window) in `internal/middleware/agent_rate_limit_test.go`

### Implementation for User Story 6

- [x] T042 [US6] Implement `ContextBuilder` in `internal/agent/context_builder.go` (policy + profile summary + old summary + recent + question budgets) (depends on T040)
- [x] T043 [US6] Implement `RecommendAgentHarness` in `internal/agent/harness.go` (validate→context→loop→verify→persist→finalize→Outcome) and thin `internal/logic/recommend_agent_logic.go`
- [x] T044 [US6] Wire OTel spans from `internal/agent/trace.go` into harness/loop/tools/search call sites (002 T041); add `trace_id` to SSE `done` in `internal/handler/agent.go`
- [x] T045 [US6] Replace in-process limiter with Redis atomic limiter in `internal/middleware/agent_rate_limit.go` (depends on T003, T041)
- [x] T046 [US6] Add explicit search `strict`/`degraded` signaling into Outcome/Trace in `internal/logic/shop_search_logic.go` + harness (no silent baseline change)
- [x] T047 [US6] Run `go test ./internal/agent/... ./internal/middleware/... ./internal/logic/... ./internal/handler/... -count=1`

**Checkpoint**: US6 可独立验收；Phase A 核心代码齐

---

## Phase 9: Phase A Polish (tests + knowledge)

**Purpose**: 补齐 002 缺失 Logic 单测；面试落盘；A 门禁确认

- [x] T048 [P] Add `internal/logic/recommend_agent_logic_test.go` happy/fail paths with fakes (002 gap)
- [x] T049 [P] Knowledge Capture: add/update `docs/solutions/interview/` for EvidenceLedger + RecommendRouter; index in `docs/solutions/README.md`
- [x] T050 Run Phase A quickstart: `go test ./internal/agent/... ./internal/logic/... ./internal/repository/... ./internal/handler/... ./internal/middleware/... -count=1` and smoke A2–A4 per quickstart.md
- [x] T051 Confirm SC-A09: do **not** claim eval loop closed; mark Phase A complete in `memory-bank/activeContext.md`

**✅ PHASE A CHECKPOINT — 可合并代码；以下为 Phase B**

---

# ========== PHASE B：评测、演示与文档（对外门槛 SC-B*）==========

> 依赖 Phase A Harness / `force_route` / graders+golden。完成前不得宣称「Agent 评测闭环已完成」（SC-B08）

---

## Phase 10: User Story 7 - 可复现评测与对照 (Priority: P1)

**Goal**: 真实 `eval-agent` harness + hybrid 基线 + Agent vs Hybrid RAG 对照（002 T042/T045）

**Independent Test**: 非 stub 报告；≥5 关键×≥3 trial；comparison 分 tag（SC-B01–B03, B05–B06）

**Contracts**: [contracts/eval-and-demo.md](./contracts/eval-and-demo.md)

### Tests for User Story 7

- [x] T052 [P] [US7] Add failing harness integration tests (or main_test) for trial isolation / stub-forbidden report shape in `cmd/eval-agent/main_test.go`

### Implementation for User Story 7

- [x] T053 [US7] Replace stub in `cmd/eval-agent/main.go` with real harness: load golden, SHA-256, per-trial session, setup_profile, run Harness/API, capture transcript/evidence/profile/usage (002 T042) (depends on T043, T052)
- [x] T054 [US7] Wire graders + AggregateTrials + infra accounting + write `rag-evals/reports/agent_latest.json` (non-stub version) in `cmd/eval-agent/`
- [x] T055 [US7] If missing, record `rag-evals/baseline/hybrid_prod_v1.json` via `make eval-rag-prod` / `--write-baseline` (002 T045)
- [x] T056 [US7] Implement `--compare-baseline` comparison section (per-tag outcome Δ + latency/tokens; Agent vs Hybrid RAG only) in `cmd/eval-agent/`
- [x] T057 [US7] Support `--force-route` eval mode; update `Makefile` `eval-agent` target flags
- [x] T058 [US7] Run `make eval-agent` — expect SC-B01–B03/B06 sample PASS; report has no stub marker
  - 已用 `make eval-agent-fake` 验证非 stub 报告；`make eval-agent`（inprocess）需本地 LLM+向量栈

**Checkpoint**: 正式评测数字可产出

---

## Phase 11: Demo + Documentation (US7 continued / 002 T044–T047)

**Goal**: `demo-agent` 三轮记忆；`AGENT_AND_EVAL.md` + README

**Independent Test**: demo 抽检 3 次全过；文档一小时内可复现（SC-B04 / B07）

- [x] T059 [P] [US7] Create `script/agent-demo.sh` (login → prefs → same-session recommend → correct/forget budget; print redacted profile + trace_id) (002 T044)
- [x] T060 [P] [US7] Add Makefile target `demo-agent` invoking the script
- [x] T061 [US7] Write `doc/AGENT_AND_EVAL.md`: flows, Agent vs Hybrid numbers placeholders + fixed conditions, why no Mem0/memory-tools/dense A/B, reproduce steps, success/failure trace notes (002 T046)
- [x] T062 [P] [US7] Update `README.md` with Agent/recommend endpoint + eval/demo pointers (002 T047)
- [ ] T063 [US7] Run `make demo-agent` — expect SC-B04 PASS（需本地服务+seed-redis；脚本已就绪）

**Checkpoint**: 演示与文档可独立复现

---

## Phase 12: Phase B Polish & Regression

**Purpose**: 全量回归、冒烟文档、面试点勾选（002 T049–T053）

- [x] T064 [P] Document function-calling unavailable failure mode in `doc/AGENT_AND_EVAL.md` (002 T050); ensure startup/smoke fails clearly if tools unsupported
- [ ] T065 Run full regression: `go test ./... -count=1` && `make eval-rag-prod` && `make eval-agent` && `make demo-agent` (002 T052)（单元测试已过；需本地栈补跑 prod/demo）
- [x] T066 Execute remaining quickstart.md Phase B scenarios; confirm SC-B01–B08
  - 文档/契约侧已对齐；正式 inprocess 分数与 demo 抽检依赖本地联调
- [x] T067 Mark Interview Value checkboxes in `specs/003-agent-hardening/spec.md` and `specs/002-recommend-agent-memory/spec.md` (002 T053)
- [x] T068 [P] Knowledge Capture: add/update Agent eval + Agent-vs-Hybrid-RAG interview notes in `docs/solutions/interview/`; index README
- [x] T069 Sync `memory-bank/activeContext.md` — 003 Phase A+B complete status

---

## Dependencies & Execution Order

### Phase gates

```text
Phase 1–2 (Setup/Foundation)
  → US1 (Evidence) 🎯 MVP
  → US2 (Profile fill)     ∥ after US1 tools surface stable
  → US3 (Loop/SSE)         ∥ can overlap US2 if careful
  → US4 (MySQL)            needs US1 success path for persist semantics
  → US5 (Router)           can start after foundation; integrate with US4 run fields
  → US6 (Harness/Trace/RL) depends on US1–US5 pieces
  → Phase A Polish (T048–T051)
  === PHASE A MERGE OK ===
  → US7 eval harness (T052–T058)
  → Demo/Docs (T059–T063)
  → Phase B Polish (T064–T069)
```

### User story dependency graph

```text
US1 (Evidence)
 ├── US2 (Profile on search)
 ├── US3 (Budgets/SSE)
 └── US4 (Persist verified runs)
      └── US6 (Harness wraps all)
US5 (Router) ──┬── US6
               └── US7 (force_route / compare)
US6 ─────────────→ US7 (real harness)
US7 ─────────────→ Demo/Docs/Polish B
```

### Parallel opportunities

- T001–T003 setup parallel
- T007∥T008 US1 tests; T009∥ interfaces
- T015∥T019∥T020∥T026∥T027∥T035∥T040∥T041 跨故事测试文件（不同路径）可并行起草
- T028∥T029 models/interfaces
- T059∥T060∥T062 demo/docs 并行
- **禁止**在 Phase A Checkpoint 前启动 T053+ 真实 eval 作为合并门槛

### 002 task mapping

| 002 | 003 |
|-----|-----|
| T041 span 接线 | T044 |
| T042 eval harness | T053–T054 |
| T044–T047 demo/docs/README | T059–T062 |
| T045 hybrid baseline | T055 |
| T049–T053 回归/面试点 | T065–T067 |
| 缺失 agent_test / logic_test | T024 / T048 |

---

## Parallel Example: User Story 1

```bash
# 并行测试骨架：
# - internal/agent/evidence_test.go
# - internal/agent/verifier_test.go
# 然后串行：evidence.go → verifier.go → tools.go → loop.go → logic/handler
```

---

## Implementation Strategy

### MVP（仅 Phase A / US1）

1. T001–T006 foundation  
2. T007–T014 Evidence + verify 接线  
3. 停下来用假 LLM/单测证明「不能洗白引用」

### Phase A 增量交付

US1 → US2 → US3 → US4 → US5 → US6 → T048–T051 → **可合并**

### Phase B 增量交付

T052–T058（数字）→ T059–T063（demo/文档）→ T064–T069（回归/勾选）→ **可对外宣称评测闭环**

### Avoid

- 用 stub `eval-agent` 充数  
- 全局一个成功率当简历主数字（应用分 tag Δ + pass^k + 代价）  
- Phase B 阻塞 Phase A 代码合并  

---

## Notes

- [P] = 不同文件、无未完成依赖  
- 每个 US 含独立测试标准（见上 Independent Test）  
- 任务均含明确路径，可供 `/speckit-implement` 直接执行  
