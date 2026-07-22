# Research: Evaluable Hybrid Shop Retrieval

**Feature**: `001-evaluable-hybrid-retrieval`  
**Date**: 2026-07-21  
**Status**: All Technical Context unknowns resolved (no remaining NEEDS CLARIFICATION)

## 1. Embedding dimension single source of truth

**Decision**: `LLM_EMBEDDING_DIM` / `llm.Config.EmbeddingDim` 为唯一维度来源；`InitShopVectorIndex` 在 `dim <= 0` 时返回错误（删除 1536 静默 fallback）；`Embed` / `EmbedBatch` 校验 `len(vec) == EmbeddingDim`，不一致则失败，禁止截断或改写。

**Rationale**: 当前默认配置 1024（`.env.example` / BGE）与索引层 fallback 1536 并存，会导致「评测能跑但结果不可信」。Spec FR-006 / SC-004 要求 fail-fast。

**Alternatives considered**:
- 索引层按模型名猜维度 → 隐式、难复现
- 静默 truncate/pad → 破坏语义，禁止

**Implement note**: 生产默认仍以环境变量为准（常见 BGE-zh 1024）；未设 env 时保持现有 client 默认 1024，但必须与实际模型返回一致，否则失败。

---

## 2. Shared search entry boundary (`ShopSearchLogic`)

**Decision**:
- **`ShopSearchLogic.Search(ctx, query, filter, strategy, topK)`**：给定已解析 filter + 策略，执行 Embed →（dense 或 hybrid）→ 有序 `[]ShopSearchResult`。
- **Filter 解析**（`ResolveFilter` / `FilterExtractor`）：显式请求条件优先；否则 LLM 抽取（复用现有 `extractFilterFromQuestion` 抽成可注入接口）。001 **不含** profile 补全（002）。
- Eval `filter-mode`：`none`（filter=nil）/ `oracle`（题面 `oracle_filter`）/ `llm`（同一 Extractor）。
- `/api/rag/chat`：显式 body filter → 否则 LLM；再调 `ShopSearchLogic`（默认 hybrid）。

**Rationale**: Spec FR-005 / US4；源计划 §1.1。分离「解析」与「检索」便于 oracle 隔离 filter 噪声，且 002 可只在 Resolve 层加 profile。

**Alternatives considered**:
- Eval 继续直连 `VectorRepo` → 路径分裂，否决
- 把 LLM filter 硬编码进 Search → 无法做 oracle/none 正交矩阵

---

## 3. Hybrid fusion: client RRF vs Redis `FT.HYBRID`

**Decision**: 客户端 RRF：Dense KNN Top20 ∥ TEXT 检索 Top20 → `FuseRRF(k=60)` → 截断 TopK（默认 5）。查询字段：`name`（WEIGHT 5.0）+ `text_content`；TEXT 路 `FT.SEARCH` 显式 `SCORER BM25`（钉死 redis-stack 7.4 可用名，避免依赖默认 TFIDF）。平局按 `shop_id` 升序打破，保证可复现。默认 `RAG_RETRIEVER=hybrid`；`--retriever=dense` 仅诊断。

**Rationale**: [Redis FT.HYBRID](https://redis.io/docs/latest/commands/ft.hybrid/) 依赖较新能力；客户端 RRF（Cormack et al. 常用 k=60）可单测、与当前 Redis Stack 兼容。源计划明确不默认 LLM rerank。

**Alternatives considered**:
- 服务端 `FT.HYBRID` → 版本绑定 + 难测
- 加权分数线性融合 → 需校准两路分数尺度；RRF 更稳
- 生成式 rerank → 慢、非确定、贵；YAGNI

**Failure policy**: 文本子检索错误 → 整次 Search 返回 error（线上失败 / eval 记 infra）；禁止静默 dense 却标 hybrid（FR-009 / Edge Cases）。

---

## 4. Formal vs smoke datasets

**Decision**:
- 正式：`rag-evals/golden/retrieval.v1.json`，25～35 条，字段含 `id/split/question/relevant_shop_ids/oracle_filter/tags/evidence`；空 `relevant_shop_ids` 加载时拒绝。
- Smoke：保留 `script/rag-eval.json`（7 条数组）；loader 兼容旧格式，仅允许 `filter-mode=none`，报告标记 smoke，**不得** `--write-baseline`。
- Dataset SHA-256 运行时计算写入 report，不手工写回 golden 文件。

**Rationale**: FR-001/002/SC-005/SC-007；Anthropic eval 实践倾向小而高质量任务集（见源计划 §0 引用）。

**Alternatives considered**: 批量 LLM 同义改写膨胀 test → 统计虚胖，禁止（仅允许进 `dev`）。

---

## 5. Metric bundle and infra denominator

**Decision**: 实现纯函数指标（参考 `.cursor/skills/rag-eval/SKILL.md` + [Redis RAG metrics](https://redis.io/blog/rag-metrics/)）：

| 指标 | 定义要点 |
|------|----------|
| HitRate@K | TopK 是否 ≥1 relevant（Success@K） |
| Recall@K | \|命中 relevant\| / \|relevant\| |
| Precision@K | \|命中 relevant\| / K |
| MRR | 首个 relevant 排名倒数均值 |
| nDCG@K | 二元 relevant |
| FilterFieldAccuracy | LLM vs oracle 逐字段；双方皆空算一致 |
| FilterCompliance@K | TopK 满足硬约束占比；无硬约束记 1.0 |
| InfraErrorRate | embed/Redis/filter LLM 失败；**不进**质量分母 |

正式评测：`n_infra_error > 0` → 非零退出，仍写 report。

**Rationale**: 修复现 `cmd/eval-rag` 把 HitRate 标成 Recall 的错误；满足 FR-003/011。

**Alternatives considered**: RAGAS 生成侧指标 → 规格 `002`（FR-003b）。

---

## 6. Redis Stack image pinning

**Decision**: 实现 Task 0 时：
1. 优先对**本机已验证可跑向量检索**的镜像执行 `docker image inspect … --format '{{index .RepoDigests 0}}'`，将 `docker-compose.yml` 的 `latest` 换为 **digest 或等价固定 tag**；
2. 若本地无镜像，拉取并验证后锁定 **`redis/redis-stack-server:7.4.0-v8`**（[Docker Hub / GitHub release v7.4.0-v8](https://github.com/redis-stack/redis-stack/releases)，Stack 维护更新已于 2025-12 停止，本规格不换 Redis 8 OSS，避免扩大范围）；
3. 将选用值写入实验说明 / report `redis_image` 字段。

**Rationale**: FR-008；`latest` 不可复现。

**Alternatives considered**: 立刻迁 Redis 8 OSS → 超出 001 范围（YAGNI）。

---

## 7. Incomplete ingest path

**Decision**: 删除未调用的 `RAGLogic.IngestShop`（缺 AvgPrice/Score/Comments/Sold）。正式写入仅 `cmd/seed-vector` 与 MQ `shop-update` RAG 消费者的完整 `ShopVectorDoc`。

**Rationale**: FR-007；避免错误旁路被误用。

**Alternatives considered**: 修补 IngestShop 补全字段 → 无调用方，删除更简单。

---

## 8. Eval matrix vs product narrative (post 2026-07-21)

**Decision**:
- **交付/验收**：默认 hybrid；录 `hybrid_prod`（`llm + hybrid`）；可选 `oracle + hybrid` 隔离 filter；dense **不**要求报告与简历。
- **CLI 仍保留** `--retriever=dense` 供排障。
- 同步修正 `.cursor/skills/rag-eval/SKILL.md` 中仍写「生产 baseline：llm+dense 及对照」的过时表述（实现期文档任务）。

**Rationale**: Spec Updated 2026-07-21；面试位置留给 Agent vs Hybrid RAG（002）。

**Alternatives considered**: 强制 dense A/B → 已明确砍掉。

---

## 9. Online / eval TopK consistency verification

**Decision**: 对齐测试 = 同一 `ShopSearchLogic` 实例路径：对 ≥10 道抽样题，固定 `question + explicit/oracle filter + hybrid + topK`，比较有序 shop_id 列表；评测期间索引只读。不必起双进程对打 HTTP（可用 in-process Logic；可选再对 `/api/rag/chat` 内部检索结果做集成抽检）。

**Rationale**: SC-003；共享入口是充分条件。

**Alternatives considered**: 仅文档约定「应该一致」而无测试 → 不足。

---

## 10. `ShopSearchResult` metadata for FilterCompliance

**Decision**: 扩展 `ShopSearchResult`（及 RediSearch `RETURN`）带回 `AvgPrice/Score/Comments/Sold`（及既有 Name/Area/TypeName），否则无法在不二次查 MySQL 的情况下计算 FilterCompliance@K。

**Rationale**: 源计划 Task 2 Step 3；评测与后续 Agent 展示权威元数据。

**Alternatives considered**: 评测再查 MySQL → 路径分叉、多一次 infra 面。
