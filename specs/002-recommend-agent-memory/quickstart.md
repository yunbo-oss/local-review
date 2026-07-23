# Quickstart: Recommend Agent with Correctable Memory

**Feature**: `002-recommend-agent-memory`  
**Goal**: 在本地验证「记忆 merge → 有界 Agent → groundedness → Agent eval → 对照 Hybrid RAG」端到端路径。

字段与接口见 [data-model.md](./data-model.md)、[contracts/](./contracts/)。实现步骤见后续 `tasks.md`。

## Prerequisites

- **001 已完成**：`ShopSearchLogic`、Hybrid 默认、`make eval-rag-prod` 可跑
- Docker：MySQL + Redis Stack `7.4.0-v8`
- `.env`：`LLM_API_KEY`、chat 模型支持 **function calling**（实现 Task 6 后 smoke）
- 种子：`make seed` + `make seed-vector`
- 可选 baseline：`rag-evals/baseline/hybrid_prod_v1.json`（`make eval-rag-prod --write-baseline`）
- 测试用户 JWT：`EVAL_TOKEN` 或 demo 脚本登录获取

## Setup

```bash
cp .env.example .env
docker compose up -d
make seed && make seed-vector
make run    # http://localhost:8088
```

## Validation scenarios

### 1. Profile merge 单测（Task 4，无需 LLM）

```bash
go test ./internal/memory/... -count=1 -v
go test ./internal/repository/... -run Memory -count=1
```

**期望**: add/remove 冲突、budget 三态、filter 补空、CAS 冲突重试表驱动 PASS。

### 2. Agent graders 单测（Task 5，无需 Agent 服务）

```bash
go test ./cmd/eval-agent/... -count=1 -v
python3 -c "import json; json.load(open('rag-evals/golden/agent.v1.json')); print('ok')"
```

**期望**: groundedness/outcome/trajectory 纯函数 PASS；golden JSON 可解析；10～15 cases。

### 3. Bounded loop 单测（Task 6，fake LLM）

```bash
go test ./internal/agent/... ./internal/logic/... -run 'Agent|Recommend' -count=1
```

**期望**: 预算截断、duplicate 拒绝、grounding error、tool error 反馈 PASS；不访问真实 Redis/LLM。

### 4. Agent API 冒烟（Task 7）

```bash
export TOKEN="..."   # 登录态 JWT
curl -N -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"question":"海淀安静一点的咖啡","session_id":"qs1"}' \
  http://localhost:8088/api/agent/recommend
```

**期望**: SSE 含 `status` → `message` → `done`；回答中 `[shop:{id}]` 均来自真实检索；Jaeger/OTel 可见 agent/tool/search spans（无隐私正文）。

**负例**:
- 无 Token → 401
- 空 `session_id` → 400
- 超限频 → 429

### 5. 三轮记忆 demo（Task 8）

```bash
make demo-agent
# 或：chmod +x script/agent-demo.sh && ./script/agent-demo.sh
```

**期望**（SC-004）:
1. 「我在海淀，人均100以内，喜欢安静咖啡」→ profile 写入
2. 同 session「那就推荐两家」→ 体现区域/预算补全
3. 「忘掉预算，我现在也可以吃辣」→ budget 清空 / dislikes 更新
4. 打印脱敏 profile 摘要 + trace_id

### 6. 正式 Agent eval（Task 8）

```bash
make eval-agent
# 等价：
go run ./cmd/eval-agent \
  --test-set=rag-evals/golden/agent.v1.json \
  --out=rag-evals/reports/agent_latest.json \
  --compare-baseline=rag-evals/baseline/hybrid_prod_v1.json
```

**期望**（SC-001 / SC-002 / SC-003）:
- 报告含 outcome / groundedness / trajectory / trial 汇总 / 成本摘要 / 实验元数据
- 成功口径 groundedness 100%
- ≥5 关键 case 各 ≥3 trial
- `n_infra_error=0`、退出码 0

### 7. Agent vs Hybrid RAG 对照（SC-005）

1. 固定 `retrieval.v1` / `agent.v1` 指纹与模型配置
2. 已有 `hybrid_prod_v1.json`（001）
3. 对比 subset 上任务成功与 P50 延迟或 tool_calls

**期望**: 对照文档/报告章节标题为 **Agent vs Hybrid RAG**；无 dense vs hybrid 数字。

### 8. 全量回归

```bash
go test ./... -count=1
make eval-rag-prod    # 001 不退化
make eval-agent
make demo-agent
```

**期望**: 全绿；`doc/AGENT_AND_EVAL.md` 可独立复现。

## Out of scope for this quickstart

- 用户事实向量记忆（Phase 2）
- Mem0 / LangGraph / Milvus
- LLM judge 作 merge 或 eval 硬门禁
- dense vs hybrid 对照

## Knowledge capture (constitution VI)

实现完成后更新 `docs/solutions/interview/`：
- `记忆为何不暴露为工具`
- `有界Agent与停止条件`
- `Agent评测-outcome-groundedness-trajectory`
- `Agent-vs-Hybrid-RAG-对照怎么讲`

并在 `docs/solutions/README.md` 登记索引。
