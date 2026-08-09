# v3 冻结 Challenge Set

这套数据与 `rag-evals/golden/*.v2.json` 的职责不同：v2 是已经参与过修复、每次改动都应保持通过的回归集；v3 用于衡量未见表达、长对话、安全攻击和诚实拒答能力，不把跑失败的题继续调到 100%。

## 数据规模与边界

- Retrieval：144 题，其中 24 题 `dev`、120 题冻结 `challenge`。
- Agent：36 个场景，其中 8 个 `dev`、28 个冻结 `challenge`；10 个关键 challenge 场景各运行 3 次，共 48 个 challenge trial。
- 底层仍使用 v2 的固定 200 家店铺、1000 条评论，挑战的是表达和任务分布，不冒充生产流量。
- `dev` 可用于调试 harness、prompt 和评分规则；`challenge` 只在代码冻结后运行。
- 文件在仓库中可见，所以它是“可复现的冻结 holdout”，不是保密 benchmark；这一限制在 manifest 中明示。
- 生成命令不调用模型，`formal_evaluation_executed=false`，因此生成文件不能被当成正式成绩。

## 正确使用协议

```bash
make generate-challenge-data

# 开发期间只跑 dev
go run ./cmd/eval-rag --test-set=rag-evals/challenge/retrieval.v3.json --split=dev
go run ./cmd/eval-agent --test-set=rag-evals/challenge/agent.v3.json --split=dev --force-route=agent_multistep

# 代码、prompt、grader 全部冻结后，才允许跑 challenge；结果应原样保留
go run ./cmd/eval-rag --test-set=rag-evals/challenge/retrieval.v3.json --split=challenge
go run ./cmd/eval-agent --test-set=rag-evals/challenge/agent.v3.json --split=challenge --force-route=agent_multistep
```

如果 challenge 失败，先记录失败类型和原始报告，再把经过人工确认的失败模式放进下一版 regression；随后换新种子、新表达重新生成 v4 challenge。不得修改 v3 题目或根据逐题结果反复调参后继续把它称为盲测。

现有 Agent runner 能隔离 case/trial 的会话和 profile，但尚不能在一个 case 内切换两个真实用户 ID；因此当前跨会话隔离用互斥的成对 `setup_profile` 检查，真正的跨用户污染测试仍需专用 runner。
