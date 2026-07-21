# Contract: `cmd/eval-rag` CLI

**Binary**: `go run ./cmd/eval-rag`  
**Date**: 2026-07-21

## Flags

| Flag | Default | Values | Notes |
|------|---------|--------|-------|
| `--test-set` | `rag-evals/golden/retrieval.v1.json` | path | Smoke: `script/rag-eval.json` |
| `--split` | `test` | `test` \| `dev` \| `all` | Smoke 无 split 时忽略 |
| `--filter-mode` | `llm`（正式） | `none` \| `oracle` \| `llm` | Smoke 旧格式仅允许 `none` |
| `--retriever` | `hybrid` | `hybrid` \| `dense` | dense 仅诊断，不写正式 baseline |
| `--top-k` | `5` | int | |
| `--out` | `rag-evals/reports/retrieval_latest.json` | path | |
| `--write-baseline` | `false` | bool | 仅正式集 + hybrid +（建议）llm |
| `--baseline` | `rag-evals/baseline/hybrid_prod_v1.json` | path | write 目标 |

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | 成功完成；正式模式下 `n_infra_error == 0` |
| `≠0` | 配置/加载非法、或正式评测存在 infra 失败、或 smoke 误用 write-baseline |

## Report JSON (required fields)

见 [data-model.md](../data-model.md) §5。至少包含：

- 实验元数据：dataset version + sha256、redis_image、embedding_model/dim、filter_mode、retriever、top_k、rrf 参数、n_*
- 指标：`hit_rate_at_k`、`recall_at_k`、`precision_at_k`、`mrr`、`ndcg_at_k`、filter 指标（适用时）、`infra_error_rate`
- `per_case` 摘要

禁止将 hit_rate 字段命名为 recall。

## Makefile aliases (target)

```makefile
eval-rag-smoke:   # script/rag-eval.json, filter-mode=none, retriever=hybrid
eval-rag-oracle:  # golden, filter-mode=oracle, retriever=hybrid
eval-rag-prod:    # golden, filter-mode=llm, retriever=hybrid  （生产口径）
```

**不做** `eval-rag-compare` dense↔hybrid 正式对照目标（Spec FR-004 / FR-012）。

## HTTP surface (unchanged product shape)

线上仍为 `POST /api/rag/chat`（Auth + SSE）。契约变更在**检索内部**走 `ShopSearchLogic`；请求/响应产品形态不要求本规格改动。可选请求体 `filter` 仍映射为显式 `VectorSearchFilter`。
