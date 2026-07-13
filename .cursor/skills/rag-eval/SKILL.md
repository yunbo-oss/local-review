---
name: rag-eval
description: >-
  local-review-go 检索阶段 RAG 评测：golden set、cmd/eval-rag、HitRate/Recall/Precision/MRR/nDCG、
  filter 指标、Hybrid 对照与实验元数据。检索评测与线上一致路径时使用；端到端 RAGAS/生成质量见 002 Agent 规格。
license: Apache-2.0
compatibility: local-review-go 仓库；Go 1.24+；Redis Stack；LLM_API_KEY；可选参考 NVIDIA rag-eval upstream
metadata:
  author: local-review-go (adapted from NVIDIA skills/rag-eval v2.6.0)
  upstream: https://github.com/NVIDIA/skills/tree/main/skills/rag-eval
  upstream_note: NVIDIA skill 面向 RAGAS + corpus/train.json；本 skill 面向 shop retrieval golden set
tags:
  - rag
  - evaluation
  - retrieval
  - local-review-go
  - hybrid
  - interview
allowed-tools: Read Grep Glob Bash(go *) Bash(make *) Write Edit
---

# local-review-go 检索评测（RAG retrieval eval）

## Purpose

指导 Agent 在本仓库做**检索阶段**可复现评测（规格 `001-evaluable-hybrid-retrieval`），对齐业界 RAG 评测共识（[Redis RAG metrics](https://redis.io/blog/rag-metrics/)、[FutureAGI 2026](https://futureagi.com/blog/rag-evaluation-metrics-2025/)、NVIDIA Blueprint 分层评测思想）。

**本 skill 管检索；不管生成。** 生成侧 faithfulness / answer relevance（RAGAS 风格）留给 `002` Agent 规格。

## When to use

- 编写或修改 `cmd/eval-rag`、`rag-evals/golden/retrieval.v1.json`、检索指标、Hybrid 对照报告
- 审视 HitRate vs Recall 命名、filter 路径是否与线上一致
- 面试叙事：如何用数字证明 Hybrid 检索收益

## When NOT to use

- 部署 NVIDIA RAG Blueprint（见 upstream `SKILL.md.upstream`）
- 端到端 RAGAS + LLM judge 全链路（002 阶段；可参考 upstream RAGAS 指标名）
- 延迟压测（另建 perf 脚本；NVIDIA 用 rag-perf）

## Prerequisites

- Docker：`mysql` + `redis-stack` 已启动；`make seed` + `make seed-vector`（或等价）
- `LLM_EMBEDDING_DIM` 与 Redis 索引维度一致
- 正式集：`rag-evals/golden/retrieval.v1.json`（25～35 条人工核验）
- 冒烟集：`script/rag-eval.json`（≤10 条，**不得**作对外 baseline）

## Metric bundle（检索阶段，001 必报）

与 NVIDIA/业界对齐：**至少 1 个 coverage + 1 个 ranking + 1 个 sanity**。

| 指标 | 定义 | 角色 | 业界别名 |
|------|------|------|----------|
| **HitRate@K** | TopK 是否至少 1 个 relevant | Sanity / 查询级成功率 | Success@K |
| **Recall@K** | TopK 命中 relevant 数 / relevant 总数 | Coverage | context recall（有 gold ids 时） |
| **Precision@K** | TopK 中 relevant 数 / K | 噪声控制 | context precision（离散版） |
| **MRR** | 首个 relevant 排名倒数的均值 | 首个好结果多快出现 | — |
| **nDCG@K** | 二元 relevant 的 DCG/IDCG（可选分级） | 排序质量 | BEIR/MTEB 常用 |

**领域附加（本项目硬约束）**

| 指标 | 用途 |
|------|------|
| FilterFieldAccuracy | LLM filter 字段 vs oracle |
| FilterCompliance@K | TopK 结果满足硬 filter 比例 |
| InfraErrorRate | embedding/Redis/LLM 失败率；**不进**质量分母 |

**002 再报（生成阶段，RAGAS 风格，001 不阻塞）**

- faithfulness / groundedness
- answer_relevance
- context_precision / context_recall（LLM judge，无 gold 时）

## Eval matrix（正交）

```
filter_mode ∈ { none, oracle, llm }
retriever   ∈ { dense, hybrid }
```

- 生产 baseline：`llm + dense`（及 `llm + hybrid` 对照）
- 诊断 only：`none + dense`
- 隔离 retriever：`oracle + dense/hybrid`

## Workflow

1. **Fix prerequisites** — Task 0：维度契约、删 IngestShop 旁路、锁 Redis 镜像 digest
2. **Golden set** — `rag-evals/golden/retrieval.v1.json`；记录 SHA-256；smoke 与 formal 分离
3. **Implement metrics** — `cmd/eval-rag/metrics.go` 纯函数；表驱动测试
4. **Shared search path** — `ShopSearchLogic`：线上 `/api/rag/chat` 与 eval 共用
5. **Run & report** — 见 [`references/local-retrieval-eval.md`](references/local-retrieval-eval.md)
6. **Knowledge capture** — 宪章 VI：结论写入 `docs/solutions/interview/`

## Commands (target state after Task 2–3)

```bash
# 冒烟（7 条，非 baseline）
LLM_API_KEY=... go run ./cmd/eval-rag --test-set=script/rag-eval.json

# 正式检索评测（示例，以实现为准）
make eval-rag FILTER_MODE=llm RETRIEVER=dense
make eval-rag-compare   # dense vs hybrid，写 rag-evals/baseline/
```

## Report metadata（对外数字必带）

dataset version + SHA-256；seed/index schema；Redis image digest；embedding/chat/filter 模型与维度；filter_mode；retriever；TopK；RRF 参数；`n_total` / `n_evaluated` / `n_infra_error`。

百分点写法：`HitRate@5 42.0% → 61.0%：+19.0pp，相对 +45.2%，n=30`

## Troubleshooting

| 信号 | 原因 | 处理 |
|------|------|------|
| HitRate ≈ Recall 且多 relevant 题 | 指标算错 | 分开实现；见 metrics 测试 |
| eval 无 filter、线上有 filter | 路径不一致 | 走 ShopSearchLogic + filter_mode |
| Hybrid 报告与 dense 相同 | 静默退化 | 文本检索失败应显式标记 |
| 维度 1024/1536 混用 | 配置 drift | 仅 `LLM_EMBEDDING_DIM` 为真相 |

## Source of truth

| 文档 | 路径 |
|------|------|
| 规格 | `specs/001-evaluable-hybrid-retrieval/spec.md` |
| 实现计划 | `docs/plans/2026-07-11-recommend-agent-eval.md` §1.6, Task 0–3 |
| 本仓库指南 | `references/local-retrieval-eval.md` |
| NVIDIA upstream | `SKILL.md.upstream`, `references/*.nvidia-upstream` |

## Agent playbook

1. 读 spec + 本 skill + plan §1.6
2. 改 metrics 先写 `metrics_test.go` 再实现
3. 任何对外「提升 X%」先核对 report 元数据
4. 讨论评测结论时更新 `docs/solutions/`
