# Contract: eval-agent & demo-agent

**Phase**: B | **Extends**: `specs/002-recommend-agent-memory/contracts/eval-agent-cli.md`

## eval-agent

### Purpose

对 `rag-evals/golden/agent.v1.json` 跑**真实**推荐（Harness 或 HTTP），确定性评分，输出非 stub 报告；可选对照 Hybrid 基线。

### Invocation

```bash
make eval-agent
# go run ./cmd/eval-agent \
#   --test-set=rag-evals/golden/agent.v1.json \
#   --out=rag-evals/reports/agent_latest.json \
#   --compare-baseline=rag-evals/baseline/hybrid_prod_v1.json \
#   --trials=3
```

可选：`--force-route=agent_multistep`（全题强制，用于对照路由开启模式）。

### Harness behavior

1. 校验 golden；计算 `dataset_hash`（SHA-256）  
2. 每 case × trial：独立 `session_id`；`setup_profile`；顺序 turns  
3. 捕获：answer、citations、observed/evidence、trajectory、profile_after、latency、tokens、route  
4. Graders：`GradeOutcome` / `GradeGroundedness` / `GradeTrajectory`（可扩展 evidence 规则）  
5. 关键 case ≥3 trial → 汇总（含跨 trial 成功率 / pass^k 等价字段）  
6. `n_infra_error` 单列；质量分母不含 infra  
7. `comparison`：Agent vs Hybrid RAG（分 tag outcome Δ + 延迟或调用量）；**无** dense 字段  
8. 报告 `version` 不得为 `*-stub`

### Exit

- `0`：harness 完成且配置合法（infra>0 可非零，以实现约定为准；规格要求 infra 可观测）  
- 非零：fatal 配置 / 无法读 golden

### Resume metrics（报告须能支撑）

- 主：分 tag outcome、pass^k、P50/token  
- 配角：成功口径 groundedness、trajectory  

---

## demo-agent

### Purpose

三轮记忆可复现演示（非打分）。

### Invocation

```bash
make demo-agent
# ./script/agent-demo.sh
```

### Flow

1. 登录拿 JWT  
2. 「海淀 + 预算…」→ 写偏好  
3. 同 session 追问推荐 → 观察补全  
4. 「忘掉预算…」→ 观察纠正  
5. 打印脱敏 profile + `trace_id`

### Expect

抽检 3 次全过（SC-B04）；不泄漏完整评价/密钥。
