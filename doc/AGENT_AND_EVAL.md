# 推荐 Agent 与可复现评测

更新日期：2026-08-19。v4 正式报告来自真实 MySQL、Redis Stack、Go 服务和 DeepSeek API。v6 的 Parallel ReAct 架构已实现并完成本地回归与评测链路冒烟；只有 `mode=inprocess` 的报告可作为模型质量结果，fake report 仅验证 harness/grader 形状。

## 1. 从空环境复现

前置条件：Docker Desktop 和可用的 DeepSeek API key。密钥只注入当前 shell，不写入 `.env` 或仓库。

```bash
make docker-reset
LLM_API_KEY='你的密钥' make docker-up
LLM_API_KEY='你的密钥' make docker-verify
LLM_API_KEY='你的密钥' make docker-eval
LLM_API_KEY='你的密钥' make docker-demo

# 冻结 v4 的同任务 Agent / Hybrid 对照
LLM_API_KEY='你的密钥' make docker-challenge-v4

# v5 目标架构的扩展挑战集
LLM_API_KEY='你的密钥' make docker-challenge-v5

# v6 Parallel ReAct 冻结挑战集（代码/prompt/grader 冻结后运行）
LLM_API_KEY='你的密钥' make docker-challenge-v6

# v6.1 生产 Router 到终答的回归评测；不运行 Hybrid 对照组
LLM_API_KEY='你的密钥' make docker-agent-e2e-v61
```

空卷验收应得到：MySQL 200 家店/1000 条评价，RediSearch `idx:shop:vector` 的 `num_docs=200`，App/MySQL/Redis/RocketMQ healthy，迁移与初始化任务 exit 0。seed 可重复运行，计数保持不变。

## 2. 系统设计

统一入口先做一次结构化 Query Understanding：同时输出意图、路由、硬过滤、软偏好、实体、证据需求、澄清信号和 1–3 条检索改写。明确单轮查询走 Hybrid RAG；需要记忆、比较、详情或评价核验时走 Agent；信息不足时澄清。规则 Router 仅作为模型失败或低配部署的回退。

```text
请求
  -> 安全门禁 + LLM Query Understanding / Query Rewrite / 置信度澄清
  -> 选择性注入 profile、episodic summary 与 query-relevant turns
  -> V2 Controller 输出结构化 act / finish / clarify（不持久化思维链）
  -> search_shops 建立候选注册表
  -> Executor 按 depends_on 执行 DAG；详情/评价在候选已知后并行
  -> Evidence Gap -> 评论 cursor 增量续查 / 改写后重搜 -> 重新决策
  -> 检索相关性重排 + 证据覆盖率二次重排
  -> claim JSON -> 引用/字段校验 -> LLM entailment 阈值门禁
  -> 回答校验成功后才允许长期记忆写入
  -> Redis checkpoint + MySQL run/tool audit + OTel distributed trace
```

主要约束：

- Memory 不是独立 Agent。`agent_memory` 只保留为旧 Router/评测兼容标签；执行时由同一个 AgentState 使用 `none / read_only / write_after_success` 策略直接、有限地注入记忆。
- 统一 Router 入口负责把 RAG、澄清和 Agent 的对话轮次写入同一份有界 session；RAG 后续追问因此能读取上一轮候选。长期 profile 仍只接受显式偏好变更或通过最终 verifier 的成功 Agent 结果。
- profile 是结构化字段，本轮显式约束优先于长期偏好；被本轮区域、类别或预算覆盖的旧字段不会再进入 prompt。
- 评价和检索文本标记为 untrusted data，文本中的指令不能改变系统策略。
- 只有本轮工具观察到的店铺可以引用；成功推荐必须使用 `[shop:id]`。
- verifier 先校验每个 claim 的同店引用和结构化字段值，再对体验类 claim 做 evidence entailment；低于默认 `0.75`、矛盾或未知都失败关闭。
- 重排置信度与评价证据覆盖度形成 `accept / verify / abstain`，近零相关候选不会被送去硬凑答案。
- V2 默认最多 4 个 controller turns、10 次成功工具调用、12 次尝试、2 次搜索、每店 2 页评论、3 个并行工具；达到预算后只允许用已有证据收尾。
- `agent.run -> agent.controller / agent.action -> tool.execute` 共享 W3C trace；SSE `run_started` 和 `done` 暴露同一 trace id。Redis checkpoint 使用 revision CAS 和 30 分钟 TTL，跨实例恢复；MySQL 只落有界工具审计摘要，不落原始评价。
- 博客发布和点赞会触发向量刷新；MQ 发送失败时仍可能短暂陈旧，当前没有 transactional outbox。

## 3. 数据、版本与隔离

- 固定 seed `20260729`：25 家基础店 + 175 家生成店，45 + 955 条评价，共 200 家店、1000 条评价。
- 955 条生成评价含 635 种不同正文、350 条语义评价，覆盖 5 个区域、8 个类别、6 个价格带、8 个语义主题。
- 数据覆盖近名难负样本、无结果、评价冲突、否定/纠正、长上下文、提示注入和伪造工具文本。
- Retrieval v2 regression：8 dev / 60 test；冻结 v3 challenge：24 dev / 120 challenge。
- Agent v2 regression：6 dev / 22 test、38 test trials；v3 保持冻结，不再修改。
- Agent v3.1 吸收已确认的系统与评分器修复，只作为回归集。
- Agent v4：8 dev / 28 challenge；10 个 critical challenge 场景各运行 3 次，共 48 trials。按当前范围不包含错别字题，重点测试语义偏好、记忆纠正、冲突、无结果、证据和注入防护。
- Agent v5：8 dev / 40 challenge；8 个 critical challenge 场景各运行 3 次，共 56 trials。覆盖统一理解/改写、错别字、规划与重规划、claim 证据、分层记忆、无结果、注入和会话隔离。
- Agent v6：8 dev / 40 challenge、56 trials；在 v5 能力族上新增 V2 runtime、搜索轮数、评论页数和最终 evidence verification 的硬断言。
- Agent v6.1：与已执行 v6 使用同一任务族的可检查 regression；去掉 v2-only runtime 断言，用于 Router 到终答的任务正确率回归。分页、重试、超时、checkpoint 与并行上限由确定性 runtime tests 单独验证。
- dev 可用于开发；challenge 只在代码、prompt、grader 和数据冻结后运行。文件在仓库中可见，因此是可复现 holdout，不是保密 benchmark。

生成器支持字节级检查：

```bash
go run ./cmd/generate-eval-data --check
go run ./cmd/generate-challenge-data --check
go run ./cmd/generate-challenge-data --suite=v31 --check
go run ./cmd/generate-challenge-data --suite=v4 --check
go run ./cmd/generate-challenge-data --suite=v5 --check
go run ./cmd/generate-challenge-data --suite=v6 --check
go run ./cmd/generate-challenge-data --suite=v61 --check
```

## 4. 冻结 v4 实验条件

| 项目 | 条件 |
|---|---|
| 代码与数据 commit | `fb85084e6460f8f26cd6f3c4ee890e37b2ec36c7` |
| 数据哈希 | `sha256:b91ef3ce136de971ae6bfa7b3bcfbb90c0945566b7678a56ca5562d41d8d1382` |
| Chat / filter | `deepseek-v4-flash`，temperature `0.1`，thinking disabled |
| Embedding | `local-feature-hash-zh-v2`，384 维、确定性、L2 normalize |
| Hybrid | RediSearch TEXT + dense KNN，RRF `k=60`，candidate 20 / top 5 |
| Agent 上限 | 3 steps / 5 successful tool calls / 8 attempts / 3 tools per turn / 45 秒 |
| 运行规模 | 28 challenge 场景、48 trials；Agent 与 Hybrid 的 case/trial 完全对齐 |
| 运行模式 | `inprocess`，真实模型与基础设施，infra error 均为 0% |
| 成本假设 | 输入 `$0.14/M`、输出 `$0.28/M`，按报告 usage 估算上界 |

Token 取自模型 usage。正式报告在 Git clean 状态运行并记录运行时版本；基础设施错误单列，避免用服务故障污染质量指标。

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

v2 是已被工程化解决的回归集；v3 揭示 OOD 同义表达、错别字、否定纠正和无结果判断的弱点。Recall@5 的 relevant set 包含所有满足硬条件的店，相关店多于 5 家时理论上限本就低于 100%。

报告：`rag-evals/baseline/hybrid_prod_v2.json`、`rag-evals/reports/retrieval_challenge_v3.json`。

## 6. Agent 指标定义

- `outcome`：任务是否命中允许项、避开禁止项，并满足硬过滤、profile、无结果和指定声明。
- `groundedness`：引用合法性、声明覆盖和结构化事实一致性；“成功回答 groundedness”只统计 outcome 成功的回答。
- `trajectory`：步骤、调用和重复调用是否在上限内，并按场景要求使用必要工具。
- `composite pass`：同一 trial 的 outcome、groundedness、trajectory 全部通过。
- `all_trials_pass_rate`：只统计至少运行 3 次的 critical 场景，要求该场景所有 trial 全部通过；它不是 pass@k。
- `trial-micro`：每次运行同权；`scenario-macro`：先聚合同一场景的 trials，再让每个场景同权。

## 7. v4 同任务 Agent vs Hybrid RAG

| 指标 | Agent | one-shot Hybrid | 差异 |
|---|---:|---:|---:|
| Trial-micro task success | 97.92% | 52.08% | +45.83 pp |
| Scenario-macro task success | 96.43% | 46.43% | **+50.00 pp** |
| Trial-micro composite pass | 97.92% | 14.58% | +83.34 pp |
| Critical 全 trial 通过率 | 100.00% | 20.00% | +80.00 pp |
| 成功回答 groundedness | 100.00% | 100.00% | 0 pp |
| Overall groundedness | 97.92% | 85.42% | 仅作诊断 |
| Trajectory pass | 100.00% | 16.67% | 不用于 task-success 对比 |
| P50 / P95 | 6043 / 10049 ms | 2576 / 10861 ms | Agent P50 更慢 |
| 平均模型调用 | 3.42 | 3.52 | -0.10 |
| 平均工具调用 | 2.56 | 0 | +2.56 |
| 平均 Token | 5292 | 1924 | 2.75 倍 |
| 48 trials 成本上界 | `$0.038347` | `$0.014515` | +`$0.023832` |
| Infra error | 0.00% | 0.00% | 0 pp |

task success 只比较 outcome，不因 Hybrid 没有工具而扣 trajectory 分。两边使用同一批 28 个场景、相同 48 个 trial、相同 profile 输入、数据哈希和 grader。v4 强制走 Agent 路由，所以 **+50.00 pp 是固定路由 Agent 相对 one-shot Hybrid 在这组复杂任务上的 scenario-macro 增益**，不是 Router + Agent 联合系统分数，也不能表述为线上整体“提升 50%”。

这组场景刻意包含记忆、纠正、评价核验、冲突、拒答和注入防护，适合衡量“何时值得多步调用”，不代表普通单轮请求的自然流量占比。Agent 用更高 P50、2.75 倍 Token 和工具调用换取复杂任务成功率；简单请求仍应由 Router 分流到 Hybrid。

正式报告：`rag-evals/reports/agent_challenge_v4.json`、`rag-evals/baseline/hybrid_task_challenge_v4.json`。

## 8. Router、记忆 Demo 与失败案例

Router 是不调用模型的四路确定性分类器：合法 force override → 注入/引用文本隔离 → 偏好变更 → 比较/详情/证据型多步意图 → 历史指代 → 澄清或 one-shot RAG。Handler 先用 `NeedsSessionHistory` 判断是否需要读取 session，避免普通请求增加一次记忆存储访问。

| 数据集 | 策略 | Accuracy | Macro-F1 | 说明 |
|---|---|---:|---:|---|
| v1 test 48 | rules v1 | 79.17% | 80.50% | 首次冻结结果 |
| v1 test 48 | rules v2 | 100.00% | 100.00% | 已查看旧失败后的 regression |
| v2 challenge 52 | rules v1 | 55.77% | 57.88% | 数据先冻结，旧 commit 隔离复跑 |
| v2 challenge 52 | rules v2 | 100.00% | 100.00% | 首次正式运行，+44.23 pp |

v2 challenge 四类各 13 题，覆盖自包含请求、多步工具需求、偏好/历史记忆、缺失上下文、force override 和引用注入。数据 commit `a55f026` 早于策略 commit `82cfe52`；两份报告数据哈希相同。由于数据是同一项目内人工设计且规则标签边界明确，100% 只能证明当前规则覆盖这些意图族，不是线上准确率。Router 分数也没有混入冻结 Agent v4 的固定路由分数。

报告：`rag-evals/reports/router_challenge_v2_policy_v1.json`、`rag-evals/reports/router_challenge_v2.json`。

三轮记忆 Demo 验证：3/3 SSE 成功；推荐轮存在 grounded citation；预算被清空；区域从海淀纠正为丰台；最终 profile version 3。报告为 `rag-evals/reports/memory_demo_latest.json`。

v4 唯一失败是 `a4-20` 的一个非 critical trial：长多轮指代下，profile 和工具调用均正确，但最终回答沿用了前文的“暂不推荐”，没有输出推荐。该失败原样保留，没有继续针对 challenge 调参。47/48 的 Wilson 95% 区间约为 89.10%–99.63%，样本仍不足以证明线上接近 98%。

## 9. v6 运行时与可观测指标

v6 report 在原有 outcome / groundedness / trajectory / latency / token 之外，记录 `runtime_version`、`runtime_status`、`search_rounds`、`max_review_pages`、`evidence_gap_count` 和 `answer_verified`，并继续保留 Query Understanding、rewrite、retrieval 与 claim coverage 指标。正式数字必须在配置 API key、冻结代码/prompt/grader 后由 `make docker-challenge-v6` 生成；不能把 fake runner 的结果写进简历。

另外，`TestAgentArchitectureEndToEndCommitsOnlyVerifiedResult` 使用确定性边界替身穿过真实编排链路，验证 multi-query、硬过滤、Planner、评价工具、Reranker、claim verifier、完整 usage 计账和成功后 episodic compaction；配对失败用例注入不存在的 `blog:999`，验证一次修复后失败关闭且 session/profile/summary 均不写入。它是架构集成测试，不替代真实模型质量评测。

### 9.1 Router E2E v6.1 首轮诊断（历史修复前）

2026-08-19 使用真实 DeepSeek、MySQL 和 Redis 运行 40 个 challenge 场景、56 trials，入口为 `router_e2e`，不强制路由、不运行对照组。首轮历史诊断结果为：

- task outcome 34/56（60.71%），scenario-macro 56.67%；成功任务 groundedness 33/34（97.06%）；infra error 0。
- 路由分布：`rag_oneshot` 19、`agent_multistep` 14、`agent_memory` 18、`clarify` 5。RAG 与 multistep 的 outcome 分别为 15/19 和 13/14；主要短板集中在 memory follow-up。
- P50/P95 为 3.741s/13.849s；257 次模型调用、80 次工具调用、430855 Token，按报告价格假设估算上界 `$0.064701`。
- 该轮确认并修复了三类共同机制：偏好更新误入无状态 RAG、会话恢复语句误标成 `preference_update`、RAG 自由文本校验失败后过度降级为无结果；Controller 的非法结构化动作增加一次有界 validation-feedback 修复。

这是开发回归诊断，不是当前效果；下节数值来自修复后的独立完整重跑，不是对首轮失败数做算术外推。

### 9.2 Router E2E v6.1 修复后完整回归

同日使用最终代码和相同 40 个 challenge 场景重新运行 56 trials，canonical 报告为 `rag-evals/reports/agent_e2e_v61.json`：

- task outcome 与 composite 均为 52/56（92.86%），scenario-macro 均为 90.00%；成功任务 groundedness 52/52（100%）；infra error 0。
- 路由分布为 `rag_oneshot` 19、`agent_multistep` 14、`agent_memory` 18、`clarify` 5；前三类 outcome 分别为 19/19、14/14、14/18，clarify 为 5/5。
- P50/P95 为 8.455s/23.141s；356 次模型调用、209 次工具调用、758480 Token，按报告价格假设估算上界 `$0.113227`。
- prompt injection、raw-review tool gate、claim-level grounding、no-result、错别字、session isolation 均为 100%；至少运行 3 次的 critical 场景全部 trial 通过。
- 剩余 4 个失败全部位于 `agent_memory` 的开放式语义候选选择：2 个安全退化为 no-result，2 个回答有合法证据但额外推荐了黄金集合外候选。它们应进入下一版真实流量标注/私有 holdout，而不是继续针对 v6.1 单题调参。

## 10. 已知限制

- 店铺、评价和 query 主要是固定种子合成数据，没有真实流量频率、人工双标或跨城市长尾。
- 本地 feature-hash embedding 便于复现，但开放域同义表达和错别字能力有限；错别字不在 Agent v4 的测量范围内。
- Retrieval 置信度门使用可解释阈值但尚未在真实人工标注流量上做概率校准；v3 的历史 no-result 结果不能代表新门控效果。
- MQ 刷新向量缺少 outbox/reconciliation，数据库提交后可能短暂陈旧。
- LLM Query Understanding、Controller 和 entailment judge 仍缺真实 query 分布的人工双标、阈值校准、灰度和线上混淆矩阵；`0.75` 目前是可解释工程阈值，不是概率校准结论。
- 1000 条基础评价保持冻结且生成店默认每店 5 条，不能真实触发第二页评论；分页、重试、超时、取消和 checkpoint-resume 当前由确定性 runtime tests 覆盖。若要做真实分页演示，应新增独立、版本化的 runtime seed overlay，不能直接改写这套任务基准。
- v4 在仓库中可见且样本较小；后续应使用真实脱敏 query、独立标注和私有 holdout 验证泛化。

完整工程问题、修复和验证过程见 `EVAL_PRACTICE_LOG.md`。
