# RAG 检索链路拆解：ShopSearch vs RAG、Hybrid/RRF、评测指标与 Golden

- **日期**: 2026-07-22
- **类型**: interview
- **标签**: [rag, hybrid, rrf, bm25, eval, architecture, golden-set]
- **关联代码**:
  - `internal/logic/shop_search_logic.go`
  - `internal/logic/rag_logic.go`
  - `internal/rag/rrf.go`
  - `internal/repository/vector_repo.go`（`SearchText` + `SCORER BM25`）
  - `cmd/eval-rag/`（`main.go` / `metrics.go` / `types.go`）
  - `rag-evals/golden/retrieval.v1.json`
  - `script/seed.sql` / `script/rag-eval.json`
- **关联功能**: `specs/001-evaluable-hybrid-retrieval`

## 一句话结论（面试口述版）

> `RAGLogic` 管「对话编排」，`ShopSearchLogic` 管「检索决策」——二者不是两套平行检索，而是上层调用下层。生产默认 Hybrid：KNN∥TEXT（BM25）各召回 Top20，客户端等权 RRF 融合后截 TopK；`cmd/eval-rag` 与线上共用同一 Search，用 HitRate/Recall/Precision 等正确指标在基于 `seed.sql` 的 25 店 catalog 上评 30 道 golden 题。

---

## Q1. `shop_search_logic` 和 `rag_logic` 逻辑一致吗？调用关系是什么？

### 结论

**检索决策一致（共用同一入口），职责不一致（分层）。**

| 组件 | 职责 | 不做的事 |
|------|------|----------|
| `ShopSearchLogic` | Resolve 之后的检索：Embed → dense/hybrid → 有序 `[]ShopSearchResult` | 不抽 filter（调用方传入）、不生成回答、不 SSE |
| `RAGLogic` | 对话编排：抽/合并 filter → 调 Search → 拼上下文+博客 → ChatStream | 不再自己 Embed+KNN |

### 调用关系

```text
HTTP POST /api/rag/chat
        │
        ▼
   RAGLogic.ChatWithFilter
        │  1) FilterExtractor.Extract（若无显式 filter）
        │  2) ResolveFilter(explicit, extracted)
        │  3) ShopSearchLogic.Search(..., strategy=hybrid默认, topK=5)
        │  4) buildShopContext + ChatStream
        ▼
   ShopSearchLogic
        ├─ dense  → Embed → VectorRepo.SearchShops
        └─ hybrid → Embed → 并行 SearchShops ∥ SearchText(BM25) → FuseRRF

cmd/eval-rag
        │  按 --filter-mode 得到 filter（none/oracle/llm）
        └─► 同一个 ShopSearchLogic.Search（--retriever）
```

DI 在 `cmd/server/main.go`：先 `NewShopSearchLogic`，再注入 `NewRAGLogic`。  
eval CLI **不经过** `RAGLogic`，直接调 `ShopSearchLogic`，保证「评测路径 = 线上检索路径」。

### 为何不算「两份逻辑」

重构前 `RAGLogic` 内联 Embed + `SearchShops`（纯 dense）。现在检索只活在 `ShopSearchLogic`；`RAGLogic` 只保留对话侧。Filter 抽取 prompt 仍定义在 `rag_logic.go`（常量 `ragFilterExtractPrompt`），由同包的 `NewLLMFilterExtractor` 使用——生产与 `--filter-mode=llm` 共用。

---

## Q2. 目前是加了混合检索策略吗？流程名词怎么讲？

### 结论

**是。生产默认已是 Hybrid；dense 仅诊断。**

- 默认策略：`RAG_RETRIEVER` 未设或为 `hybrid` → `RetrieverHybrid`
- Hybrid 流程（**先多召回再融合，不是先取最终 TopK**）：
  1. Embed(question)
  2. 并行各取 `candidateK`（默认 **20**）：KNN（`SearchShops`）∥ TEXT（`SearchText`，`name` + `text_content`，`SCORER BM25`）
  3. `FuseRRF(k=60)` → 截断最终 `topK`（线上通常 **5**）
- 硬约束：TEXT **子检索失败（error）** → 整次 Search 返回 error，禁止静默退化成 dense 却标成 hybrid
- dense：`--retriever=dense` / `RAG_RETRIEVER=dense`，不录正式 `hybrid_prod` baseline

预过滤（area/type/price/score/comments）两路共用同一 `VectorSearchFilter`。

### 名词速记

| 名词 | 含义 |
|------|------|
| **Dense / KNN** | 问句 embedding 后向量近邻；适合「意思近、用词不同」 |
| **TEXT / BM25** | RediSearch 全文相关排序；适合店名、品类等词面 |
| **Hybrid** | 两路都跑再融合；不是只跑一路 |
| **candidateK → topK** | 默认 20→5：先扩召回池，再 RRF 截断给下游 |

### 「一路没有」怎么处理（失败 ≠ 空结果）

| 情况 | 行为 |
|------|------|
| 任一路 **报错** | 整次 Hybrid **error**（含 TEXT 空 query） |
| 一路成功但 **返回空列表** | 仍进 RRF，等价于只用另一路排名 |
| 某店 **只在一路出现** | 保留；只累加该路 \(1/(60+\mathrm{rank}+1)\)，通常低于两路都靠前的店 |

---

## Q3. `rrf.go` 详解：怎么算？为何只按排名？比例怎么答？

### 算法（本仓库实现）

对多路有序列表中的每个文档：

\[
\mathrm{score}(d)=\sum_{list\ i}\frac{1}{rrfK + \mathrm{rank}_i(d) + 1}
\]

- `rank` 为 **0-based** 下标；公式里 `+1` 变成常见的 1-based 排名。
- 默认 `rrfK = 60`（Cormack et al. 常用常数：缓和「只看第一名」的极端，又不过分拉平）。
- 按 score **降序**；平局按 `shop_id` **升序**打破，保证同输入同输出（可复现评测）。
- 截断 `topK`。

示例：dense=`[1,2,3]`，text=`[2,4,1]`，`rrfK=60`：

| shop | dense | text | 约分 | 结果 |
|------|-------|------|------|------|
| **2** | \(1/62\) | \(1/61\) | **最高** | 排第一（两路都靠前） |
| 1 | \(1/61\) | \(1/63\) | 次之 | |
| 4 | 无 | \(1/62\) | 单路 | 仍可进 TopK |
| 3 | \(1/63\) | 无 | 单路 | 仍可进 TopK |

### 为何客户端 RRF，而不是别的

| 方案 | 为何未选 / 为何选 |
|------|-------------------|
| **客户端 RRF（选用）** | 不依赖 Redis 新命令；纯函数可单测；不校准两路分数尺度 |
| 线性加权融合 | 需对齐 KNN 距离与 BM25 分，调参脆 |
| Redis `FT.HYBRID` | 绑较新能力，难单测、环境版本漂移 |
| LLM rerank | 慢、贵、非确定；001 YAGNI |

**失败策略与 RRF 正交**：融合之前 TEXT 路若报错，根本不进入 RRF——避免「hybrid 报告其实是 dense」。

### 面试追问：「只按排名融合是业界做法吗？比例怎么分？」

**是常见做法。** Cormack RRF（常 \(k=60\)）被广泛用于 hybrid（ES/OpenSearch、Azure AI Search、多数 RAG 框架：「多路召回 → RRF」）。本仓库是**标准等权 RRF**：两路各 Top20，列表级权重均为 1。

**口述口径（比例题）：**

> 我们没用 0.7×dense + 0.3×text：KNN 距离与 BM25 分尺度不同，硬配权重脆。RRF 两路地位相同，只按排名贡献；『比例』不是先验切的，而是**排名重叠**决定谁更高。可理解为列表级 50:50，但结果不是固定半 dense 半 text——真正抬分的是两路都靠前。若评测发现某类 query 系统性强偏，再上 Weighted RRF \(\sum w_i/(k+\mathrm{rank}_i)\)，用 golden 调 \(w\)，不拍脑袋。

可调旋钮（比瞎说比例专业）：`candidateK`、`rrfK`、最终 TopK、字段 WEIGHT（TEXT **路内**）、Weighted RRF / 后置 rerank（有证据再加）。

相关代码：`internal/rag/rrf.go`；调用：`shop_search_logic.searchHybrid`。

---

## Q4. TEXT 检索具体怎么算？有 score 吗？然后呢？为何钉 BM25？

### 结论

**TEXT = RediSearch `FT.SEARCH` + 显式 `SCORER BM25`；引擎内有相关分，但 hybrid 只保留排序后的 ID 列表进 RRF，不把 BM25 分与 KNN 距离混加。**

### TEXT 查询怎么拼

1. 问句按空白拆词、转义特殊字符，词之间 `|`（OR）
2. `@name|@text_content:(...)`，与预过滤 AND
3. 索引：`name` **WEIGHT 5.0**，`text_content` 默认 1.0 → 店名命中更吃香
4. `LIMIT 0, candidateK`（默认 20），按 BM25 降序返回

### BM25 原理（口述）

直觉：**稀有词权重大、词频饱和、长文略降权。** 相对朴素 TF-IDF：TF 不会线性爆、显式长度归一。落在本路：只影响 TEXT Top20 的先后顺序。

### score 有没有？然后呢

```text
Redis 对命中店算 BM25 score
  → 按分降序截 Top20
  → Go：不取 WITHSCORES；ShopSearchResult.Score 主要给 KNN vector_score（TEXT 路常为 0）
  → 抽出有序 shop_id[]
  → 与 dense ID 列表一起 FuseRRF → 截 TopK → RAG 拼 prompt
```

| 阶段 | score 是什么 | 之后干什么 |
|------|----------------|------------|
| TEXT 内部 | BM25 相关分 | 只用来排 TEXT Top20 |
| 进入 Go | 数值基本丢弃，保留顺序 | `shopIDs(textRes)` |
| 融合 | RRF 分（来自排名） | 截 Top5 返回 |

### 为何显式 BM25，而不是依赖模块默认？

compose 钉 `redis-stack-server:7.4.0-v8` 时，未声明 scorer 时常落在 **TFIDF**；更新栈可能默认变成 BM25STD → baseline 漂移。显式 `SCORER BM25`（7.4 可用名）：

- 行为钉死、可复现；面试口径清晰：「TEXT=BM25，dense=KNN，融合=RRF」
- 长 `text_content` 上 BM25 通常比裸 TFIDF 更稳一点
- **融合策略不变**：仍 RRF；改 scorer 后应重跑 hybrid 评测并更新 baseline

相关代码：`vector_repo.SearchText`（`SCORER BM25`）。

---

## Q5. 可信评测 CLI + 指标 + 题集：逻辑是什么？统计了哪些指标？

### 端到端逻辑

```text
加载题集（golden 或 smoke）
  → 校验 relevant 非空（正式还要求 evidence）
  → 按 --split 过滤（smoke 无 split）
  → 逐题：
       filter-mode: none | oracle | llm
       ShopSearchLogic.Search(retriever)
       成功 → 算质量指标；失败 → n_infra_error++（不进质量分母）
  → 写 report JSON（含 dataset sha256、模型维、redis 镜像等元数据）
  → 正式跑且 n_infra_error>0 → 非零退出
  → --write-baseline 仅正式集 + hybrid（禁止 smoke）
```

Makefile：`eval-rag-smoke` / `eval-rag-oracle` / `eval-rag-prod`。

### 质量指标（分母 = `n_evaluated`，不含 infra）

| 指标 | 本仓库定义 | 面试一句话 |
|------|------------|------------|
| **HitRate@K** | TopK 是否 ≥1 个 relevant（0/1） | 「有没有捞到至少一个对的」= Success@K |
| **Recall@K** | \|命中的 relevant\| / \|relevant\| | 「相关店里找回了多少比例」 |
| **Precision@K** | \|命中的 relevant\| / **K** | 「前 K 个里有多少比例是相关的」 |
| **MRR** | 第一个 relevant 的 1/rank（1-based） | 「第一个对的有多靠前」 |
| **nDCG@K** | 二元 relevant 的归一化折损累计增益 | 「整体排序质量」，靠前命中权重大 |

**禁止**：把 HitRate 命名/口述成 Recall。多 relevant 题上两者会分叉（测试强制覆盖）。

### 过滤与基础设施指标

| 指标 | 含义 |
|------|------|
| **FilterFieldAccuracy** | llm 抽出的 filter vs `oracle_filter` 逐字段一致率（双方皆空算一致） |
| **FilterCompliance@K** | TopK 结果满足 oracle 硬约束的占比；无硬约束记 1.0 |
| **InfraErrorRate** | embed/Redis/filter LLM 失败占比；**单列**，不稀释 HitRate |

报告字段见 `cmd/eval-rag/types.go` 的 `EvalReport`。

### 题集两种

| 集合 | 路径 | 角色 |
|------|------|------|
| Smoke | `script/rag-eval.json`（≤7，旧字段 `expected_shop_ids`） | 冒烟；仅 `--filter-mode=none`；禁止写 baseline |
| Formal | `rag-evals/golden/retrieval.v1.json` | 对外数字与 `hybrid_prod` 基线 |

---

## Q6. Golden 基于哪个测试集？多少店铺？是不是 seed？

### 结论

**是。Golden 人工标注基于本仓库 `script/seed.sql`（及其中 blog 文案），不是外部公开 IR 数据集。**

| 项 | 数量 / 说明 |
|----|-------------|
| Seed 店铺 catalog | **25 家**（`tb_shop` id 1–25） |
| Seed 类型 | 美食 / 咖啡 / 酒店 |
| Seed 点评 | `tb_blog` 提供「浪漫」「约会」等语义证据 |
| Golden 题量 | **30**（`test` 26 + `dev` 4） |
| 作为 relevant 出现过的店 | **22** 个不同 shop_id（并非每家店都当过标注答案） |
| Smoke | 从早期 7 题演进而来，与 golden 部分题面重叠，但 **非正式集** |

标注字段：`id/split/question/relevant_shop_ids/oracle_filter/tags/evidence`；`evidence` 指向 seed 店名、区域、均价或 blog 表述，保证可人工核对。

向量侧：`make seed` 灌 MySQL → `make seed-vector` 把完整 `ShopVectorDoc`（含价格评分）写入 Redis；评测假定索引与 seed 一致。

---

## 常见追问

1. **Q**: 为什么 eval 不调 `RAGLogic`？  
   **A**: `RAGLogic` 含生成与 SSE；评测要的是检索 TopK ID。共用 `ShopSearchLogic` 既对齐路径又避免生成噪声进 IR 指标。

2. **Q**: Hybrid 比纯 dense 强在哪？  
   **A**: 店名/品类等词面（「全聚德」「烤鸭」）TEXT/BM25 补召回；语义问句（「适合约会」）靠 KNN；RRF 融合两路排名，无需对齐分数尺度。

3. **Q**: 是不是先取 20 再算 RRF？  
   **A**: 是。两路各 `candidateK=20` → `FuseRRF` → 截最终 `topK`（线上常 5）。不是先只取 5 再融合。

4. **Q**: dense/text「比例」多少？  
   **A**: 无显式 7:3；等权 RRF，有效偏置由重叠排名与评测决定。见 Q3。

5. **Q**: TEXT 失败会不会偷偷变 dense？  
   **A**: 不会。TEXT error → 整次 Search error；空结果才继续 RRF。

6. **Q**: BM25 分最后用在哪？  
   **A**: 只在 Redis 内排 TEXT Top20；Go 侧不拿来和向量分加权，融合只用排名。见 Q4。

7. **Q**: 为什么质量分母不含 infra？  
   **A**: Redis/API 挂了会把 HitRate 拉低，看起来像「检索变差」。应报 `InfraErrorRate` 并让正式入口非零退出。

8. **Q**: test/dev 怎么用？  
   **A**: `test` 冻结后禁止为刷分改标；同义改写等只进 `dev`。正式 `eval-rag-prod` 默认 `--split=test`。

## 如何避免再犯 / 复习提示

- 新检索策略只加在 `ShopSearchLogic`，禁止在 Handler/eval 旁路 Embed+KNN。
- TEXT scorer 必须在 `FT.SEARCH` **显式声明**（当前 `BM25`），禁止依赖模块默认以免版本漂移。
- 改召回/融合/scorer 后重跑 hybrid 评测并更新 baseline，否则数字不可比。
- 对外数字必须带 report 元数据（dataset sha、dim、retriever、filter_mode）。
- 口述指标前先分清 HitRate vs Recall；演示链路画「RAG 编排 → Search 决策」两层即可；融合口述「等权 RRF，不是线性加权比例」。

## 参考

- 内部：`docs/solutions/interview/2026-07-21-shop-search-path-alignment.md`、`2026-07-21-hitrate-vs-recall-precision.md`、`hybrid-default-skip-dense-ab.md`
- 规格：`specs/001-evaluable-hybrid-retrieval/`（含 research：客户端 RRF + 显式 BM25）
- 外部：Cormack et al. Reciprocal Rank Fusion；Redis Search scoring（BM25）；[Redis RAG metrics](https://redis.io/blog/rag-metrics/)
