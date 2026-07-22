# Quickstart: Evaluable Hybrid Shop Retrieval

**Feature**: `001-evaluable-hybrid-retrieval`  
**Goal**: 在本地 Docker Compose 上验证「契约正确 → 共享 Hybrid 检索 → 正式评测可复现」端到端路径。

详细字段与 CLI 见 [data-model.md](./data-model.md)、[contracts/](./contracts/)。实现步骤见后续 `tasks.md`。

## Prerequisites

- Docker：MySQL + Redis Stack（compose 钉死 `redis/redis-stack-server:7.4.0-v8`，非 `latest`）
- `.env`：`LLM_API_KEY`、`LLM_EMBEDDING_DIM` 与所用 embedding 模型一致
- 种子：`make seed` + `make seed-vector`（完整元数据写入）
- 正式集：`rag-evals/golden/retrieval.v1.json`（25～35，evidence 非空）
- Smoke：`script/rag-eval.json`（仅冒烟）

## Setup

```bash
cp .env.example .env   # 配置 LLM_* 与维度
docker compose up -d
make seed
make seed-vector
# 可选：启动 API 服务做线上对齐抽检
make run               # http://localhost:8088
```

维度不一致时应在 seed-vector / 启动索引时**立即失败**（可读错误），不得静默继续。

## Validation scenarios

### 1. Smoke（非 baseline）

```bash
make eval-rag-smoke
# 或：go run ./cmd/eval-rag --test-set=script/rag-eval.json --filter-mode=none --retriever=hybrid
```

**期望**: 跑通；报告标记 smoke；不得写入 `rag-evals/baseline/`。

### 2. 生产口径正式评测 + 基线

```bash
make eval-rag-prod
# 等价：filter-mode=llm retriever=hybrid --split=test
go run ./cmd/eval-rag \
  --test-set=rag-evals/golden/retrieval.v1.json \
  --filter-mode=llm --retriever=hybrid --split=test \
  --write-baseline \
  --out=rag-evals/reports/hybrid_prod.json
```

**期望**（SC-001 / SC-006）:
- 报告含 HitRate@5、Recall@5、Precision@5、MRR、nDCG@5、Filter*、InfraErrorRate 与实验元数据
- `n_infra_error=0`、进程退出码 0
- 生成 `rag-evals/baseline/hybrid_prod_v1.json`
- **不要求** dense 对照报告

### 3. 隔离 filter 抽取噪声（可选）

```bash
make eval-rag-oracle
```

**期望**: 独立报告；便于对比 llm vs oracle，仍为 hybrid。

### 4. 线上 / 评测路径一致（SC-003）

对 ≥10 题：同一 `question` + 显式/oracle filter + `hybrid` + TopK，比较 `ShopSearchLogic` 有序 shop_id 与线上对话检索结果（评测期间索引只读）。

**期望**: 有序 ID 列表 100% 一致（文案可不同）。

### 5. 维度契约（SC-004）

临时将 `LLM_EMBEDDING_DIM` 设为与模型不符的值，执行 embed 或建索引。

**期望**: 100% 失败；无错误维度写入。

### 6. 指标未混用（SC-002）

在含多 relevant 或 TopK 噪声的题目上检查报告：HitRate@5、Recall@5、Precision@5 可以不同。

### 7. 单元测试（实现后）

```bash
go test ./cmd/eval-rag/... ./internal/rag/... ./internal/logic/... ./internal/config/redis/... ./internal/llm/... -count=1
```

**期望**: PASS（含 metrics、RRF、非法 dim、空 relevant 拒绝加载）。

## Out of scope for this quickstart

- Agent / 记忆 / RAGAS 生成质量（002）
- dense vs hybrid 正式对照与简历数字
- AidLux 犀牛派录 baseline
