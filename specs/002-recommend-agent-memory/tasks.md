# Tasks: Recommend Agent with Correctable Memory

**Input**: Design documents from `/specs/002-recommend-agent-memory/`

**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/ ✅, quickstart.md ✅

**Tests**: Included — plan / source Tasks 4–8 要求 TDD（profile merge、extract、tools、bounded loop、graders、handler SSE 先写失败测试）

**Organization**: 按用户故事分阶段；推荐执行顺序 **US1 ∥ US4(graders) → US2 → US3 → US5**（US1/US4 可并行；US2 依赖 US1；US3 依赖 US2；US5 依赖 US2–US4）

**Out of scope**: Mem0 / LangGraph / 记忆工具 / 向量记忆 Phase 2 / dense↔hybrid 对照（见 spec FR-017）

**Prerequisite**: 规格 `001` 共享 `ShopSearchLogic` + Hybrid 默认；对照需 `rag-evals/baseline/hybrid_prod_v1.json`（若缺失，US5 对照任务先录制）

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1–US5 maps to spec user stories
- Include exact file paths in descriptions

## Path Conventions

- Go: `cmd/`, `internal/`, `pkg/`, `script/`, `rag-evals/`, `doc/`, `docs/solutions/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 包骨架、Redis key 常量、Agent 配置占位；不改线上行为

- [x] T001 Create package dirs `internal/memory/`, `internal/agent/`, `cmd/eval-agent/` per plan.md Project Structure
- [x] T002 [P] Add Redis key helpers `AgentSessionKey` / `AgentProfileKey` in `pkg/utils/redisx/keys.go` (`agent:sess:{userId}:{sessionId}`, `agent:profile:{userId}`)
- [x] T003 [P] Document Agent env defaults (`AGENT_MAX_STEPS=3`, `AGENT_MAX_TOOL_CALLS=5`, `AGENT_RUN_TIMEOUT=45s`, `AGENT_TOOL_TIMEOUT=10s`, `AGENT_MAX_TOOL_RESULT_CHARS=6000`) in `.env.example`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: LLM tool-calling 抽象与 Agent 配置类型 — 所有故事依赖的横切基础

**⚠️ CRITICAL**: 完成后再进入用户故事实现（US4 graders 纯函数可与 Phase 2 尾部并行，但不得依赖未完成的 ToolChatClient 实现细节）

- [x] T004 Add failing tests for `ChatWithTools` / `AssistantTurn` shapes in `internal/llm/client_test.go` per research.md §5
- [x] T005 Implement `ToolChatClient` interface + `ChatWithTools` on `internal/llm/client.go` (OpenAI-compatible tools; Agent 依赖接口不绑 go-openai 类型); keep existing `ChatStream` / `ChatComplete` (depends on T004)
- [x] T006 [P] Add Agent `RunConfig` defaults struct in `internal/agent/types.go` (maxSteps/maxToolCalls/timeouts/resultChars) loaded from env
- [x] T007 Run `go test ./internal/llm/... ./internal/agent/... -count=1` and confirm Phase 2 tests PASS (agent may only have types)

**Checkpoint**: Tool calling 与 RunConfig 就绪 — 可开始 US1 与 US4 graders

---

## Phase 3: User Story 1 - 可纠正的结构化偏好记忆 (Priority: P1) 🎯 MVP

**Goal**: Session + Profile 持久化；补丁 merge（删除优先、budget 三态）；filter 仅补空；抽取失败不覆盖；CAS 并发安全

**Independent Test**: 写入初始偏好 → 空缺条件提问验证补全 → 纠正/清空话术 → 检查偏好终态；可先在 RAG 检索路径验证「仅补空」，不依赖 Agent

**Contracts**: [contracts/memory-repo.md](./contracts/memory-repo.md)  
**Data model**: [data-model.md](./data-model.md) §1–4

### Tests for User Story 1 ⚠️

> Write FIRST; ensure FAIL before implementation

- [x] T008 [P] [US1] Add failing table-driven `MergeProfile` / `MergeFilterWithProfile` tests (add/remove conflict → remove wins; budget nil/0/positive; explicit filter not overwritten; summary ≤80) in `internal/memory/profile_test.go`
- [x] T009 [P] [US1] Add failing `ExtractProfilePatch` parse/validate tests (strict JSON; unknown fields reject; assistant output not used as source) in `internal/memory/extract_test.go`
- [x] T010 [P] [US1] Add failing `MemoryRepo` tests (session RPUSH/LTRIM/EXPIRE; profile WATCH/CAS retry; load empty → zero value) in `internal/repository/memory_repo_test.go`

### Implementation for User Story 1

- [x] T011 [P] [US1] Define `Profile`, `Message`, `ProfilePatch` in `internal/memory/types.go` per data-model.md
- [x] T012 [US1] Implement `MergeProfile` + `MergeFilterWithProfile` in `internal/memory/profile.go` (depends on T008, T011)
- [x] T013 [US1] Implement `ExtractProfilePatch` (user utterance only + old profile → patch) in `internal/memory/extract.go` (depends on T009, T011)
- [x] T014 [P] [US1] Create `MemoryRepo` interface in `internal/repository/interface/memory.go` per contracts/memory-repo.md
- [x] T015 [US1] Implement Redis `MemoryRepo` in `internal/repository/memory_repo.go` (session pipeline; profile CAS ≤3 retries; TTL 7d/90d) (depends on T002, T010, T012, T014)
- [x] T016 [US1] Extend `ShopSearchLogic` ResolveFilter path to apply `MergeFilterWithProfile` (explicit > extracted > profile fill-empty only) in `internal/logic/shop_search_logic.go`
- [x] T017 [US1] Wire `MemoryRepo` into `cmd/server/main.go`; on RAG path load profile by `middleware.GetUserInfo` when logged in; profile load failure → Warn and continue without profile in `internal/handler/rag.go` / `internal/logic/rag_logic.go` as needed
- [x] T018 [US1] Run `go test ./internal/memory/... ./internal/repository/... ./internal/logic/... ./internal/handler/... -count=1` and confirm PASS

**Checkpoint**: US1 可独立验收（SC-004 记忆语义 / FR-001–004）— MVP 记忆闭环

---

## Phase 4: User Story 4 - Agent 评测定义（Graders + Golden）(Priority: P1)

**Goal**: 先定义场景与确定性 graders（助手实现前可单测）；覆盖记忆/多步/无结果/失败/防循环

**Independent Test**: `go test ./cmd/eval-agent` graders PASS；`agent.v1.json` 可解析；10～15 条且关键 tag 齐全（完整 harness 在 US5）

**Contracts**: [contracts/eval-agent-cli.md](./contracts/eval-agent-cli.md)  
**Note**: 可与 Phase 3 **并行**（不同目录）

### Tests for User Story 4 ⚠️

- [x] T019 [P] [US4] Add failing grader tests (unknown shop_id → groundedness FAIL; over max_steps → trajectory FAIL; profile mismatch → outcome FAIL; alternate valid tool order not FAIL) in `cmd/eval-agent/graders_test.go`

### Implementation for User Story 4

- [x] T020 [P] [US4] Author `rag-evals/golden/agent.v1.json` (10–15 cases: setup_profile/turns/expected/tags/evidence; ≥5 critical with trials≥3; cover memory/multi_step/no_result/tool_error/anti_loop) from seed catalog
- [x] T021 [US4] Implement report/case types in `cmd/eval-agent/types.go` per data-model.md AgentCase/AgentReport
- [x] T022 [US4] Implement pure graders `GradeOutcome` / `GradeGroundedness` / `GradeTrajectory` / `AggregateTrials` in `cmd/eval-agent/graders.go` (depends on T019, T021)
- [x] T023 [US4] Run `go test ./cmd/eval-agent -count=1` and `python3 -c "import json; json.load(open('rag-evals/golden/agent.v1.json')); print('ok')"` — expect PASS / ok

**Checkpoint**: 评测定义就绪；US5 再接通 harness 跑真实 Agent

---

## Phase 5: User Story 2 - 有界多步店铺推荐助手 (Priority: P1)

**Goal**: 三只读工具 + bounded loop + RecommendAgentLogic + 登录态 SSE 入口；预算/超时/去重；记忆服务端注入（非工具）

**Independent Test**: 登录后对需「先搜再读详情/评价」的问题，在预算内返回可读推荐；未登录/空参数拒绝

**Contracts**: [contracts/agent-tools.md](./contracts/agent-tools.md), [contracts/agent-recommend-api.md](./contracts/agent-recommend-api.md)  
**Depends on**: US1 (MemoryRepo + profile fill)

### Tests for User Story 2 ⚠️

- [x] T024 [P] [US2] Add failing tool executor tests (illegal JSON, negative shop_id, oversized limit, unknown fields, result truncation, tool timeout) in `internal/agent/tools_test.go`
- [x] T025 [P] [US2] Add failing bounded-loop tests with fake LLM (search→answer; search→get_shop; duplicate tool+args rejected; maxSteps truncate; tool error feedback; context cancel stops) in `internal/agent/loop_test.go`
- [x] T026 [P] [US2] Add failing handler tests (401 unauth; 400 empty question/session_id; SSE content-type; error/done events; cancel propagates) in `internal/handler/agent_test.go`

### Implementation for User Story 2

- [x] T027 [P] [US2] Implement tool schemas + `ToolExecutor` (`search_shops` → ShopSearchLogic hybrid; `get_shop` → ShopRepo; `list_shop_blogs` → BlogRepo) in `internal/agent/tools.go` (depends on T005, T024)
- [x] T028 [US2] Implement bounded loop in `internal/agent/loop.go` (budgets, dedupe key=toolName+canonicalJSON, status callbacks) (depends on T006, T025, T027)
- [x] T029 [US2] Implement `RecommendAgentLogic` in `internal/logic/recommend_agent_logic.go`: LoadSession+LoadProfile → system inject summary → loop → (groundedness deferred to US3 soft-pass if needed) → AppendSession on success → ExtractProfilePatch+MergeProfile (merge fail Warn only); disconnect → no profile write (depends on T015, T028)
- [x] T030 [US2] Add unit tests for RecommendAgentLogic happy path with fakes in `internal/logic/recommend_agent_logic_test.go`
- [x] T031 [US2] Implement `AgentHandler` SSE (`status`/`message`/`done`/`error`; no CoT/tool raw/profile dump) in `internal/handler/agent.go` (depends on T026, T029)
- [x] T032 [US2] Register `authGroup.POST("/agent/recommend", ...)` in `internal/handler/router.go`; wire deps in `cmd/server/main.go` (fail start if required deps missing)
- [x] T033 [US2] Run `go test ./internal/agent/... ./internal/logic/... ./internal/handler/... ./internal/llm/... -count=1` and confirm PASS

**Checkpoint**: US2 可独立冒烟（FR-005–007 / SC-006–007 部分）；groundedness 硬门禁见 US3

---

## Phase 6: User Story 3 - 推荐回答必须有据可查 (Priority: P1)

**Goal**: observed shop ID 集合；最终回答 `[shop:{id}]` 必须 ⊆ observed；校验失败不得发成功 message；无结果/工具失败不编造

**Independent Test**: 固定 observed 集合场景（含无结果、工具错误）→ 引用集合子集校验；无结果明确说明

**Depends on**: US2 loop + RecommendAgentLogic

### Tests for User Story 3 ⚠️

- [x] T034 [P] [US3] Add failing groundedness tests (cite unknown id → error, no successful message; expect_no_results path; tool error no fabricated shops) in `internal/agent/loop_test.go` and/or `internal/logic/recommend_agent_logic_test.go`

### Implementation for User Story 3

- [x] T035 [US3] Track `observedShopIDs` across tool results in `internal/agent/loop.go` / tools executor; expose to logic
- [x] T036 [US3] Parse `[shop:{id}]` citations; validate ⊆ observed before emitting answer; on fail return grounding error (no success SSE message) in `internal/logic/recommend_agent_logic.go` and `internal/handler/agent.go` (depends on T034, T035)
- [x] T037 [US3] Ensure duplicate tool+args rejection counts toward trajectory and does not expand observed set incorrectly in `internal/agent/loop.go`
- [x] T038 [US3] Run `go test ./internal/agent/... ./internal/logic/... ./internal/handler/... -count=1` — confirm SC-002/SC-007 sample cases PASS

**Checkpoint**: Groundedness 硬约束就绪（FR-008 / SC-002）

---

## Phase 7: User Story 5 - 演示、限流、可观测与对照文档 (Priority: P2)

**Goal**: 用户级限流；OTel spans（无隐私正文）；接通 `eval-agent` harness；demo 三轮；`doc/AGENT_AND_EVAL.md`；Agent vs Hybrid RAG 对照；知识落盘

**Independent Test**: `make demo-agent` 三轮可复现；`make eval-agent` 出完整报告；观测摘要可区分 steps/tool/latency 且无原文隐私

**Depends on**: US2–US4；001 `hybrid_prod` baseline（缺失则先录制）

### Implementation for User Story 5

- [x] T039 [P] [US5] Implement user-scoped `AgentRateLimit` middleware in `internal/middleware/agent_rate_limit.go`; return 429 before LLM; wire on recommend route in `internal/handler/router.go`
- [x] T040 [P] [US5] Add OTel helpers in `internal/agent/trace.go` (`agent.run`, `llm.tool_turn`, `tool.execute`, `rag.embed`, `rag.search`, `llm.generate`); attributes limited to model/tool/status/steps/tokens/latency/candidate_count — no question/profile/blog body
- [ ] T041 [US5] Wire spans into loop / RecommendAgentLogic / ShopSearchLogic call sites as needed (depends on T040)
- [ ] T042 [US5] Implement `cmd/eval-agent/main.go` harness: per-trial session_id; setup_profile; run Agent; capture transcript/tools/observed/profile/usage; run graders; SHA-256; infra non-zero exit; optional `--compare-baseline` per contracts/eval-agent-cli.md (depends on T022, T029, T036)
- [x] T043 [P] [US5] Add Makefile targets `eval-agent` and `demo-agent` in `Makefile`
- [ ] T044 [P] [US5] Create `script/agent-demo.sh` (login → state prefs → same-session recommend → correct/forget budget; print redacted profile + trace_id)
- [ ] T045 [US5] If missing, record `rag-evals/baseline/hybrid_prod_v1.json` via `make eval-rag-prod` / `--write-baseline` (001 prerequisite for SC-005)
- [ ] T046 [US5] Write `doc/AGENT_AND_EVAL.md`: Hybrid RAG vs Agent flows; Agent vs Hybrid RAG numbers placeholders + fixed conditions; why no Mem0/memory-tools/dense A/B; how to reproduce; success+failure trace analysis
- [ ] T047 [P] [US5] Update `README.md` with Agent endpoint + eval/demo pointers; sync `memory-bank/activeContext.md` for 002 progress
- [x] T048 [US5] Knowledge Capture (constitution VI): add/update `docs/solutions/interview/` entries for 记忆非工具、有界Agent、Agent评测、Agent-vs-Hybrid-RAG；index in `docs/solutions/README.md`
- [ ] T049 [US5] Run quickstart.md scenarios 1–8: `go test ./... -count=1`, `make eval-agent`, `make demo-agent`; confirm SC-001/SC-003/SC-004/SC-005/SC-008

**Checkpoint**: 全规格 DoD 可宣称完成

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: 收尾对齐与回归

- [ ] T050 [P] Confirm startup fails clearly if chat model lacks function calling (smoke) per research.md §5; document in `doc/AGENT_AND_EVAL.md`
- [ ] T051 [P] Spot-check SSE never leaks tool raw / profile / CoT in `internal/handler/agent_test.go`
- [ ] T052 Run full regression: `go test ./... -count=1` && `make eval-rag-prod` && `make eval-agent` && `make demo-agent`
- [ ] T053 Mark completed interview talking points in `specs/002-recommend-agent-memory/spec.md` Interview Value section after docs land

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: 无依赖，立即开始
- **Foundational (Phase 2)**: 依赖 Setup — **阻塞** US2/US3（US1 记忆纯函数可不依赖 T005，但 MemoryRepo 与 wiring 在 T015+；US4 graders 可不依赖 T005）
- **US1 (Phase 3)**: Setup + keys；MVP
- **US4 graders (Phase 4)**: 可与 US1 **并行**（不同文件树）
- **US2 (Phase 5)**: 依赖 Phase 2 + US1
- **US3 (Phase 6)**: 依赖 US2
- **US5 (Phase 7)**: 依赖 US2–US4（+ 001 baseline）
- **Polish (Phase 8)**: 依赖拟交付故事完成

### User Story Dependencies

```text
Phase1 Setup
    ↓
Phase2 Foundational (ToolChatClient + RunConfig)
    ↓
    ├─ US1 Memory (MVP) ──────────────┐
    │                                  │
    └─ US4 Graders+Golden (parallel) ──┤
                                       ↓
                                 US2 Bounded Agent + SSE
                                       ↓
                                 US3 Groundedness
                                       ↓
                                 US5 Rate limit / OTel / harness / demo / docs
                                       ↓
                                     Polish
```

- **US1 (P1)**: 不依赖 Agent；可先交付记忆 + RAG 补空
- **US4 (P1)**: graders/golden 不依赖 Agent；harness 在 US5
- **US2 (P1)**: 依赖 US1 MemoryRepo
- **US3 (P1)**: 依赖 US2
- **US5 (P2)**: 依赖 US2–US4 与 001 baseline

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Types/models before merge/repo/services
- Services before HTTP
- Story checkpoint before next priority（US4 graders 例外可并行）

### Parallel Opportunities

- T002 ∥ T003（Setup）
- T008 ∥ T009 ∥ T010（US1 测试）
- T011 ∥ T014（US1 类型与接口）
- **US1 实现 ∥ US4 T019–T023**（推荐双线）
- T024 ∥ T025 ∥ T026（US2 测试）
- T039 ∥ T040 ∥ T043 ∥ T044 ∥ T047（US5 部分）

---

## Parallel Example: US1 ∥ US4

```bash
# Line A — Memory MVP
Task: "Failing MergeProfile tests in internal/memory/profile_test.go"
Task: "Implement MergeProfile in internal/memory/profile.go"
Task: "Implement MemoryRepo in internal/repository/memory_repo.go"

# Line B — Eval definition (same time, different dirs)
Task: "Author rag-evals/golden/agent.v1.json"
Task: "Failing graders in cmd/eval-agent/graders_test.go"
Task: "Implement graders in cmd/eval-agent/graders.go"
```

---

## Parallel Example: User Story 2

```bash
# Tests first (parallel):
Task: "Tool executor failing tests in internal/agent/tools_test.go"
Task: "Bounded loop failing tests in internal/agent/loop_test.go"
Task: "Handler failing tests in internal/handler/agent_test.go"

# Then sequential: tools → loop → RecommendAgentLogic → handler → router/DI
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1 Setup + Phase 2 Foundational（至少 T002；T004–T005 可稍后若只验 RAG 补空）
2. Phase 3 US1 全部完成
3. **STOP and VALIDATE**: profile merge + RAG 空字段补全 + 纠正清空
4. 再开 US4 graders 与 US2 Agent

### Incremental Delivery

1. Setup + Foundational → 基础就绪
2. US1 → 可演示「可纠正记忆」
3. US4 graders + golden → 评测尺子就位
4. US2 → SSE 推荐助手可冒烟
5. US3 → groundedness 硬门禁
6. US5 → eval 报告 + demo + 对照文档 + 面试知识落盘
7. Polish → 全量回归

### Suggested MVP Scope

- **最小可演示 MVP**: Phase 1–3（US1）— 记忆与 RAG 偏好补全
- **面试完整叙事 MVP**: 再加 US2 + US3 + US4 graders + US5 eval/demo（完整 Agent vs Hybrid RAG）

---

## Notes

- [P] = 不同文件、无未完成依赖
- [USn] = 对应 spec 用户故事
- 源计划 Task 4→US1；Task 5→US4；Task 6→US2；Task 6/7 groundedness→US3；Task 7–8→US5
- Commit 仅在用户明确要求时执行
- 避免：把记忆做成模型工具；流式未校验就发 message；dense vs hybrid 数字写入文档
- Constitution VI：US5 T048 为强制知识落盘任务
