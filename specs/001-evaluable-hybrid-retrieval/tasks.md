# Tasks: Evaluable Hybrid Shop Retrieval

**Input**: Design documents from `/specs/001-evaluable-hybrid-retrieval/`

**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/ ✅, quickstart.md ✅

**Tests**: Included — plan / source Tasks 0–3 require TDD（metrics、RRF、维度校验、ShopSearchLogic 先写失败测试）

**Organization**: 按用户故事分阶段；依赖顺序 **US2 → US4 → US1 → US3**（四者均为 P1，US1 完整验收依赖 Hybrid 默认）

**Out of scope**: Agent / 记忆 / RAGAS / dense↔hybrid 正式对照（见 spec FR-012）

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1–US4 maps to spec user stories
- Include exact file paths in descriptions

## Path Conventions

- Go: `cmd/`, `internal/`, `pkg/`, `script/`, `rag-evals/`, `docs/solutions/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 评测目录与忽略规则，不改业务行为

- [ ] T001 Create `rag-evals/golden/`, `rag-evals/baseline/.gitkeep`, and `rag-evals/reports/.gitignore` (`*` + `!.gitignore`) per plan.md
- [ ] T002 [P] Update root `.gitignore` to ignore `rag-evals/reports/*` while keeping `rag-evals/golden/` and `rag-evals/baseline/` trackable
- [ ] T003 [P] Keep `script/rag-eval.json` as smoke-only; add a one-line comment at top of file or adjacent note in `script/` / Makefile comment that it is NOT a formal baseline

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 无额外框架；确认现有向量写入主路径仍完整，作为所有故事前提

**⚠️ CRITICAL**: 完成后再进入用户故事（尤其 US2 契约修复）

- [ ] T004 Verify complete `ShopVectorDoc` write paths in `cmd/seed-vector/main.go` and `internal/mq/shop_update_handler.go` still populate `AvgPrice`/`Score`/`Comments`/`Sold` (document any gap in task notes; fix only if missing)

**Checkpoint**: 正式入库路径已知且完整 — 可开始 US2

---

## Phase 3: User Story 2 - 配置与数据契约正确 (Priority: P1)

**Goal**: 维度唯一真相、删除不完整入库旁路、Redis 镜像固定版本，避免「评测能跑但不可信」

**Independent Test**: 非法/不一致维度操作失败；完整写入路径带过滤元数据；compose 镜像非 `latest`

### Tests for User Story 2 ⚠️

> Write FIRST; ensure FAIL before implementation

- [ ] T005 [P] [US2] Add failing tests for `dim <= 0` and no silent 1536 fallback in `internal/config/redis/vector_test.go`
- [ ] T006 [P] [US2] Add failing tests for `validateEmbeddingDimension` / Embed length mismatch in `internal/llm/client_test.go`

### Implementation for User Story 2

- [ ] T007 [US2] Remove 1536 silent fallback; fail on illegal dim in `internal/config/redis/vector.go` (depends on T005)
- [ ] T008 [US2] Enforce embedding length vs `LLM_EMBEDDING_DIM` after Embed/EmbedBatch in `internal/llm/client.go` (depends on T006)
- [ ] T009 [US2] Delete incomplete `IngestShop` from `internal/logic/rag_logic.go` and any interface mentioning it
- [ ] T010 [US2] Pin `redis/redis-stack-server` to verified tag or digest in `docker-compose.yml` (prefer `docker image inspect` of working local image; else `7.4.0-v8` per research.md) and record chosen value in `specs/001-evaluable-hybrid-retrieval/quickstart.md` Prerequisites
- [ ] T011 [US2] Run `go test ./internal/config/redis/... ./internal/llm/... ./internal/logic/... -count=1` and confirm PASS

**Checkpoint**: US2 可独立验收（SC-004 / FR-006–008）

---

## Phase 4: User Story 4 - 线上与评测共用同一检索入口 (Priority: P1)

**Goal**: 抽出 `ShopSearchLogic` + 可注入 FilterExtractor；RAG 与 eval 同路径（先 dense 可跑，Hybrid 在 US3）

**Independent Test**: 同一 question + 显式 filter + 同一 strategy 下，有序 TopK shop_id 一致（抽样 ≥10；索引只读）

### Tests for User Story 4 ⚠️

- [ ] T012 [P] [US4] Add failing unit tests for ResolveFilter / Search wiring in `internal/logic/shop_search_logic_test.go` per `contracts/shop-search.md`

### Implementation for User Story 4

- [ ] T013 [P] [US4] Extend `ShopSearchResult` with `AvgPrice`/`Score`/`Comments`/`Sold` and update RediSearch `RETURN` in `internal/repository/interface/vector.go` and `internal/repository/vector_repo.go`
- [ ] T014 [US4] Implement `ShopSearchLogic` + `FilterExtractor` (reuse production LLM extract) in `internal/logic/shop_search_logic.go` — Search supports `dense` first; `hybrid` may return clear ErrNotImplemented until US3 (depends on T012, T013)
- [ ] T015 [US4] Refactor `RAGLogic` to depend on `ShopSearchLogic` (no inline Embed+Search duplication) in `internal/logic/rag_logic.go`
- [ ] T016 [US4] Wire `ShopSearchLogic` in `cmd/server/main.go` dependency injection
- [ ] T017 [US4] Add in-process consistency helper or test asserting ordered IDs for fixed cases in `internal/logic/shop_search_logic_test.go` (SC-003 sampling prep)
- [ ] T018 [US4] Run `go test ./internal/logic/... ./internal/repository/... -count=1` and confirm PASS

**Checkpoint**: 线上对话检索已走共享入口；eval 可对接同一 Logic（US1）

---

## Phase 5: User Story 1 - 可信检索评测基线 (Priority: P1) 🎯 MVP 核心

**Goal**: 正式 golden set + 正确指标 bundle + `cmd/eval-rag`（filter-mode × retriever）+ smoke 分离；可出带实验元数据的报告（完整 hybrid 基线在 US3 录制）

**Independent Test**: 固定种子 + 正式题集跑正式评测命令，得到 HitRate/Recall/Precision/MRR/nDCG/Filter*/Infra + 元数据；HitRate 不得标成 Recall

### Tests for User Story 1 ⚠️

- [ ] T019 [P] [US1] Add failing table-driven metrics tests (HitRate≠Recall, MRR, empty relevant reject, infra not in quality denominator) in `cmd/eval-rag/metrics_test.go`

### Implementation for User Story 1

- [ ] T020 [P] [US1] Author `rag-evals/golden/retrieval.v1.json` (25–35 cases: id/split/question/relevant_shop_ids/oracle_filter/tags/evidence) from `script/seed.sql` + blogs; empty relevant forbidden; test split freeze discipline
- [ ] T021 [US1] Implement metrics pure functions + report types in `cmd/eval-rag/metrics.go` and `cmd/eval-rag/types.go` (depends on T019)
- [ ] T022 [US1] Refactor `cmd/eval-rag/main.go`: load golden+smoke schemas, `--filter-mode` none|oracle|llm, `--retriever`, `--split`, `--out`, `--write-baseline`; call `ShopSearchLogic`; compute SHA-256; infra errors → non-zero exit on formal runs; block baseline write for smoke
- [ ] T023 [US1] Add Makefile targets `eval-rag-smoke`, `eval-rag-oracle`, `eval-rag-prod` in `Makefile` (default retriever hybrid when US3 done; until then document dense-only diagnostic if needed)
- [ ] T024 [US1] Document smoke vs formal distinction in `.cursor/skills/rag-eval/SKILL.md` (remove outdated dense A/B production baseline wording)
- [ ] T025 [US1] Run `go test ./cmd/eval-rag/... -count=1` and a smoke eval against `script/rag-eval.json` with `--filter-mode=none`

**Checkpoint**: 指标与题集可信；正式入口默认指向 golden（完整 `hybrid_prod` 见 US3）

---

## Phase 6: User Story 3 - 默认混合检索上线 (Priority: P1)

**Goal**: 客户端 TEXT + KNN + FuseRRF；线上与正式评测默认 hybrid；文本失败不静默降级；录制 `hybrid_prod` 基线

**Independent Test**: 默认走 Hybrid；文本子检索失败显式失败；可写入 `rag-evals/baseline/hybrid_prod_v1.json`

### Tests for User Story 3 ⚠️

- [ ] T026 [P] [US3] Add failing `FuseRRF` tests (overlap ranks first, unilateral docs kept, stable tie-break by shop_id, TopK cap) in `internal/rag/rrf_test.go`
- [ ] T027 [P] [US3] Add failing tests that hybrid must error when text search fails (no silent dense success) in `internal/logic/shop_search_logic_test.go`

### Implementation for User Story 3

- [ ] T028 [US3] Implement `FuseRRF` in `internal/rag/rrf.go` (depends on T026)
- [ ] T029 [US3] Add TEXT search (`name` + `text_content`, Top20) on `VectorRepo` in `internal/repository/interface/vector.go` and `internal/repository/vector_repo.go`
- [ ] T030 [US3] Implement hybrid path in `internal/logic/shop_search_logic.go`: parallel KNN+TEXT via errgroup, RRF k=60, default strategy hybrid; env `RAG_RETRIEVER` default `hybrid` (depends on T027–T029)
- [ ] T031 [US3] Ensure `/api/rag/chat` and `cmd/eval-rag` default to hybrid; dense remains `--retriever=dense` / diagnostic only
- [ ] T032 [US3] Record production baseline: `go run ./cmd/eval-rag --filter-mode=llm --retriever=hybrid --split=test --write-baseline` → `rag-evals/baseline/hybrid_prod_v1.json` with `n_infra_error=0` (**no** dense compare report)
- [ ] T033 [US3] Run `go test ./internal/rag/... ./internal/logic/... ./internal/repository/... ./cmd/eval-rag/... -count=1` and `make eval-rag-prod` per quickstart.md

**Checkpoint**: US3 + US1 生产口径基线完成（SC-001 / SC-006）

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: 对齐文档、端到端抽检、进度同步

- [ ] T034 [P] Run quickstart.md scenarios 1–6 (smoke, prod baseline already done, oracle optional, path consistency ≥10 cases, dim fail check, metric divergence spot-check) and note results
- [ ] T035 [P] Sync any remaining wording in `docs/plans/2026-07-11-recommend-agent-eval.md` §1.1 eval matrix comments if still implying required dense A/B delivery (pointer to spec 2026-07-21)
- [ ] T036 Update `memory-bank/activeContext.md` to mark 001 implementation status after validation
- [ ] T037 Confirm SC-002/SC-003/SC-005/SC-007 against generated report artifacts

---

## Phase 8: Knowledge Capture (Constitution VI) 📚

**Purpose**: 秋招可复习知识落盘

**⚠️ CRITICAL**: 宣称 001 完成前必须完成本阶段（或证明 N/A）

- [ ] T038 [P] Add or update `docs/solutions/interview/` entry for 评测与线上路径对齐（`ShopSearchLogic`）using `docs/solutions/TEMPLATE.md`
- [ ] T039 [P] Add or update `docs/solutions/interview/` entry for HitRate vs Recall vs Precision（含本仓库指标定义）
- [ ] T040 [P] Confirm existing `docs/solutions/interview/hybrid-default-skip-dense-ab.md` still accurate after Hybrid landing; amend if needed
- [ ] T041 [P] If blockers occurred (dim mismatch, index rebuild, annotation disputes): add `docs/solutions/blockers/` entries
- [ ] T042 Update `docs/solutions/README.md` index for new/updated entries; ensure each interview note has 一句话结论 + ≥2 追问

**Checkpoint**: 知识可检索；可进入 `/speckit-implement`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: 无依赖
- **Foundational (Phase 2)**: 依赖 Setup
- **US2 (Phase 3)**: 依赖 Foundational — **BLOCKS** 可信评测与 Hybrid
- **US4 (Phase 4)**: 依赖 US2 — 共享入口（dense 可先交付）
- **US1 (Phase 5)**: 依赖 US4（eval 必须调 ShopSearchLogic）；golden/metrics 测试可与 US4 部分并行
- **US3 (Phase 6)**: 依赖 US4；完整 US1 验收（默认 hybrid + baseline）依赖 US3
- **Polish (Phase 7)**: 依赖 US3
- **Knowledge Capture (Phase 8)**: 依赖故事完成 / 卡点已解决

### User Story Dependencies

```text
US2 (契约) ──► US4 (共享入口) ──► US1 (评测基线 CLI+指标+golden)
                      │                    │
                      └──────► US3 (Hybrid 默认 + hybrid_prod) ──► US1 完整 Independent Test
```

- **US2**: 不依赖其他故事
- **US4**: 依赖 US2
- **US1**: 依赖 US4；完整独立测试依赖 US3
- **US3**: 依赖 US4（及 US1 的 eval CLI 以便录基线）

### Within Each User Story

- Tests FAIL → implement → PASS
- Repo/interface before Logic；Logic before CLI/Handler 接线
- 禁止在未完成 US3 时把 dense 结果写成对外 `hybrid_prod` baseline

### Parallel Opportunities

- T001–T003 Setup 可并行（T001 与 T002/T003）
- T005∥T006；T013∥T012 准备；T019∥T020；T026∥T027
- T038∥T039∥T040∥T041（Knowledge）
- 单人建议严格按 Phase 顺序；双人：A=US2→US4→US3，B=golden(T020)+metrics(T019/T021) 在 US4 完成后合流 T022

---

## Parallel Example: User Story 2

```bash
# 并行写失败测试：
Task: "Add failing tests in internal/config/redis/vector_test.go"
Task: "Add failing tests in internal/llm/client_test.go"

# 再串行实现 T007 → T008 → T009 → T010 → T011
```

## Parallel Example: User Story 1

```bash
# 可并行：
Task: "metrics_test.go failing cases"
Task: "Author rag-evals/golden/retrieval.v1.json"

# 合流后：
Task: "metrics.go + types.go → main.go CLI → Makefile → skill 文档"
```

## Parallel Example: User Story 3

```bash
# 可并行：
Task: "internal/rag/rrf_test.go"
Task: "shop_search_logic_test.go hybrid failure policy"

# 合流后：
Task: "rrf.go → VectorRepo TEXT → ShopSearchLogic hybrid → baseline"
```

---

## Implementation Strategy

### MVP First

1. Phase 1–2 Setup + Foundational  
2. Phase 3 US2 契约  
3. Phase 4 US4 共享入口（dense）  
4. Phase 5 US1 golden + 正确指标 + eval CLI  
5. **STOP**：用 `--retriever=dense` 诊断跑通正式集（**不得**对外宣称 hybrid 基线）  
6. Phase 6 US3 Hybrid + `hybrid_prod` → 规格完整验收  
7. Phase 7–8 Polish + Knowledge Capture  

### Incremental Delivery

| Increment | 可演示价值 |
|-----------|------------|
| US2 | 维度/镜像契约 fail-fast |
| US4 | 线上/评测同入口 |
| US1 | 可信指标报告（路径已对齐） |
| US3 | Hybrid 默认 + Agent 对照用 RAG 基线 |

### Suggested MVP Scope

**最小可演示**：US2 + US4 + US1（eval 报告正确）。  
**规格完成定义**：再加 US3 + `hybrid_prod` + Knowledge Capture（T038–T042）。

---

## Notes

- [P] = 不同文件且不依赖未完成任务
- 每任务含明确路径，可供 `/speckit-implement` 直接执行
- Commit 仅在用户明确要求时进行
- 实现期遵循 AGENTS.md §5.8 解释改动
- **禁止**交付 dense vs hybrid 对照报告或简历叙事（FR-012）
