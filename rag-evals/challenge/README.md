# 版本化 Challenge Set

这组数据与 `rag-evals/golden/*.v2.json` 的职责不同：v2 是已经参与修复、每次改动都要保持通过的回归集；v3 是首个冻结 challenge；v3.1 吸收已确认的系统、golden 和 grader 修复；v4 是旧 tool-loop 的冻结结果；v5 评测统一理解、规划、证据与记忆；v6 专门冻结 Parallel ReAct、评论续查和最终证据门禁；v6.1 是 v6 执行后建立的可检查 Router E2E regression。

## 数据规模与边界

- Retrieval v3：24 个 `dev`、120 个冻结 `challenge`。
- Agent v3 / v3.1 / v4：每版 8 个 `dev`、28 个 `challenge`；10 个关键 challenge 场景各运行 3 次，共 48 个 challenge trial。
- Agent v5：8 个 `dev`、40 个 `challenge`；8 个关键 challenge 场景各运行 3 次，共 56 个 challenge trial。重新纳入错别字，并完整保留注入、无结果、claim 和隔离场景族。
- Agent v6：规模同 v5；每个 case 额外断言 `v2_react`、4 turns / 10 tools、最多 2 次搜索、每店最多 2 页评价和 `answer_verified=true`。
- Agent v6.1：规模同 v6，复用其任务族但不注入 V2 runtime 专属断言；用于评估生产 Router 到用户结果，允许根据共同失败机制迭代。
- Router v2：12 个 `dev`、52 个冻结 `challenge`；四类路由各 13 题，并包含 force override、缺失历史和引用注入边界。
- 底层使用固定 seed 的 200 家店铺、1000 条评价。挑战的是表达和任务分布，不代表生产流量或容量规模。
- v4 按当前评测范围移除错别字题，保留语义偏好、难负样本、无结果、评论冲突、长上下文、记忆纠正和提示注入文本。
- `dev` 可用于调试 harness、prompt 和评分规则；`challenge` 只在代码、prompt、grader 和数据冻结后运行。
- 文件在仓库中可见，所以它是可复现的 repository-visible holdout，不是保密 benchmark。
- manifest 的 `formal_evaluation_executed_at_generation=false` 表示生成命令本身不调用模型；正式执行证据在包含运行时元数据的 report 中。

## 生成与检查

```bash
go run ./cmd/generate-challenge-data --check
go run ./cmd/generate-challenge-data --suite=v31 --check
go run ./cmd/generate-challenge-data --suite=v4 --check
go run ./cmd/generate-challenge-data --suite=v5 --check
go run ./cmd/generate-challenge-data --suite=v6 --check
go run ./cmd/generate-challenge-data --suite=v61 --check

# Router v2 是人工标注文件，直接运行 schema/评测校验
make eval-router-challenge
```

## 使用协议

开发期间只跑 dev 或 v3.1 regression：

```bash
go run ./cmd/eval-agent \
  --test-set=rag-evals/challenge/agent.v3.1.json \
  --split=dev --force-route=agent_multistep
```

冻结后运行 v4；Agent 与 Hybrid 必须使用同一数据和 trial 计划：

```bash
LLM_API_KEY='你的密钥' make docker-challenge-v4
```

正式输出：

- `rag-evals/reports/agent_challenge_v4.json`
- `rag-evals/baseline/hybrid_task_challenge_v4.json`

challenge 失败后应保留原始报告，将人工确认的失败模式加入下一版 regression，再换 seed 和表达生成下一版 holdout。不能修改当前 challenge 后继续沿用同一版本名。

v5 正式运行：

```bash
LLM_API_KEY='你的密钥' make docker-challenge-v5
```

生成器本身不调用模型；`rag-evals/reports/agent_v5_harness_fake.json` 仅验证 40 个 challenge case 的解析、评分和报告链路，不能作为模型质量结果。

v6 正式运行：

```bash
LLM_API_KEY='你的密钥' make docker-challenge-v6
```

v6 的 fake run 同样只用于检查 schema、grader 和聚合器；正式简历数字必须来自 `mode=inprocess` 报告，并同时保留同任务 Hybrid baseline。

Router E2E regression 不要求对照组：

```bash
LLM_API_KEY='你的密钥' make docker-agent-e2e-v61
```

其中 `outcome` 是端到端任务正确率；只有实际进入 `agent_multistep` / `agent_memory` 的 trial 才应用 Agent 工具与 runtime trajectory 合约，RAG/clarify 路由不能因为“没有调用 Agent 工具”被扣任务分。

当前 runner 能隔离 case/trial 的 session 和 profile，但尚不能在一个 case 内切换两个真实用户 ID；跨会话隔离使用互斥的成对 `setup_profile` 检查，真正的跨用户污染测试仍需要专用 runner。
