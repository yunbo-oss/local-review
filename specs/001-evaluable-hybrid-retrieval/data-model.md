# Data Model: Evaluable Hybrid Shop Retrieval

**Feature**: `001-evaluable-hybrid-retrieval`  
**Date**: 2026-07-21

本规格以**评测与检索契约数据**为主；店铺/点评实体沿用现有 MySQL model，向量文档沿用 Redis Hash。

## 1. RetrievalCase（检索评测题）

正式集文件：`rag-evals/golden/retrieval.v1.json`。

| 字段 | 类型 | 约束 |
|------|------|------|
| `id` | string | 唯一，如 `r001` |
| `split` | string | `dev` \| `test`；test 冻结后不得因跑分失败改标 |
| `question` | string | 非空 |
| `relevant_shop_ids` | []int64 | **非空**；加载时拒绝空列表 |
| `oracle_filter` | object | 人工给定硬过滤；可为空对象（表示无硬约束） |
| `tags` | []string | 如 `semantic` / `area` / `lexical` / `no_result` |
| `evidence` | string | **非空**；人工核验依据 |

**文件级**:

| 字段 | 说明 |
|------|------|
| `version` | 如 `retrieval.v1` |
| `cases` | RetrievalCase 数组；全集 25～35 |

**Smoke 兼容**（`script/rag-eval.json`）：JSON 数组，元素仅 `question` + `expected_shop_ids`；映射为 RetrievalCase 时无 oracle/evidence/split，标记为 smoke。

### Validation rules

- 相关店铺必须存在于当前 seed catalog；证据须可核对
- LLM 同义改写题目只许进入 `dev`
- 标注错误：升级 dataset version + 变更说明，禁止偷偷改 test 刷分

### Relationships

- RetrievalCase.`relevant_shop_ids` → Shop（MySQL / 向量 doc）
- RetrievalCase.`oracle_filter` → FilterConditions

---

## 2. FilterConditions（过滤条件）

与现有 `VectorSearchFilter` 对齐（评测 JSON 可用 camelCase，入库映射到 Go 字段）。

| 字段 | 含义 | 比较规则（FilterFieldAccuracy） |
|------|------|--------------------------------|
| `area` / `Area` | 区域 TAG | 字符串精确匹配；双方皆空算一致 |
| `type_name` / `TypeName` | 类型 TAG | 同上 |
| `max_price` / `MaxPrice` | 人均上限 | 默认精确相等；0 = 未设 |
| `min_price` / `MinPrice` | 人均下限 | 同上 |
| `min_score` / `MinScore` | 评分下限 | 同上 |
| `min_comments` / `MinComments` | 评论数下限 | 同上 |
| `max_distance` / `MaxDistance` | 语义距离阈值 | 线上可选；oracle 题通常不设 |

**来源优先级（001）**: 显式请求 > LLM 抽取 > 无。**不含** profile（002）。

---

## 3. ShopCandidate / ShopSearchResult（店铺候选）

Logic / Repo 返回的有序候选（HitRate/Recall/MRR/nDCG/FilterCompliance 的计算对象）。

| 字段 | 说明 |
|------|------|
| `ShopID` | 主键 |
| `Name`, `TypeName`, `Area` | 展示与合规检查 |
| `TextContent` | Prompt 上下文 |
| `AvgPrice`, `Score`, `Comments`, `Sold` | 硬过滤合规与展示（扩展 RETURN） |
| `Score`（距离）或融合分 | KNN 距离或 RRF 后排序位置由列表顺序表达 |

**State**: 无持久化状态机；单次请求内有序列表。

---

## 4. ShopVectorDoc（向量文档，已有）

Redis key：`vec:shop:{id}`；索引：`idx:shop:vector`。

| 字段 | 索引类型 | 用途 |
|------|----------|------|
| name | TEXT WEIGHT 5.0 | Hybrid 文本路 |
| text_content | TEXT | Hybrid 文本路 |
| type_name / area | TAG | 预过滤 |
| avg_price / score / comments / sold | NUMERIC | 预过滤 / 合规 |
| embedding | VECTOR HNSW FLOAT32 COSINE | Dense 路 |

写入路径：**仅** seed-vector + MQ RAG 消费者（完整字段）。删除不完整 `IngestShop` 旁路。

---

## 5. RetrievalReport（检索评测报告）

| 字段 | 说明 |
|------|------|
| `dataset_version` | 如 `retrieval.v1` |
| `dataset_sha256` | 内容指纹 |
| `seed_version` / `index_schema_version` / `redis_image` | 可复现元数据 |
| `embedding_model` / `embedding_dim` / `filter_model` | 模型契约 |
| `filter_mode` | `none` \| `oracle` \| `llm` |
| `retriever` | `hybrid`（默认）\| `dense` |
| `top_k` / `rrf_k` / 文本与向量候选宽度 | 融合参数 |
| `n_total` / `n_evaluated` / `n_infra_error` | 样本与失败 |
| 指标字段 | HitRate@K、Recall@K、Precision@K、MRR、nDCG@K、Filter*、InfraErrorRate |
| `per_case` | 逐题结果与错误摘要 |
| `is_smoke` | 若 true 禁止作为对外 baseline |

---

## 6. ExperimentBaseline（实验基线）

| 字段 | 说明 |
|------|------|
| 标识 | `hybrid_prod` / 文件 `rag-evals/baseline/hybrid_prod_v1.json` |
| 冻结条件 | 正式 test 划分 + `filter_mode=llm` + `retriever=hybrid` + `n_infra_error=0` |
| 内容 | 完整 RetrievalReport 快照（供 002 Agent 对照） |

**不做**: `dense_prod` 基线交付。

---

## 7. State transitions（题目集）

```text
draft case → human verified (evidence 非空)
         → assigned split=dev|test
         → (test) FROZEN
发现标注错误 → bump dataset version + changelog → 新冻结（禁止 silent edit test）
```
