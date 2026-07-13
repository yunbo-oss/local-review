# local-review-go 检索评测指南

> 改编自 NVIDIA `rag-eval` skill 的分层思想，适配本仓库 `cmd/eval-rag` + `rag-evals/` 布局。
> Upstream: https://github.com/NVIDIA/skills/tree/main/skills/rag-eval (v2.6.0)

## 1. 评测分层（为何 001 只做检索）

| 层 | 001 本规格 | 002 后续 | 参考 |
|----|------------|----------|------|
| 检索 | HitRate, Recall, Precision, MRR, nDCG@K + filter | — | BEIR, MTEB, Redis blog |
| 生成 | — | faithfulness, answer_relevance | RAGAS / NVIDIA eval |
| 基础设施 | InfraErrorRate, 实验元数据 | OTel spans | OpenAI agent evals |

**教训（FutureAGI 2026）**：只评生成不评检索，会出现「回答听起来对但 context recall 已掉 30%」的静默回归。

## 2. 指标公式（二元 relevant，shop_id 级）

设 TopK 结果为 `[d1..dK]`，relevant 集合为 `R`，|R| = n。

- **HitRate@K** = 1 if ∃ di ∈ R else 0（查询级；报告取平均）
- **Recall@K** = |{di} ∩ R| / n
- **Precision@K** = |{di} ∩ R| / K
- **MRR** = 1/rank(first relevant)，无 relevant 则 0
- **nDCG@K** = DCG@K / IDCG@K，二元 relevant 时 rel(di)=1 if di∈R else 0

**禁止**：把 HitRate 平均后标为 Recall。

## 3. Golden set 布局

```text
rag-evals/
  golden/
    retrieval.v1.json    # 正式集
  baseline/
    dense_prod_v1.json
    hybrid_prod_v1.json
  reports/               # gitignore
```

单条 case 最小字段：

```json
{
  "id": "r001",
  "split": "test",
  "question": "...",
  "relevant_shop_ids": [5, 7],
  "oracle_filter": {"area": "朝阳区", "typeName": "美食"},
  "tags": ["semantic", "area"],
  "evidence": "seed 中可核验依据"
}
```

## 4. 与 NVIDIA upstream 的差异

| NVIDIA rag-eval | local-review-go |
|-----------------|-----------------|
| corpus/ + train.json | retrieval.v1.json（shop ids） |
| evaluate_rag.py + RAGAS | cmd/eval-rag（Go，确定性 IR 指标） |
| NVIDIA_API_KEY judge | 001 不用 LLM judge；002 可选 |
| HTTP RAG server :8081 | ShopSearchLogic in-process |

Upstream 文件备份：`references/*.nvidia-upstream`、`SKILL.md.upstream`。

## 5. 部署环境建议

**推荐（001 评测）**：开发机 Docker Compose（MySQL + Redis Stack + 本地 Go eval）。

**不推荐作为主评测环境**：AidLux 犀牛派/犀牛鸟应用

- 社区版 AidLux **不支持 Docker**（官方 FAQ）
- 全栈含 RocketMQ + 多 Go 实例对 8GB 边缘板卡过重
- RAG eval 强依赖 **可复现** digest/seed/API；边缘环境增加变量
- 秒杀/RocketMQ 与检索 eval **无关**，eval 只需 MySQL+Redis+LLM API

若仅有 AidLux 企业版 + Docker：可跑 **精简栈**（mysql + redis-stack + eval-rag），仍建议在 Mac/CI 记录 baseline，边缘机只做演示。

**腾讯「犀牛鸟」**：高校科研合作计划，非应用部署平台——与「跑评测」无关。

## 6. 面试口述模板

> 检索评测按业界分层：001 用 HitRate/Recall/Precision/MRR/nDCG 与 filter 合规，保证与线上同路径；002 再加 groundedness。Hybrid 对照必须带 dataset hash 和 n，失败 case 也要展示。
