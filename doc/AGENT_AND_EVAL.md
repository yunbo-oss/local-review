# 推荐 Agent 与可复现评测

更新日期：2026-07-29。正式报告均来自真实 MySQL、Redis Stack、DeepSeek API 和 Go 运行时；仓库不包含 placeholder、fake 或 stub 正式结论。

## 1. 一键复现

前置条件：Docker Desktop、可用的 DeepSeek API key。密钥只从当前 shell 临时注入，不要写进 `.env`。

```bash
# 1) 真正的空环境
make docker-reset

# 2) 构建并启动 MySQL / Redis Stack / RocketMQ / Go，
#    自动执行迁移、200/1000 seed、Redis seed、200 条向量和 MQ topic 初始化
LLM_API_KEY='你的密钥' make docker-up

# 3) 固定数据 + 常规 API + 登录 + RAG/Agent SSE + 引用校验
LLM_API_KEY='你的密钥' make docker-verify

# 4) 正式 Retrieval、同任务 Hybrid RAG、Agent 评测
LLM_API_KEY='你的密钥' make docker-eval

# 5) 三轮记忆演示
LLM_API_KEY='你的密钥' make docker-demo
```

宿主机执行同一套评测：

```bash
make generate-eval-data
make test
make eval-rag-prod-baseline
make eval-hybrid-task
make eval-agent
```

## 2. 数据与隔离

- 固定生成种子：`20260729`。
- 基础 25 家店/45 条评论，加生成 175 家店/955 条评论，共 200 家店/1000 条评论。
- 5 个区域、8 个类别、6 个价格带、8 个语义主题。
- 显式覆盖近名难负样本、无结果、评价冲突和 2 条提示注入评论。
- Retrieval golden：68 题，dev 8 / test 60。
- Agent golden：28 场景，dev 6 / test 22。
- 8 个 critical Agent 场景各 3 trial；其余 test 场景各 1 trial，共 38 trial。
- 正式报告只读取 test split；dev 仅用于修复和定点回归。
- Agent trial 使用独立 session，并在每个 trial 前覆盖测试用户 profile，避免记忆污染。

可重复性清单见 `rag-evals/dataset_manifest.v2.json`；生成一致性用：

```bash
go run ./cmd/generate-eval-data --check
```

## 3. 正式实验条件

| 项目 | 条件 |
|---|---|
| Chat / filter 模型 | `deepseek-v4-flash` |
| Thinking | `disabled`，保证现有 OpenAI-compatible tool history 一致 |
| Embedding | `local-feature-hash-zh-v2`，384 维、确定性、L2 normalize |
| Hybrid | dense KNN + RediSearch text，RRF `k=60` |
| Candidate / Top K | 20 / 5 |
| Redis | `redis/redis-stack-server:7.4.0-v8` |
| MySQL | `mysql:8.0.43` |
| RocketMQ | `apache/rocketmq:5.3.2` |
| Agent 上限 | 3 steps / 5 tool calls / 8 attempts |
| Agent 数据哈希 | `sha256:4a0d11891dad4d4e808aa0cc3cd8383d4d416fa39f4ed7b6509ed801f97f6e25` |

Token 来自模型响应 usage，不是字符数估算。成本按 2026-07-29 DeepSeek V4 Flash cache-miss 上限价格估算：输入 `$0.14/M`、输出 `$0.28/M`，因此是保守上界。

## 4. 正式 Retrieval 结果

报告：`rag-evals/baseline/hybrid_prod_v2.json`。

| 指标 | test 60 |
|---|---:|
| HitRate@5 | 100.00% |
| Recall@5 | 81.63% |
| Precision@5 | 83.21% |
| MRR | 1.0000 |
| NDCG@5 | 1.0000 |
| Filter field accuracy | 100.00% |
| Filter compliance@5 | 100.00% |
| No-result accuracy | 100.00% |
| Task success | 100.00% |
| P50 / P95 | 857 ms / 1176 ms |
| Infra error rate | 0.00% |

Recall 没有通过缩小 relevant set 做高：硬过滤题把所有满足条件的基础店和生成店都放进 relevant set，因此当相关店超过 5 家时，Recall@5 的理论上限本来就小于 100%。

## 5. Agent 指标口径

- `outcome_rate`：任务结果是否命中正例、避开显式禁止项、满足 hard filter/profile/无结果要求。
- `groundedness_rate`：成功店铺回答必须有 `[shop:id]`，且引用必须属于本轮 evidence ledger；语义偏好还必须能在检索评价摘要或实际读取评价中找到证据。
- `trajectory_pass_rate`：步骤、工具数和重复调用均在上限内。
- `composite_pass_rate`：同一 trial 的 outcome、groundedness、trajectory 全部通过。
- `tag_outcome_rates` 与 `tag_composite_pass_rates` 分开报告，不能混称。
- `all_trials_pass_rate`：仅统计 trial 数不少于 3 的关键场景，要求该场景所有 trial 全过。旧名 `pass_at_k` 已删除，因为它不是“至少一次成功”的 pass@k。
- infra error 单列，不进入质量分母。

## 6. Agent vs Hybrid RAG：同任务对照

报告：

- `rag-evals/baseline/hybrid_task_v2.json`
- `rag-evals/baseline/agent_prod_v2.json`

双方严格使用相同的 22 个 test 场景、相同的 38 个 trial 和相同数据哈希；没有再用 Agent outcome 减 Retrieval HitRate。

| 指标 | Agent | Hybrid RAG | 差异 |
|---|---:|---:|---:|
| Task success | 100.00% | 81.58% | +18.42 pp |
| Composite pass | 100.00% | 81.58% | +18.42 pp |
| 成功回答 groundedness | 100.00% | 100.00% | 0 pp |
| 全部 trial 通过率 | 100.00% | 75.00% | +25.00 pp |
| P50 latency | 7612 ms | 3064 ms | +4548 ms |
| P95 latency | 12430 ms | 7080 ms | +5350 ms |
| 平均模型调用 | 4.11 | 2.21 | +1.89 |
| 平均工具调用 | 2.74 | 0 | +2.74 |
| 平均 Token | 5098.89 | 1074.03 | +4024.87 |
| 38 trial 成本上界 | `$0.029826` | `$0.006653` | `$0.023173` |
| Infra error rate | 0.00% | 0.00% | 0 pp |

结论：在包含记忆、纠正、近名、评价证据和提示注入的同任务集上，Agent 明显提高成功率和多 trial 稳定性；代价是约 2.5 倍 P50 延迟、更多模型/工具调用和约 4.75 倍 Token。这个对照支持“需要多步证据和记忆时用 Agent，单轮清晰问题用 Hybrid RAG”的路由策略，而不是声称 Agent 无条件优于 RAG。

## 7. Harness 的正确性保护

- 工具预算耗尽时不再返回空答案，而是补齐 tool response 后用已有证据有界收尾。
- 用户明说的区域/类别/预算由确定性 guard 保留；模型不能把“商务宴请”擅自硬过滤为“美食”。
- 语义适配必须来自不可信但可追溯的评价证据，店名和常识不能证明“安静/无障碍”等事实。
- 常见 Markdown `shop:` 链接规范化成 `[shop:id]`；缺引用或混入未知店铺时只允许一次无工具、白名单限定的修订，随后重新走完整 verifier。
- 语义任务先在硬过滤后的 20 个候选中按同一评价证据规则稳定重排，再裁剪为模型 Top-K；店铺对比任务保持原排序。
- 多店价格校验使用引用店价格集合，不再误杀不同价格的合法比较。
- 提示注入评论始终标为 untrusted data，不能改变 system policy。

完整问题和修复证据见 `doc/EVAL_PRACTICE_LOG.md`。

## 8. 三轮记忆 Demo

`make docker-demo` 自动完成：

1. 写入“海淀、预算 80”偏好；
2. 同 session 按长期偏好推荐适合学生的店并检查 `[shop:id]`；
3. 清空预算，切换到丰台区家庭聚餐，并再次检查引用。

脚本遇到任一 SSE error、缺 message/done、推荐缺引用、预算未清空或丰台区未写入都会非零退出。脱敏结果写入 `rag-evals/reports/memory_demo_latest.json`。

## 9. 仍需诚实说明的限制

- 数据是固定种子的合成评测集，不代表线上自然流量；100% 是该封闭 test set 上的结果，不应外推。
- 本地 feature-hash embedding 追求可复现和零 embedding 成本，不等价于通用语义向量模型。
- Hybrid baseline 没有可写长期记忆和只读工具，这正是比较的系统差异；它不是对所有 RAG 实现的结论。
- Agent P95 约 12.4 秒，不适合所有交互；生产需要路由、缓存和更短上下文。
- 语义证据 gate 使用有限中文概念词表；开放域同义表达仍需要更强的判别器或人工标注集。
- RocketMQ Broker 在本机 Docker Desktop 的内存占用较高。
