# 秋招面试演示说明

## 30 秒项目定位

这是一个 Go 点评系统。我把 RAG/Agent 原型补成了可从空 Docker volume 复现的闭环：固定生成 200 家店、1000 条评价；用同一批任务比较 Hybrid RAG 和有界工具 Agent；把 outcome、groundedness、trajectory、稳定性、延迟、Token、成本和基础设施错误分开报告。

## 3 分钟演示

1. `docker compose ps -a`：App、MySQL、Redis Stack、RocketMQ healthy；迁移、seed、向量和 topic 初始化 exit 0。
2. 打开 `rag-evals/dataset_manifest.v2.json` 和 `rag-evals/challenge/manifest.v3.json`：展示固定 seed、200/1000、split、覆盖标签与 freeze policy。
3. `LLM_API_KEY=... make docker-verify`：生成一致性、Router、登录、RAG/Agent SSE 和引用检查。
4. 打开四份核心报告：v2 Retrieval、v3 Retrieval、v3 Hybrid、v3 Agent；指出相同 dataset hash、cases/trials 和冻结 commit。
5. `LLM_API_KEY=... make docker-demo`：写入偏好、使用偏好、清空预算并切换区域。

不要在现场重新跑完整 challenge；它约需数分钟且设计为冻结后一次性运行。展示已保存报告即可。

## 推荐的简历表述

> 设计有界多步推荐 Agent，接入 Hybrid Retrieval、店铺详情/评价工具和结构化用户画像，通过 3-step/5-call 预算、EvidenceLedger 与引用/事实校验抑制循环和无依据推荐；基于 200 家店、1000 条评价构建 50 个正式 test/challenge 场景（86 次真实 DeepSeek trial），冻结 challenge 上 scenario-macro task success 较同任务 Hybrid RAG 提升 **41.67 个百分点**，成功回答 groundedness 100%。

更短版本：

> 构建可复现的有界推荐 Agent：200 店/1000 评价、50 个正式场景/86 trials；冻结 challenge 上同任务成功率较 Hybrid RAG 提升 41.67pp，成功回答 groundedness 100%，并量化 P50/P95、Token 与工具成本。

原简历中的“55 场景、+18.42pp”不要继续使用：55 与当前数据清单不一致；18.42pp 来自旧 harness，曾漏计多轮调用且 grader 较松。正式 v2 的 scenario-macro 增益是 +12.12pp，冻结 v3 是 +41.67pp。

## 指标怎么讲

- Retrieval v2 60 题是 regression：task success 100%，但 v3 challenge 120 题只有 64.17%，说明不能把已修复回归集的 100% 外推线上。
- v3 Agent 28 场景/48 trials：trial-micro outcome 52.08%，scenario-macro 53.57%，composite 45.83%，critical 全 trial 通过率 30%。
- 同任务 Hybrid：scenario-macro 11.90%，因此 Agent +41.67pp；双方都在相同场景、相同 trial 计划和相同 grader 上运行。
- 成功回答 groundedness 100% 不等于所有回答都正确：失败回答也可能引用或事实不合格；overall groundedness 为 93.75%。
- Agent P50/P95 7.51s/12.56s，Hybrid 2.77s/10.95s；Agent 平均 Token 6131，约为 Hybrid 的 3.16 倍。
- 结论是“复杂请求按需路由到 Agent”，不是“所有请求都走 Agent”。

## 面试官要求现场验证

```bash
make docker-reset
LLM_API_KEY='...' make docker-up
LLM_API_KEY='...' make docker-verify
LLM_API_KEY='...' make docker-demo

# 只读核验
docker compose exec -T mysql sh -lc 'mysql -uroot -p"$MYSQL_ROOT_PASSWORD" local_review_go -e \
  "SELECT COUNT(*) AS shops FROM tb_shop; SELECT COUNT(*) AS reviews FROM tb_blog;"'
docker compose exec -T app sh -lc \
  'redis-cli -h "$REDIS_ADDR" -p "$REDIS_PORT" -a "$REDIS_PASSWORD" FT.INFO idx:shop:vector'
```

实际凭据由环境提供；演示时不要把 key 或密码投屏。向量计数查看 `FT.INFO` 的 `num_docs`，索引名是 `idx:shop:vector`。

## 诚实限制

- 合成集不是线上分布，challenge 也不是秘密 benchmark。
- v3 no-result accuracy 12.5%、Router accuracy 79.17%，是优先改进项。
- 本地 feature-hash embedding 的错别字和开放域泛化有限。
- Agent 延迟和 Token 明显更高。
- MQ 向量刷新没有 outbox，极端失败时可能短暂陈旧。

完整追问与背诵答案见 `AGENT_INTERVIEW_GUIDE.md`。
