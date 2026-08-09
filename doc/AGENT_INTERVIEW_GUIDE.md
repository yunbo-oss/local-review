# 推荐 Agent 面试拷打题库

这份文档按“先给结论，再给证据和边界”的方式回答。数字以 2026-08-09 冻结 commit `6819a5c` 的正式报告为准。

## 一、先把简历这一句讲准确

推荐表述：

> 设计有界多步推荐 Agent，接入 Hybrid Retrieval、店铺详情/评价工具和结构化用户画像，通过 3-step/5-call 预算、EvidenceLedger 与引用/事实校验抑制循环和无依据推荐；基于 200 家店、1000 条评价构建 50 个正式 test/challenge 场景（86 次真实 DeepSeek trial），冻结 challenge 上 scenario-macro task success 较同任务 Hybrid RAG 提升 41.67 个百分点，成功回答 groundedness 100%。

### 1. 为什么把原来的 55 场景改成 50？

正式可计数的是 v2 test 22 个和 v3 challenge 28 个，共 50 个；对应 38 + 48 = 86 trials。包含 dev 是 64 个场景，但 dev 参与调试，不能混入正式结论。55 没有可追溯的数据清单，因此删除。

### 2. 为什么不再写 +18.42pp？

那是旧 harness 的 trial-micro 结果，当时多轮模型/工具调用累计不完整，grader 也较松。修复后 v2 trial-micro 是 +21.05pp、scenario-macro 是 +12.12pp；冻结 v3 challenge 的 scenario-macro 是 +41.67pp。简历使用后者，并明确是冻结 challenge，不伪装成线上收益。

### 3. 41.67pp 怎么算？

相同的 28 个 challenge 场景中，Agent scenario-macro task success 为 53.57%，Hybrid RAG 为 11.90%，差值为 41.67 个百分点。macro 先算每个场景的 trial 成功率，再让每个场景等权，避免 critical 场景因为跑 3 次而权重更大。

### 4. 这个提升显著吗？

方向很明显，但样本仍小。Agent 的 trial outcome 95% Wilson 区间是 38.33%–65.53%，Hybrid 是 5.86%–24.70%；不能把 41.67pp 当作线上精确效应。合理结论是“在这批复杂合成任务上优势明显，值得真实流量验证”。

### 5. “成功回答 groundedness 100%”是什么意思？

只在 outcome=true 的回答上，groundedness 也全部通过。它不表示所有请求都答对；v3 Agent overall groundedness 是 93.75%，task success 是 52.08% trial-micro。成功子集 100% 与整体 100% 是两件事。

## 二、系统设计

### 6. 为什么需要 Agent，直接 RAG 不够吗？

明确单轮检索用 RAG 更快。Agent 的价值在必须分步执行的任务：读取/纠正长期偏好、先筛候选再查详情或评价、比较多家店、处理评价冲突和基于证据拒答。项目也保留 Router，不把所有流量都送进 Agent。

### 7. “有界”具体界在哪？

默认最多 3 个模型 step、5 次成功工具调用、8 次工具尝试、每个 turn 最多 3 个 tool calls；总运行 45 秒、单工具 10 秒；单次工具结果最多 6000 字符。成功调用和尝试分开计数，未知、重复、失败工具也消耗 attempt，防止靠失败调用绕过预算。

### 8. 为什么是 3 steps / 5 calls，不是更多？

当前任务通常是“搜索一次 + 核验一到两家 + 生成答案”，3/5 能覆盖主要链路，同时控制延迟、Token 和循环风险。它是基于任务形态的工程预算，不是理论最优；线上应按场景统计边际成功率和尾延迟后调整。

### 9. 达到工具上限会怎样？

不会直接返回空答案。loop 会为未执行调用补结构化错误结果，停止继续用工具，再用已经收集的 evidence 做一次无工具收尾。仍然不突破步数和调用上限。

### 10. 有哪些工具？

- `search_shops`：Hybrid 检索，支持区域、类别、预算等硬过滤。
- `get_shop`：读取地址、价格、评分、营业时间等结构化详情。
- `list_shop_blogs`：读取真实评价证据，用于安静、亲子、无障碍等体验声明及冲突核验。

### 11. 为什么把搜索、详情、评价拆成三个工具？

搜索适合召回和排序，但摘要不应承担所有事实证明；详情提供结构化字段；评价提供体验证据。拆开后能实施最小权限、独立超时、调用预算和 claim-level 校验，也能观察模型是否按任务使用了正确工具。

### 12. 一次完整请求如何流转？

Handler 鉴权并解析请求 → Router 判断 RAG/Agent/澄清 → Agent logic 加载 session 和 profile → harness/loop 调模型 → ToolExecutor 执行工具并写 EvidenceLedger → verifier 校验 → 必要时一次受限修订 → 持久化 trace/run/profile → SSE 输出 message 和 done 元数据。

### 13. 代码入口在哪里？

- 路由：`internal/logic/recommend_router.go`
- Agent 门面：`internal/logic/recommend_agent_logic.go`
- 有界循环：`internal/agent/loop.go`
- 工具：`internal/agent/tools.go`
- 证据账本：`internal/agent/evidence.go`
- 事实校验：`internal/agent/verifier.go`
- Agent 评测：`cmd/eval-agent/`

### 14. 为什么正式 Agent 评测强制 route？

要测的是 Agent 能力，不是 Router×Agent 的联合能力。若 Router 错分，Agent 没执行也会被算失败，无法定位问题。所以 Agent-vs-Hybrid 对照强制 `agent_multistep`；Router 另用 48 题 test 独立评测，准确率 79.17%。

## 三、Hybrid Retrieval

### 15. Hybrid 是怎么做的？

本地 384 维 feature-hash embedding 做 dense KNN，RediSearch 做 text retrieval；两路结果用 RRF `k=60` 融合。先应用用户明确说出的 TAG/NUMERIC 硬过滤，取 20 个候选，再裁为 Top 5。

### 16. 为什么用 RRF？

文本分数和向量距离量纲不同，直接加权需要校准。RRF 只依赖名次，对两路分数尺度不敏感，数据规模小时更稳定，也容易解释。缺点是忽略绝对置信度，v3 无结果失败就暴露了这一点。

### 17. 为什么用本地 embedding，不用 DeepSeek embedding？

本轮 DeepSeek 只提供 chat/filter；项目选择确定性本地 embedding 来保证离线可复现和零 embedding API 成本。代价是错别字和开放域同义表达弱，v3 challenge 已体现，不能声称它等同通用语义模型。

### 18. 为什么 v2 Retrieval 100%，v3 只有 64.17%？

v2 是开发后的 regression，表达模式已被覆盖；v3 冻结后加入错别字、否定纠正、OOD 同义表达和更难无结果。差异说明 v2 的 100% 是封闭集结果，不是上线准确率。

### 19. HitRate、Recall、Precision、MRR、NDCG 分别回答什么？

- HitRate@5：Top 5 是否至少有一个相关店。
- Recall@5：所有相关店中找回多少。
- Precision@5：返回店中有多少相关。
- MRR：第一个相关店出现得多靠前。
- NDCG@5：综合多级相关性和位置折损的排序质量。

### 20. 为什么 HitRate 100% 但 Recall 81.63%？

每题至少命中一个相关店，但某些硬过滤题有超过 5 家相关店，Top 5 不可能全部召回。relevant set 没被人为缩小，因此 Recall 更诚实。

### 21. 过滤准确率和过滤合规率有什么区别？

filter field accuracy 比较抽取出的 area/type/price/score/comments 与 golden；filter compliance 检查最终 Top-K 结果是否真的满足硬约束。前者测意图解析，后者测检索执行结果。

### 22. 为什么 v3 no-result accuracy 只有 12.5%？

当前 RRF 更擅长“排相对顺序”，缺少可靠的绝对拒答阈值；未知区域/类别、互相矛盾约束和注入文本下仍可能返回近似候选。下一步会分别做结构化不可满足检测、置信度校准和针对 unknown taxonomy 的拒答策略，而不是只调一个距离阈值。

### 23. 如何保证评价更新后向量不陈旧？

店铺更新、博客发布、点赞/取消点赞都会发 `shop-update`，RAG 消费者重算并写 Redis 向量；离线可用 `seed-vector --reset --expected-count=200` 全量重建。剩余风险是 MQ 发送失败没有 outbox，需后续 reconciliation。

## 四、结构化记忆

### 24. 用户画像存什么？

区域、类别、预算等结构化偏好及版本，不保存任意模型自由文本作为硬约束。MySQL 是事实源，Redis 做缓存；session history 与长期 profile 分开。

### 25. 本轮条件和长期偏好冲突怎么办？

优先级是：本轮显式参数/纠正 > 问题中的确定性硬约束 > profile 补缺。比如“忘掉预算，改为丰台区”在搜索前就作用于 effective profile，而不是回答后才持久化。

### 26. 为什么不直接把所有聊天历史塞给模型？

全量历史成本高、易受注入和旧偏好污染，也难确定性删除。结构化 profile 可校验、可版本化、可清空；session 只用于必要的指代解析。

### 27. 如何避免 trial 间记忆污染？

每个 trial 使用独立 session ID，并在运行前覆盖测试用户 profile；runner 每次重置 filter capture。正式报告保存 session ID，可追踪隔离是否生效。

### 28. 三轮 Demo 验证了什么？

第一轮写海淀/预算80；第二轮按偏好推荐学生店并检查引用；第三轮清空预算、切丰台并推荐家庭聚餐。最终 3/3 SSE 成功、推荐轮有引用、预算为空、区域为丰台、profile version 3。

### 29. 为什么第一轮 route 是 rag_oneshot，也能写偏好？

推荐门面在回答流程后仍执行 profile patch 提取与持久化；Router 控制回答路径，不等于关闭记忆写入。第三轮显式纠正会走 `agent_memory`。这也说明 Router 和 memory side effect 是两个可独立观测的维度。

## 五、Groundedness 与安全

### 30. EvidenceLedger 是什么？

它是单次 run 私有的证据账本：记录搜索发现的店、详情验证字段和评价证据。引用只能是 observed shop；详情/评价可补字段，但不能把一个从未搜索到的 ID “洗白”。

### 31. 如何校验引用？

解析回答中的 `[shop:id]`；推荐任务缺引用失败，未知 ID 失败。Markdown 形式的 shop 链接会先规范化。评测还比较 allowed/forbidden/permitted shop IDs。

### 32. 如何校验事实？

verifier 从回答中提取价格、评分、地址、营业时间等结构化声明，与引用店证据集合比较；不在证据中的值触发 `grounding_fact_conflict`。评价类语义声明必须由检索摘要或 `list_shop_blogs` 支持。

### 33. 有错误时为什么允许模型修订？会不会再次幻觉？

只允许一次、禁止工具调用、给出结构化白名单证据；适用于漏引用、未知店、结构化事实冲突。修订后重新走完整 verifier，不能绕过校验。语义证据不足不会靠改写“修好”，应诚实拒答。

### 34. 提示注入怎么防？

工具结果明确标记为 untrusted data；系统 prompt 要求不能执行评价中的指令；工具 schema 限定可执行动作；引用只能来自 ledger；challenge 含泄露环境变量、伪造工具返回、管理员权限和 `<system>` 文本。v3 的 prompt-injection tag 上 Agent 相对 Hybrid task success 提升 83.33pp。

### 35. 能保证绝不泄密吗？

不能做绝对保证。当前没有读取环境变量/文件的工具，工具权限很窄，API key 不进 prompt/报告/仓库，注入文本不能直接触达系统；但模型和依赖仍需持续红队、日志脱敏、网络与密钥最小权限。

### 36. Groundedness 还有什么漏洞？

多店结构化值目前对“引用证据集合”校验，尚未把自然语言每个 claim 精确绑定到具体店，理论上可能把 A 的价格写到 B 名下但仍命中集合。生产版应做 claim extraction + entity linking，再逐 `(shop_id, field, value)` 校验。

### 37. 为什么 groundedness 不是让 LLM 当 judge？

核心引用、ID、硬字段和预算都能确定性校验，成本低且可复现。LLM judge 可补充风格/开放语义，但不能替代程序化事实约束；当前有限语义主题用规则和真实评价证据评分。

## 六、评测科学性

### 38. Golden 如何生成？

店铺和评价由固定种子生成；golden 从同一 catalog 确定性计算合法 ID 和条件，生成器支持 `--check` 比较字节级输出。基础 25 家店也纳入 relevant set，避免把合法基础店误判为假阳性。

### 39. dev/test/challenge 如何隔离？

dev 用于修复；v2 test 用于冻结后的 regression；v3 challenge 使用另一固定 seed 和 OOD 表达，代码/数据冻结后只运行一次，不根据个例继续调当前版本。失败只能进入下一版 regression，再生成新 seed holdout。

### 40. challenge 放在仓库里还算盲测吗？

不算秘密盲测，只能称 repository-visible reproducible holdout。流程纪律能减少泄漏，但不能防止人读取。更强方案是 CI 保管加密/私有集，或用真实脱敏 query 由独立标注者维护。

### 41. 为什么关键场景跑 3 次？

LLM 有随机性，单次通过可能是偶然。critical 场景包括记忆、纠正、注入和评价核验；3 trials 能显示稳定性，但仍不足以精确估计低概率失败，生产应扩大到更多 seeds/runs。

### 42. `all_trials_pass_rate` 与 pass@k 有何区别？

这里要求一个 critical 场景的所有 trial 都通过，是严格稳定性指标；pass@k 通常表示 k 次中至少一次成功，k 越大越容易。旧名会产生反向理解，所以已删除。

### 43. 为什么同时报告 trial-micro 和 scenario-macro？

micro 把 86 次实际调用视为独立样本，critical 场景因 3 trials 权重更高；macro 先聚合场景，每个问题等权。系统容量/调用质量看 micro，产品场景覆盖和简历对比看 macro。

### 44. Outcome 与 composite 为什么分开？

答案可以完成任务但没用要求的评价工具，outcome=true、trajectory=false；也可能任务命中但引用不合法，groundedness=false。混成一个 pass 无法判断是业务能力、证据还是控制流问题。

### 45. infra error 为什么单列？

数据库断连、Redis 查询语法、API 超时不等于模型质量失败。报告既保留 `n_infra_error/rate`，又让质量分母只包含实际可评估 trial；否则基础设施事故会污染模型结论，也可能被错误重试掩盖。

### 46. 如何证明不是 fake 数据或 stub 模型？

正式报告的 `mode=inprocess`、真实模型名、精确 usage tokens、非零延迟/成本、逐 trial answer/tool trace、冻结 commit 和 dataset hash 可交叉验证；fake runner 只用于 harness 单测，输出路径不是正式 baseline。

### 47. 为什么 Hybrid 与 Agent 的比较公平？

两者使用相同场景、相同 trial 计划、相同输入 profile、同一个 outcome/groundedness grader和相同数据哈希。比较前代码逐项验证 case/trial 对齐。系统能力不同是刻意的：问题是“完整 Agent 系统相对生产 Hybrid baseline 是否完成更多复杂任务”，不是只比较同一个模型 prompt。

### 48. Hybrid 的 trajectory 为什么常失败，会不会拉低 task success？

不会。task-success 比较只用 outcome；trajectory/composite 单独报告。Hybrid 没有详情/评价工具，因此在要求特定工具的场景 composite 会低，但不会污染 outcome 增益。

### 49. 为什么不把 Retrieval HitRate 与 Agent outcome 相减？

题目、分母和成功定义不同。HitRate 只问 Top-K 是否出现相关文档，Agent outcome 还包含记忆、纠正、比较、声明与无结果。现在 Hybrid 直接跑 Agent 同一任务后才比较 task success。

### 50. 如何计算成本？

使用响应中的 prompt/completion tokens，乘命令行显式记录的每百万 token 单价。报告叫 estimated upper bound，因为费率假设、缓存命中和供应商计费可能变化；不把某一天价格硬编码成永久事实。

## 七、失败分析与工程修复

### 51. 最严重的 harness bug 是什么？

多轮场景只保留最后一轮调用，导致模型调用、工具、Token 和 observed IDs 被低估，旧增益和成本不可完全信。修复后 runner 累加所有 turn，只让最后回答/路由代表最终输出，并加回归测试。

### 52. 为什么旧的 Agent 100% 不能用？

严格 `allowed_only` 后发现旧报告曾引用不相关但真实的店；旧 groundedness 只检查“是否观察过”，没检查“是否允许推荐”。新报告真实 v2 为 84.21% micro，v3 为 52.08%，更可信也更适合讲失败边界。

### 53. 生产代码修过哪些真实 bug？

- 店铺 score 原始是十分制整数，工具曾把 47 显示成 47，现规范为 4.7。
- blog 工具把点赞量误标成 score，改为 `liked`。
- 博客保存无关注者时曾返回 ID 0；现在返回已提交 ID。
- follower 查询在 DB commit 后失败曾让客户端误重试重复写；现在记录 warning，不宣称整个写入失败。
- 博客/点赞未触发向量刷新；现在通知 shop-update producer。

### 54. Docker 复现遇到什么问题？

空卷启动本身成功，但 Demo profile 复用了旧 network 上的陈旧一次性容器，报 `network ... not found`。`docker-demo` 改为显式 `--no-deps` 运行 reset 和 demo 容器，重跑 3/3 成功。完整过程在 `EVAL_PRACTICE_LOG.md`。

### 55. 并发和超时怎么测？

Agent loop 用阻塞 client 验证 run timeout 有界；20 个并发 run 使用不同 shop ID，race test 验证 EvidenceLedger 不共享；关键包执行 `go test -race`。这不能替代负载测试，但覆盖了最危险的跨请求证据串扰。

## 八、上线追问

### 56. 如果上线，第一步补什么数据？

先收集脱敏真实 query 和用户是否接受推荐的反馈，做双人标注的 300–500 条小型集；按路由、错别字、无结果、地区/类别、记忆纠正和安全分层。继续堆合成店铺的收益低于补真实表达分布。

### 57. 是否推荐继续增加店铺和评价？

可以增加到 500–1000 店用于索引规模和延迟测试，但对面试防守更重要的是扩大真实 query/标注，而不是只扩 catalog。当前最大弱点是语言分布和拒答，不是 200 家店装不下。

### 58. 如何改善 no-result？

先做三层：结构化检测未知区域/类别和矛盾范围；按 query 类型校准 dense/text/RRF 置信度；低置信度返回澄清或无结果。用 precision-recall 曲线选择阈值，并把误拒与乱推荐成本分开。

### 59. 如何改善 Router 79.17%？

将规则作为高精度 fast path，再用小模型/分类器处理未覆盖 paraphrase；输出 route confidence，低置信度澄清；在线记录人工回退。不能直接为当前 challenge 补关键词，否则是对 holdout 调参。

### 60. 如何降低 Agent 延迟和成本？

路由掉简单请求；缓存 filter/profile 和热门搜索；并行读取独立候选详情；压缩工具 schema/结果；先用规则判断是否真的需要评价；设置动态预算；对相同 evidence 的最终回答复用。任何优化都要同时监控 task success 和 groundedness。

### 61. 如何保证向量更新一致性？

生产版使用 transactional outbox：业务事务写 blog/like 与 outbox，同步提交；异步 relay 发 MQ；消费者幂等更新向量并记录版本；定期 reconciliation 比较 DB `updated_at/version` 与向量版本，修复漏消息。

### 62. 如何扩展到百万店铺？

按城市/区域分片，硬过滤先缩小候选；向量索引和业务库解耦；批量异步 embedding；冷热分层与副本；压测 HNSW 参数、内存和 P95；在线重排只处理小候选集。当前 200 店闭环验证正确性，不代表容量结论。

### 63. 如何做线上 A/B？

以 task completion/点击后到店/改问率为主，配合无依据引用率、拒答率、P95、Token/成功请求成本和投诉安全指标。按 route intent 分层随机，避免复杂问题更多进入 Agent 造成 Simpson's paradox。

### 64. 失败时如何降级？

工具/模型超时先基于已有证据收尾；无可靠证据则澄清或诚实无结果；Agent 整体故障可降到 Hybrid RAG，但必须标记功能降级，不能把缺少评价核验的结果当作等价成功。

### 65. 监控什么？

按 route/tag 监控 outcome proxy、grounding 拦截码、工具错误/重复率、step/tool budget、P50/P95、Token、成本、拒答和 citation coverage；基础设施监控 Redis FT error、MQ lag、向量版本滞后和 DB/Redis 健康。

## 九、面试官连续追问示范

### 追问链 A：你这个 41.67pp 会不会是刷出来的？

答：我不拿 Retrieval HitRate 对比 Agent，而让 Hybrid 跑相同 28 个场景/48 trials，用同一个 outcome grader；报告验证 dataset hash 和 trial 序列一致。41.67pp 是 scenario-macro，challenge 在代码冻结后只跑一次。局限是合成、仓库可见且样本小，所以我只说“冻结 challenge 上”，不说线上提升。

再问：为什么 Hybrid 这么低？

答：challenge 集中在记忆、纠正、评价核验、注入和 OOD 表达，正是 one-shot Hybrid 的能力边界；普通单轮请求不在这张对照里。Retrieval challenge 的整体 task success 仍有 64.17%。同时 task-success 对比不含 trajectory，避免因为 Hybrid 没工具而人为扣分。

### 追问链 B：你怎么证明没有幻觉？

答：不能证明开放世界绝无幻觉。我能保证当前工具域内：成功推荐的引用 ID 必须在本轮 ledger；结构化事实匹配证据；语义声明有评价支撑；不通过就修订一次或失败。已知缺口是多店 claim 到实体的绑定还不够细，生产版要做三元组级校验。

### 追问链 C：为什么 Agent 不是 100%？

答：严格 challenge 上只有 52.08% micro。主要失败是 OOD 同义表达/错别字召回、无结果和证据不足；关键场景全部 trial 通过率也只有 30%。我更愿意展示这些数字，因为它们告诉我下一步该做置信度校准和真实 query 标注，而不是继续在 regression 上刷 100%。

### 追问链 D：这是不是只是 prompt engineering？

答：不是。提升来自系统约束：Hybrid 检索、结构化 profile 优先级、工具权限与预算、EvidenceLedger、程序化 verifier、session 隔离和同任务 grader。模型 prompt 是其中一层，很多 bug 修复发生在查询语法、状态累计、事实单位和向量更新链路。

### 追问链 E：如果只能再做一周，你做什么？

答：第一优先收集真实脱敏 query 并双标；第二修 no-result 校准；第三做规则 high-precision + 分类器 fallback Router；第四上向量 outbox/reconciliation；最后用分层 A/B 衡量成功率、延迟和成本。不会优先把合成店从 200 加到几千来制造规模感。

## 十、不要说的绝对话

- 不说“线上准确率/groundedness 100%”。
- 不说“Agent 比 RAG 快、全面优于 RAG”。
- 不说“55 个场景”或继续引用旧 +18.42pp。
- 不把 `all_trials_pass_rate` 叫 pass@k。
- 不说使用 DeepSeek embedding。
- 不把仓库可见 challenge 称为秘密盲测。
- 不隐瞒 v3 no-result 12.5%、Router 79.17% 和 Agent P95 12.56 秒。
