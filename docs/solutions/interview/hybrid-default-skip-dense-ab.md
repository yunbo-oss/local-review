# Hybrid 默认上线，跳过 dense 对照

- **日期**: 2026-07-21
- **模块**: RAG 检索 / 001-evaluable-hybrid-retrieval
- **标签**: `interview` `rag` `eval` `tradeoff`

## 背景

计划原要求 dense vs hybrid 同题对照并写入简历。维护者反馈面试篇幅有限，希望数字留给 Agent。

## 决策

| 做 | 不做 |
|----|------|
| 线上 / eval 默认 Hybrid RRF | dense↔hybrid 正式 A/B 与简历叙事 |
| 录 `hybrid_prod` 作 Agent 对照基线 | 为检索策略切换单独占一栏简历 |
| 正确指标 + golden + 共享入口 | 省略全部检索评测（仍要 Hybrid 基线） |

## 面试怎么说

> Hybrid 是词面+向量的业界默认组合，我们直接上生产路径。评测预算留给「单次 Hybrid RAG vs Agent」——那才是产品层要不要多步工具的决策。

## 风险

若 Hybrid 实现有 bug，没有 dense 对照会更难归因。缓解：RRF/文本检索单测 + Hybrid 基线 smoke；Agent 对比时若全面变差先查检索。

## 相关文档

- `docs/plans/2026-07-11-recommend-agent-eval.md`（2026-07-21 取舍）
- `specs/001-evaluable-hybrid-retrieval/spec.md`
