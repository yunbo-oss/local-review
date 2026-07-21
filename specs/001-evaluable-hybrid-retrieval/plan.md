# Implementation Plan: Evaluable Hybrid Shop Retrieval

**Branch**: `001-evaluable-hybrid-retrieval` | **Date**: 2026-07-21 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-evaluable-hybrid-retrieval/spec.md`

**Note**: This plan covers source-plan Tasks 0–3 only (Hybrid default + shared retrieval + trustworthy eval). Recommend Agent / memory / Agent eval → `002-*`.

## Summary

在现有 Naive Dense RAG 之上，交付**可复现、与线上同路径的 Hybrid 检索生产口径**：先修维度/入库/镜像契约，再建正式 golden set 与正确指标 bundle，抽出 `ShopSearchLogic` 供 `/api/rag/chat` 与 `cmd/eval-rag` 共用，默认客户端 RRF（TEXT + HNSW），录制 `hybrid_prod` 基线供后续 Agent 对照。**不做** dense vs hybrid 对照交付与简历叙事；dense 仅诊断开关。

技术路线对齐源计划 `docs/plans/2026-07-11-recommend-agent-eval.md` §1.1 / §1.6 / Tasks 0–3，验收以本规格为准。

## Technical Context

**Language/Version**: Go 1.24+

**Primary Dependencies**: Gin、GORM、go-redis/v9、go-openai 兼容客户端（现有 `internal/llm`）、OpenTelemetry（已有 HTTP 层；本规格不强制补齐 RAG 内部 span，可顺带留 hook）、golang.org/x/sync（errgroup）

**Storage**: MySQL（店铺/点评种子）、Redis Stack（Hash + RediSearch TEXT + HNSW VECTOR；索引 `idx:shop:vector`）

**Testing**: `go test` 表驱动（metrics、RRF、维度校验、ShopSearchLogic）；`make eval-rag-smoke` / `eval-rag-oracle` / `eval-rag-prod`；抽样对齐线上 vs eval TopK ID（SC-003）

**Target Platform**: macOS / Linux 开发机 + Docker Compose（MySQL + Redis Stack）；**非** AidLux 犀牛派作 baseline 环境

**Project Type**: 单体 Go Web 服务 + 独立 CLI 评测工具（`cmd/server`、`cmd/eval-rag`、`cmd/seed-vector`）

**Performance Goals**: 店铺 catalog 约数十家；检索 TopK=5；Hybrid 两路各取 Top20 后 RRF；正式评测 25～35 题可在约 1 小时内跑完（含 LLM filter）

**Constraints**:
- `LLM_EMBEDDING_DIM` 为维度唯一真相；非法/不一致必须失败，禁止静默 fallback/截断
- Hybrid 文本子检索失败 → 整次请求失败或记 infra error，禁止静默标 hybrid 成功
- 质量指标分母 = `n_evaluated`；`n_infra_error` 单列；正式入口 infra 失败非零退出
- 禁止 HitRate 命名为 Recall；禁止 smoke 集作对外 baseline
- 不做 Mem0 / Eino / LangGraph / Milvus / FT.HYBRID 服务端绑定 / dense A/B 交付

**Scale/Scope**: 正式集 25～35 题；共享 Logic + Hybrid 默认 + 一份 `hybrid_prod` 基线；OTel RAG 内部 span 与 Agent 不在本规格

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*
*Source: `.specify/memory/constitution.md` v1.1.1 (local-review-go)*

- [x] **I. Interview-First**: 可讲清「线上/评测路径分裂 → 共享入口 + Hybrid 默认 + 正确指标」；面试卖点：① Hybrid RRF 生产路径 ② 共享检索入口 ③ HitRate≠Recall + 质量/infra 分母分离（跳过 dense A/B 的取舍见已有 `docs/solutions/interview/hybrid-default-skip-dense-ab.md`）
- [x] **II. Layered Architecture**: `Handler → ShopSearchLogic / RAGLogic → VectorRepo 接口 → Redis`；eval CLI 直接调 Logic，不直连 DB；无 Handler 业务逻辑
- [x] **III. Plan → Build → Test**: 验证见下方 Project Structure / quickstart；指标与 RRF 先测后实现
- [x] **IV. Explain-as-You-Build**: 实现阶段按 AGENTS.md §5.8 解释（计划不产出正文）
- [x] **V. Simplicity**: 客户端 RRF，不换向量库、不上生成式 rerank、不做 dense 正式对照；复杂度见 Complexity Tracking（仅客户端 Hybrid 相对纯 dense）
- [x] **VI. Knowledge Capture**: 已有 hybrid 取舍条目；实现期补充/更新 `评测与线上路径对齐`、`HitRate vs Recall vs Precision`（interview/）；卡点按需 blockers/

## Project Structure

### Documentation (this feature)

```text
specs/001-evaluable-hybrid-retrieval/
├── plan.md              # This file
├── research.md          # Phase 0
├── data-model.md        # Phase 1
├── quickstart.md        # Phase 1
├── contracts/           # Phase 1
│   ├── shop-search.md
│   └── eval-rag-cli.md
└── tasks.md             # Phase 2 (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
cmd/
├── server/main.go                 # 注入 ShopSearchLogic → RAGLogic
├── eval-rag/
│   ├── main.go                    # CLI：filter-mode × retriever + report
│   ├── types.go
│   ├── metrics.go
│   └── metrics_test.go
└── seed-vector/main.go            # 完整 ShopVectorDoc 写入（保留）

internal/
├── logic/
│   ├── shop_search_logic.go       # 新建：共享检索入口
│   ├── shop_search_logic_test.go
│   └── rag_logic.go               # 改：依赖 ShopSearchLogic；删 IngestShop
├── rag/
│   ├── text.go                    # 已有 embedding 文本
│   ├── rrf.go                     # 新建：FuseRRF 纯函数
│   └── rrf_test.go
├── repository/
│   ├── interface/vector.go        # 扩展 SearchText / 结果元数据字段
│   └── vector_repo.go             # TEXT 检索 + 既有 KNN
├── config/redis/vector.go         # 删 1536 fallback；非法 dim 报错
├── llm/client.go                  # Embed 后维度校验
└── handler/rag.go                 # 仍 SSE；检索走共享 Logic

rag-evals/
├── golden/retrieval.v1.json       # 正式集 25～35
├── baseline/
│   ├── .gitkeep
│   └── hybrid_prod_v1.json        # 录制后提交或本地冻结
└── reports/                       # gitignore 本地产物

script/rag-eval.json               # smoke ≤7，非 baseline
docker-compose.yml                 # pin redis-stack-server 固定 tag
Makefile                           # eval-rag-smoke / oracle / prod
```

**Structure Decision**: 延续单体 Go 仓库分层；新增 Logic 共享入口与 `internal/rag` 纯函数；评测数据与报告放仓库根 `rag-evals/`（与源计划一致），不新建微服务。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| 客户端 Hybrid（TEXT + KNN + RRF）相对纯 dense | Spec US3 / FR-009：生产默认 Hybrid；词面补店名/专有名词 | 仅 dense：规格明确默认 hybrid；仅依赖 `FT.HYBRID`：绑定较新 Redis，且难单测 |
| 单独 `ShopSearchLogic`（相对 RAGLogic 内联） | FR-005 / SC-003：线上与 eval 同路径；002 Agent tool 复用 | eval 继续直连 VectorRepo：路径分裂，基线不可信 |

## Implementation Phases (for /speckit-tasks)

对齐源计划 Tasks 0–3（细节以 tasks.md 为准）：

0. **契约修复**：维度唯一真相、删 `IngestShop`、pin Redis 镜像  
1. **Golden set**：`retrieval.v1.json` + smoke 分离  
2. **Eval + 共享入口**：正确指标、filter-mode、ShopSearchLogic、接线 RAG  
3. **Hybrid 默认**：TEXT 检索 + FuseRRF(k=60)、默认 hybrid、录 `hybrid_prod`

## Post-Design Constitution Re-check

- 设计仍遵守分层与 YAGNI；知识落盘已规划；无新增未论证框架。**GATE PASS**。
