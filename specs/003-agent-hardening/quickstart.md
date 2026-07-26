# Quickstart: Agent Hardening + 002 Closure

**Goal**: 先验证 Phase A（代码可信与工程外壳），再验证 Phase B（评测/demo/对照）。  
**Spec**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md)

---

## Prerequisites

```bash
docker compose up -d
cp -n .env.example .env
go mod tidy
make seed && make seed-redis && make seed-vector   # 按环境需要
make run   # 或分布式 compose
```

需有效 `LLM_API_KEY`（tool calling）；JWT 可登录。

---

# Phase A — 代码验收（SC-A*）

> 合并门槛：下列通过即可；**不要求**非 stub `eval-agent` 报告。

### A1. 单测矩阵

```bash
go test ./internal/agent/... ./internal/logic/... ./internal/repository/... ./internal/handler/... ./internal/middleware/... -count=1
```

**期望**:
- Evidence：空引用 / 未知 ID / 空 blogs 洗白 / 事实冲突 / no-result
- Profile 搜索补空 / 覆盖 / 清空预算
- Loop：重复调用耗预算、单 turn 上限、严格 JSON
- Router：单轮 / 纠偏好 / 评价信号 / `force_route`
- SSE：无 tool raw / 完整 profile / CoT（handler 测试）
- Profile CAS / 缓存回填相关 repo 测试

### A2. 冒烟：强制 Agent 路径

```bash
# 登录后：
curl -N -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"question":"海淀安静咖啡","session_id":"qs-a2","force_route":"agent_multistep"}' \
  http://localhost:8088/api/agent/recommend
```

**期望**: SSE `status` → `message` → `done`；`done` 含 `trace_id`、`route`；引用合法。

### A3. 冒烟：路由走 RAG

```bash
curl -N -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"question":"南山人均100咖啡","session_id":"qs-a3"}' \
  http://localhost:8088/api/recommend
# 若尚未做统一入口，则验证 Router 单测 + 手动确认规则命中 rag_oneshot
```

**期望**: `done.route=rag_oneshot`（或等价）；延迟明显低于强制 Agent（定性即可）。

### A4. 负例

- 无 Token → 401  
- 空 session → 400  
- 超频 → 429（多实例共享时仍成立）  
- 校验失败 → 仅 `error`，无成功 `message`

### A5. 持久化抽检

1. 完成一次成功推荐并纠正偏好  
2. 清空 Redis profile 缓存  
3. 再次加载偏好仍存在  

**期望**: SC-A04。

### Phase A Checkpoint

- [ ] SC-A01–A08 相关自动化/冒烟通过  
- [ ] SC-A09：未将 stub eval 当作完成  
- [ ] 可选：`docs/solutions/interview/` Evidence / Router 条目  

---

# Phase B — 评测与收口（SC-B*）

> **仅在 Phase A 达标后执行。** 对外「评测闭环」以此为准。

### B1. Hybrid 基线（若缺失）

```bash
make eval-rag-prod
# 按 001 约定 --write-baseline → rag-evals/baseline/hybrid_prod_v1.json
```

### B2. 正式 Agent eval

```bash
make eval-agent
# 或带对照：
go run ./cmd/eval-agent \
  --test-set=rag-evals/golden/agent.v1.json \
  --out=rag-evals/reports/agent_latest.json \
  --compare-baseline=rag-evals/baseline/hybrid_prod_v1.json
```

**期望**（SC-B01–B03, B06）:
- 报告非 stub；含 outcome / groundedness / trajectory / trial / 成本 / dataset / experiment  
- 成功口径 groundedness 100%  
- ≥5 关键题 × ≥3 trial  
- comparison 标题语义为 **Agent vs Hybrid RAG**；分 tag 可见 Δ；无 dense 数字  
- `n_infra_error` 可观测  

### B3. Demo

```bash
make demo-agent
```

**期望**（SC-B04）: 偏好 → 补全推荐 → 纠正/清空；脱敏摘要 + `trace_id`；抽检 3 次全过。

### B4. 文档与回归

```bash
go test ./... -count=1
make eval-rag-prod
make eval-agent
make demo-agent
```

**期望**:
- `doc/AGENT_AND_EVAL.md` 可独立复现  
- README 指向 API / eval / demo  
- 勾选 `002` / `003` Interview Value 面试点（SC-B07–B08）  

### Phase B Checkpoint

- [ ] SC-B01–B08  
- [ ] 简历可用数字：分 tag outcome Δ、pass^k、P50/token；groundedness 仅作门禁一句  

---

## Out of scope for this quickstart

- HyDE / Multi-Query  
- dense vs hybrid  
- Mem0 / 多 Agent  
- 开放式文笔自动打分门禁  
