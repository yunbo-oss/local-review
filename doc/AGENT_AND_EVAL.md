# 推荐 Agent 与可复现评测

更新日期：2026-08-09。以下正式结果来自真实 MySQL、Redis Stack、Go 服务和 DeepSeek API；报告包含冻结 commit、数据哈希、模型、温度、延迟、Token 和逐 trial 明细，不使用 placeholder、fake 或 stub 数据。

## 1. 一键复现

前置条件：Docker Desktop、可用的 DeepSeek API key。密钥只注入当前 shell，不写入 `.env` 或仓库。

```bash
make docker-reset
LLM_API_KEY='你的密钥' make docker-up
LLM_API_KEY='你的密钥' make docker-verify
LLM_API_KEY='你的密钥' make docker-eval
LLM_API_KEY='你的密钥' make docker-demo
```

冻结代码和数据后，v3 challenge 只运行一次：

```bash
LLM_API_KEY='你的密钥' make docker-challenge
```

空卷验收应看到：MySQL 200 家店/1000 条评价、RediSearch `idx:shop:vector` 的 `num_docs=200`，App/MySQL/Redis/RocketMQ healthy，迁移与所有初始化任务 exit 0。seed 可重复运行，计数不变。

## 2. Agent 设计

统一入口先做确定性路由：明确单轮请求走 Hybrid RAG；需要记忆、纠正、比较、详情或评价核验时走 Agent；信息不足时先澄清。正式 Agent 对照使用 `force_route=agent_multistep`，避免把 Router 的误差混入 Agent 能力。

```text
请求
  -> Router / force route
  -> 加载结构化 profile + 本轮显式纠正
  -> 有界 tool loop
       search_shops -> get_shop / list_shop_blogs
       最多 3 steps、5 次成功工具调用、8 次尝试、每轮最多 3 个工具
  -> EvidenceLedger 引用与事实校验
  -> 必要时一次无工具白名单修订
  -> SSE answer + route/steps/tool_calls/tokens/trace_id
```

关键控制：

- profile 是结构化字段，不把整段历史直接塞进 prompt；用户本轮显式约束优先于长期偏好。
- 评价和检索文本均标为 untrusted data，提示注入文本不能改变系统策略。
- 只有本轮工具观察到的店可引用；成功推荐必须使用 `[shop:id]`。
- verifier 校验引用合法性、声明覆盖、价格、评分、地址和营业时间；语义适配必须有评价证据。
- 达到预算时用已有证据有界收尾，不继续循环；总运行 45 秒、单工具 10 秒超时。
- 博客发布和点赞会触发店铺向量刷新；MQ 失败时仍可能短暂陈旧，当前没有 transactional outbox。

## 3. 数据、split 与防泄漏

- 固定 seed `20260729`：25 家基础店 + 175 家生成店，45 + 955 条评价，共 200/1000。
- 955 条生成评价含 635 种不同正文、350 条语义评价，覆盖 5 区域、8 类别、6 价格带、8 语义主题。
- 覆盖近名难负样本、无结果、评价冲突、错别字、否定/纠正、长上下文、提示注入和伪造工具文本。
- v2 regression：Retrieval 8 dev / 60 test；Agent 6 dev / 22 test，8 个 critical 各 3 trial，共 38 test trials。
- v3 frozen challenge：Retrieval 24 dev / 120 challenge；Agent 8 dev / 28 challenge，10 个 critical 各 3 trial，共 48 challenge trials。
- 合计 50 个正式 Agent test/challenge 场景、86 次真实 trial；若包含 dev，共 64 个场景。不能写成 55 个。
- dev 可用于开发；test/challenge 只在冻结后运行。challenge 在仓库中可见，因此是“可复现 holdout”，不是秘密线上 benchmark。

生成与校验：

```bash
go run ./cmd/generate-eval-data --check
go run ./cmd/generate-challenge-data --check
```

## 4. 实验条件

| 项目 | 条件 |
|---|---|
| 冻结 commit | `6819a5c5ff6e5cc3f6a6ae0d2f92cc748d1061e4` |
| Chat / filter | `deepseek-v4-flash`，temperature `0.1`，thinking disabled |
| Embedding | `local-feature-hash-zh-v2`，384 维、确定性、L2 normalize |
| Hybrid | RediSearch text + dense KNN，RRF `k=60`，candidate 20 / top 5 |
| Agent 上限 | 3 steps / 5 tool calls / 8 attempts / 45 秒 |
| 成本假设 | 输入 `$0.14/M`、输出 `$0.28/M`；只作为显式费率假设下的上界 |

Token 来自模型 usage。正式报告记录 git clean、Go 版本和数据哈希；infra error 单列，不用质量失败掩盖服务故障。

## 5. Retrieval 正式结果

| 指标 | v2 test 60 | v3 challenge 120 |
|---|---:|---:|
| HitRate@5 | 100.00% | 70.54% |
| Recall@5 | 81.63% | 57.98% |
| Precision@5 | 83.21% | 47.14% |
| MRR | 0.9732 | 0.6143 |
| NDCG@5 | 0.9802 | 0.5948 |
| Filter field accuracy | 100.00% | 92.88% |
| Filter compliance@5 | 100.00% | 78.45% |
| No-result accuracy | 100.00% | 12.50% |
| Task success | 100.00% | 64.17% |
| P50 / P95 | 759 / 1083 ms | 785 / 1090 ms |
| Infra error | 0.00% | 0.00% |

v2 说明回归集已被工程化解决；v3 才揭示开放表达弱点：错别字、否定纠正、OOD 同义表达和无结果判断。Recall@5 的 relevant set 包含所有满足硬条件的店，相关店多于 5 家时理论上限本就低于 100%。

报告：`rag-evals/baseline/hybrid_prod_v2.json`、`rag-evals/reports/retrieval_challenge_v3.json`。

## 6. Agent 指标口径

- `outcome`：任务结果是否命中允许项、避开禁止项、满足硬过滤/profile/无结果及指定声明。
- `groundedness`：引用合法性 + 声明覆盖 + 结构化事实一致性；“成功回答 groundedness”只在 outcome 成功的回答上统计。
- `trajectory`：步骤/调用/重复调用在上限内，并按场景要求使用必要工具。
- `composite pass`：同一 trial 的 outcome、groundedness、trajectory 全部通过。
- `all_trials_pass_rate`：仅在至少 3 trial 的 critical 场景上统计，要求一个场景的所有 trial 全过；不是 pass@k。
- trial-micro 每次运行同权；scenario-macro 每个场景同权。简历优先用 scenario-macro，避免 3-trial 场景被额外加权。

## 7. 同任务 Agent vs Hybrid RAG

### v2 regression

| 指标 | Agent | Hybrid | 差异 |
|---|---:|---:|---:|
| Trial-micro task success | 84.21% | 63.16% | +21.05 pp |
| Scenario-macro task success | 87.88% | 75.76% | +12.12 pp |
| Composite pass | 84.21% | 63.16% | +21.05 pp |
| Critical 全 trial 通过率 | 62.50% | 37.50% | +25.00 pp |
| 成功回答 groundedness | 100.00% | 100.00% | 0 pp |
| P50 / P95 | 6852 / 9352 ms | 2903 / 6288 ms | 更慢 |
| 平均模型 / 工具调用 | 4.45 / 2.45 | 2.21 / 0 | 更多 |
| 平均 Token | 5361 | 1154 | 4.65 倍 |
| 38 trials 成本上界 | `$0.031633` | `$0.007363` | +`$0.024270` |
| Infra error | 0% | 0% | 0 pp |

### v3 frozen challenge

| 指标 | Agent | Hybrid | 差异 |
|---|---:|---:|---:|
| Trial-micro task success | 52.08% | 12.50% | +39.58 pp |
| Scenario-macro task success | 53.57% | 11.90% | **+41.67 pp** |
| Trial-micro composite | 45.83% | 2.08% | +43.75 pp |
| Critical 全 trial 通过率 | 30.00% | 0.00% | +30.00 pp |
| 成功回答 groundedness | 100.00% | 100.00% | 0 pp |
| Agent overall groundedness | 93.75% | 91.67% | 仅作诊断 |
| P50 / P95 | 7505 / 12559 ms | 2773 / 10953 ms | 更慢 |
| 平均模型 / 工具调用 | 5.35 / 2.06 | 3.52 / 0 | 更多 |
| 平均 Token | 6131 | 1939 | 3.16 倍 |
| 48 trials 成本上界 | `$0.044886` | `$0.015092` | +`$0.029794` |
| Infra error | 0% | 0% | 0 pp |

结论不是“Agent 全面优于 RAG”，而是：在需要记忆、纠正、评价核验和提示注入防护的同任务上，Agent 用更多延迟、Token 和调用换来更高成功率；明确单轮问题仍应走 Hybrid。v3 的 95% Wilson 区间较宽（Agent outcome 38.33%–65.53%），不能把 +41.67 pp 外推成线上精确增益。

## 8. Router 与三轮记忆 Demo

Router test 48 题准确率 79.17%，infra error 0。`rag_oneshot` recall 92.31%，但同义改写的多步/记忆请求召回不足，说明当前规则策略偏保守；正式 Agent 对照强制路由，因此没有把 Router 分数混入 Agent 分数。

三轮 Demo 已验证：3/3 SSE 成功；第 2/3 轮有 grounded citation；预算被清空；区域从海淀纠正为丰台；最终 profile version 3。报告为 `rag-evals/reports/memory_demo_latest.json`。

## 9. 仍未解决的限制

- 数据是固定种子的封闭合成集，没有真实流量频率、人工双标或跨城市长尾。
- 本地 feature-hash embedding 便于复现，但错别字和开放域同义表达能力有限。
- v3 no-result accuracy 只有 12.5%，应优先增加检索置信度校准、规则化未知区域/类别处理和拒答测试。
- 多店回答的结构化事实校验基于引用证据集合，尚未把每个自然语言 claim 精确绑定到对应店。
- 博客/点赞通过 MQ 刷新向量，但 MQ 发送失败缺少 outbox/reconciliation，可能短暂陈旧。
- Router test 79.17%；生产应增加可观测路由、灰度和回退，不应假设规则完全覆盖同义表达。
- challenge 在仓库中可见，不能冒充秘密盲测；下一版应从真实脱敏 query 建立人工标注集。

完整实践故障见 `EVAL_PRACTICE_LOG.md`，面试问答见 `AGENT_INTERVIEW_GUIDE.md`。
