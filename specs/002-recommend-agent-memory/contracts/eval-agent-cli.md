# Contract: eval-agent CLI

**Binary**: `cmd/eval-agent`  
**Makefile target**: `eval-agent`（待添加）  
**Date**: 2026-07-23

## Purpose

对 `agent.v1.json` 运行推荐 Agent，输出确定性评分报告，支持与 Hybrid RAG 基线对照（Spec FR-010–013 / US4）。

## Invocation

```bash
go run ./cmd/eval-agent \
  --test-set=rag-evals/golden/agent.v1.json \
  --out=rag-evals/reports/agent_latest.json

# 可选
#   --split=test
#   --base-url=http://localhost:8088
#   --token=$TOKEN
#   --trials=3          # 覆盖 case 级 trials（关键 case 默认 ≥3）
#   --compare-baseline=rag-evals/baseline/hybrid_prod_v1.json
```

## Flags

| Flag | Default | 说明 |
|------|---------|------|
| `--test-set` | `rag-evals/golden/agent.v1.json` | 场景集路径 |
| `--out` | stdout | 报告 JSON 路径 |
| `--split` | all | `dev` / `test` / all |
| `--base-url` | `http://localhost:8088` | Agent API |
| `--token` | env `EVAL_TOKEN` | Bearer JWT |
| `--compare-baseline` | optional | 001 hybrid_prod 报告路径 |

## Harness behavior

1. 对每个 case × trial：
   - 生成唯一 `session_id`
   - 预写 `setup_profile`（若存在）
   - 顺序发送 `turns[].user` 到 `POST /api/agent/recommend`（或 in-process logic 模式用于单测）
   - 捕获 SSE events、tool trace（若 API 不暴露 trace，则 logic 层 hook 或 eval 内嵌 Run）
   - 读取最终 profile
2. 运行 graders：`GradeOutcome`, `GradeGroundedness`, `GradeTrajectory`
3. 关键 case（`trials≥3`）→ `AggregateTrials`
4. 写 report + 实验元数据

## Report schema (summary)

```json
{
  "version": "agent-eval.v1",
  "dataset_hash": "sha256:...",
  "experiment": {
    "chat_model": "...",
    "agent_max_steps": 3,
    "agent_max_tool_calls": 5
  },
  "summary": {
    "outcome_rate": 0.0,
    "groundedness_rate": 0.0,
    "trajectory_pass_rate": 0.0,
    "trial_consistency": {},
    "p50_latency_ms": 0,
    "p95_latency_ms": 0,
    "avg_tool_calls": 0.0,
    "avg_tokens": 0.0
  },
  "n_total": 0,
  "n_evaluated": 0,
  "n_infra_error": 0,
  "cases": [],
  "comparison": {
    "baseline_path": "rag-evals/baseline/hybrid_prod_v1.json",
    "baseline_hash": "...",
    "notes": "Agent vs Hybrid RAG"
  }
}
```

## Graders (deterministic)

| Grader | Pass 条件 |
|--------|-----------|
| Outcome | filter_contains ⊆ 实际；allowed/forbidden shops；profile_after 字段匹配 |
| Groundedness | 引用 shop_id ⊆ observed；expect_no_results 时不编造 |
| Trajectory | steps ≤ max_steps；tool_calls ≤ max；无非法 duplicate |

**非门禁**: 开放式措辞 beautify；可选 LLM rubric 仅人工参考。

## Exit code

- `0` — `n_infra_error=0` 且 harness 完成
- 非零 — infra 失败或 fatal 配置错误

## Out of scope

- LLM-as-judge CI 门禁
- dense vs hybrid 对照
- 修改 agent.v1 expected 以刷分
