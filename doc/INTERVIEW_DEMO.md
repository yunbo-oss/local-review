# 秋招面试演示说明

## 30 秒项目定位

这是一个 Go 点评系统。我把原本“代码写了但没跑过”的 RAG/Agent 原型补成了可从空 Docker volume 复现的评测闭环：固定生成 200 家店和 1000 条评论，统一评测 Retrieval、Hybrid RAG 与有界工具 Agent，并把任务成功、groundedness、轨迹、稳定性、延迟、Token 和成本拆开报告。

## 3 分钟演示顺序

1. 展示 `docker compose ps -a`：MySQL、Redis Stack、RocketMQ、Go app healthy；迁移、seed、向量和 topic 初始化任务 exit 0。
2. 展示 `rag-evals/dataset_manifest.v2.json`：固定 seed、200/1000、golden 数量和覆盖标签。
3. 执行 `LLM_API_KEY=... make docker-verify`：数据生成一致、登录/API/RAG/Agent SSE 和 `[shop:id]` 引用通过。
4. 打开三个正式报告：
   - `rag-evals/baseline/hybrid_prod_v2.json`
   - `rag-evals/baseline/hybrid_task_v2.json`
   - `rag-evals/baseline/agent_prod_v2.json`
5. 执行 `LLM_API_KEY=... make docker-demo`：写偏好、同 session 使用偏好、清空预算并切到丰台区。

## 面试时怎么讲指标

Retrieval 的 test 60 题上，HitRate@5 100%，Recall@5 81.63%，Precision@5 83.21%，MRR 和 NDCG@5 都是 1.0，过滤字段准确率和 Top-5 过滤合规都是 100%，基础设施错误率 0%。

Agent 与 Hybrid RAG 使用完全相同的 22 个场景和 38 个 trial。Agent task success 100%，Hybrid RAG 81.58%，提升 18.42 个百分点；成功回答 groundedness 都是 100%。Agent 的代价是 P50 从 3.06 秒增到 7.61 秒，平均 Token 从 1074 增到 5099，所以线上策略不是“所有请求都走 Agent”，而是单轮清晰请求走 Hybrid RAG，记忆、比较、评价核验和风险场景走 Agent。

## 可诚实写进简历的表述

> 为 Go 点评系统构建可复现 Hybrid RAG/Tool Agent 评测闭环：固定种子生成 200 家店、1000 条评论和 96 个 dev/test 用例；在相同 22 个任务、38 次真实 DeepSeek trial 上，将 Agent task success 从 Hybrid RAG 的 81.58% 提升至 100%，成功回答 groundedness 100%、infra error 0%，并量化 P50/P95、工具调用、Token 与成本开销。

不要写：

- “线上准确率 100%”——这是封闭合成 test set。
- “Agent 比 RAG 快”——实际 P50/P95 更慢。
- “pass@k 100%”——这里的指标是关键场景“全部 trial 通过率”，不是至少一次成功概率。
- “使用 DeepSeek embedding”——DeepSeek 本轮只用于 chat/filter；embedding 是仓库内确定性 feature-hash。

## 最值得追问的三个工程点

### 1. 为什么不是只看 HitRate？

Retrieval 的 HitRate 不能代表 Agent 是否完成记忆、纠正、评价冲突或提示注入任务。所以另外在同一 Agent 场景集上跑 Hybrid RAG，并用同一个 outcome grader 比 task success。

### 2. 如何保证 grounded？

每次 Agent 运行维护 EvidenceLedger。只有 search 发现的店可引用；详情和评价只能补证据，不能“洗白”未知 ID。成功店铺回答必须含 `[shop:id]`；价格必须属于引用证据集合；安静、无障碍等语义偏好必须能在不可信评价摘要或实际读取评价中找到对应证据。

### 3. 为什么 Agent 能提高成功率？

主要来自服务端记忆合并、分步工具核验、硬约束 guard 和语义 evidence gate，而不是单纯增加 prompt。Hybrid RAG 在无记忆多轮、纠正和部分近名/证据场景失败；Agent 用更高延迟和 Token 换取确定性控制。

## 失败案例怎么讲

不要隐藏首轮只有 26.3% 的中间结果。它帮助定位了 harness 问题：工具预算耗尽直接空答、filter capture 串场、引用 grader 过松、多店价格误杀、全文查询语法错误和 golden 自相矛盾。逐项修复和回归记录在 `doc/EVAL_PRACTICE_LOG.md`，最终正式报告才进入结论。

## 仍未解决

- 合成数据无法代表真实用户分布。
- 本地 feature-hash embedding 的开放域泛化有限。
- 语义 evidence gate 是有限中文概念词表。
- Agent P95 约 12.4 秒，生产仍需路由、缓存、上下文压缩和更多真实标注回归集。
