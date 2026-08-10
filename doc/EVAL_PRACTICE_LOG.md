# 本地可复现评测实践问题与修复日志

本文只记录本轮从“代码存在但从未真实运行”推进到可复现评测闭环时遇到的问题。正式实验条件、最终指标和演示命令见 `AGENT_AND_EVAL.md`；这里出现的中间分数均为诊断数据，不可作为正式项目结论。

## 记录原则

- 每个问题按“现象、根因、修复、验证、剩余限制”记录。
- 只有真实服务、真实数据和真实模型调用产生的报告可以进入正式结论。
- API 密钥只通过进程环境临时注入，不写入仓库、报告或日志文档。
- 修复 harness 后先跑定点回归，再跑完整 test split；dev 只用于开发诊断。

## 1. Docker 初始闭环不完整

**现象**：原 Compose 只能启动部分基础设施，无法一条命令完成迁移、数据初始化、向量初始化、评测和 Demo。

**根因**：迁移逻辑只存在于服务启动流程；seed、向量、评测和 Demo 都依赖宿主机手工操作；多个服务同时构建同名镜像还会触发 BuildKit 竞态。

**修复**：抽出可复用迁移入口并增加 `--migrate-only`；Compose 增加 `migrate`、`mysql-seed`、`redis-seed`、`vector-init`、`rocketmq-init`、`data-check`、`api-smoke`、三类正式评测和 memory demo；只由 `app` 构建镜像，其余任务复用该镜像。

**验证**：从空 volume 启动成功；MySQL 为 200 家店/1000 条评论；向量为 200 条；三类 seed 重复执行后计数不变；健康检查、登录、鉴权 API、秒杀和退出通过。

**剩余限制**：RocketMQ Broker 在本机 Docker Desktop 上内存占用较高，现场运行前应预留约 1 GB。

## 2. 生成 SQL 无法重复执行

**现象**：首次生成的 `seed-eval.sql` 在最后一条 VALUES 后提前出现分号，导致后面的 `ON DUPLICATE KEY UPDATE` 语法错误。

**根因**：生成器把最后一个元组的分隔符写成分号，而不是让整个 INSERT 在 upsert 子句之后结束。

**修复**：最后一个元组不输出逗号或分号，统一在 upsert 子句末尾结束语句；生成器增加固定随机种子和 `--check`。

**验证**：空库初始化和三次重复 seed 均保持 200/1000；生成文件哈希可重复。

## 3. Hybrid 检索全量基础设施错误

**现象**：oracle 评测 60/60 搜索失败，Redis 报 `Syntax error near text_content`。

**根因**：RediSearch 将 `@name|@text_content:(...)` 解析成非法字段表达式。

**修复**：改为 `(@name:(...) | @text_content:(...))`；strict 模式继续要求 dense/text 任一路报错都显式计入 infra error。

**验证**：基础设施错误率从 100% 降为 0%。

## 4. 近名店难例无法精确召回

**现象**：修复语法后，`静巷咖啡·国贸店` 和多组“双子旗舰店”仍被近名分店挤出 Top 5。

**根因**：整句自然语言被当成一个全文词项；中文店名中的间隔点又使普通 token 查询无法精确匹配。

**修复**：识别 `「...」` 精确店名并使用 RediSearch phrase query；对没有标点的自然店名查询增加前缀召回，并按去标点后的完整店名匹配稳定重排。

**验证**：7 个 test 词法难例在 oracle Hybrid 中全部命中。

## 5. Golden 把合法原始店铺判成假阳性

**现象**：价格/类别问题返回满足条件的原始 seed 店铺，却因为 relevant set 只含生成店 ID 而被判错。

**根因**：v2 生成器只看到了新增的 175 家店，没有把 `seed.sql` 的 25 家基础店纳入 catalog。

**修复**：生成器镜像基础店的检索元数据，并把所有满足硬条件的店纳入 relevant set，不再只取生成顺序前 5 家。

**验证**：oracle Hybrid 达到 100% task success、0% infra error；Recall@5 仍按完整 relevant set 计算，没有用缩小分母刷分。

## 6. 无结果样例统计口径错误

**现象**：无结果样例实际通过，但日志显示失败，filter compliance 还被记为 0。

**根因**：日志只看 HitRate；空返回传入普通 Top-K 合规函数后分母为零。

**修复**：日志改看 task success；明确 `expect_no_results` 且空返回时 filter compliance 记为 1；排序质量指标继续排除无结果题。

**验证**：4 个无结果题的 NoResult Accuracy 和过滤合规率均正确聚合。

## 7. Agent run 首次持久化失败

**现象**：Agent 可以调用模型，但 `agent_runs` 插入失败，MySQL JSON 列拒绝空字符串。

**根因**：Begin 阶段把 `evidence_summary_json` 初始化为 `""`。

**修复**：初始化为合法 JSON `{}`，完成阶段再写真实证据摘要。

**验证**：后续运行可得到 `COMPLETED/FAILED`、步数、调用数、Token、grounding 和 stop reason。

## 8. DeepSeek V4 工具调用兼容问题

**现象**：旧默认模型名已失效；V4 默认 thinking 模式与现有 OpenAI-compatible tool history 不完全兼容。

**根因**：项目沿用旧模型名，且没有显式声明 thinking 策略。

**修复**：默认模型更新为 `deepseek-v4-flash`；请求传输层显式发送 `thinking.type=disabled`；保留本地 deterministic embedding，避免不存在的 DeepSeek embedding API。

**验证**：真实流式聊天和 function calling 可用，Token usage 可从响应精确采集。

## 9. 多店价格回答被 groundedness 误杀

**现象**：回答“店 A 人均 42，店 B 人均 35”时，被判定店 B 与 42 元冲突。

**根因**：校验器只取回答中的第一个价格，再和所有引用店逐一比较。

**修复**：把回答中每个明确价格与“所有被引用店的证据价格集合”比较；任一未知价格仍失败。

**验证**：合法多店不同价格通过，虚构 99 元仍返回 `grounding_fact_conflict`。

## 10. Agent 错误路径把真实运行数据清成 0

**现象**：失败报告中明明模型和工具已运行，`Answer/Steps/ModelCalls/ToolCalls/Tokens` 却全为 0。

**根因**：门面层在 grounding/error 路径只返回 trace 元数据，没有返回 loop 已产生的数据。

**修复**：所有返回路径先构造完整 `RecommendResult`，错误仅改变状态，不丢弃回答、证据、调用和用量。

**验证**：失败案例报告可以用于根因分析，且不会把模型错误伪装成零成本。

## 11. 工具预算耗尽直接产生空答案

**现象**：首轮全量 Agent 中大量场景在第 3 步达到 5 次工具上限后直接结束，最终答案为空。

**根因**：循环在达到 tool call/attempt 上限时立即 `return error`，没有进入已有的无工具收尾逻辑。

**修复**：对未执行的 tool call 补齐结构化错误结果，然后跳出工具阶段，用已有证据做一次有界无工具收尾；仍不突破步数和工具数上限。

**验证**：多轮记忆定点场景从空答案变为 outcome/groundedness/trajectory 全通过。

## 12. 评测记录的 filter 被后续搜索覆盖

**现象**：回答和首次搜索使用了正确的区域/类型/预算，但报告显示 `typeName` 或 `maxPrice` 缺失。

**根因**：capture 只保留最后一次 search filter；模型后续为查详情发起无 filter 搜索时覆盖了前值。

**修复**：按字段累计本 trial 的有效硬约束；新一轮显式非空值可覆盖旧值，空值不清除。

**验证**：无结果、精确店名、价格等定点场景不再因 capture 丢字段而失败。

## 13. 模型省略可选工具参数导致约束漂移

**现象**：相同问题多 trial 中，模型偶尔在 `search_shops` 的 query 写了“咖啡/预算”，但没有填写对应可选字段。

**根因**：只依赖模型把自然语言硬条件复制到 tool arguments，存在不必要的随机性。

**修复**：在 Agent 入口对有限的区域/类别词表和明确数值预算做确定性提取；工具显式参数优先，问题硬条件补缺，最后才由 profile 补缺。

**验证**：朝阳区/咖啡/50 元定点场景的 filter 和 outcome 全通过。

## 14. 引用格式和 groundedness grader 过松

**现象**：模型偶尔输出 `[喜茶](shop:12)`；旧 grader 对没有任何 `[shop:id]` 的店铺回答也给 groundedness 通过。

**根因**：生成模型偏好 Markdown 链接写法；grader 只检查“已有引用是否属于 observed”，没有检查成功店铺回答必须有引用。

**修复**：将常见 Markdown shop 链接规范化为 `[shop:id]`；对仅缺引用的回答允许一次无工具、不得新增事实的受限修订；grader 要求有正例目标的成功回答至少一个引用。

**验证**：引用修订仍经过 evidence ledger 校验；未知 ID 和事实冲突不会被自动修复。

## 15. 语义偏好被店名或常识替代证据

**现象**：模型根据“静巷”店名推断适合学习，或把基础详情没有写无障碍误当作完整证据。

**根因**：工具描述没有区分结构化详情与体验评价；模型把预算一词也当成“学生平价”语义，压低了无障碍等主意图的向量权重。

**修复**：明确语义适配必须调用 `list_shop_blogs`，`get_shop` 只证明地址/价格/营业时间；限制一次搜索后最多核验两个候选；本地 embedding 删除“预算 → 学生平价”的错误语义别名。

**验证**：用 `local-feature-hash-zh-v2` 重建 200 条向量后，无障碍、安静办公和语义无结果定点场景全部通过；最终 60 题 Retrieval 和 38 trial Agent 正式评测的语义相关场景均通过。

## 16. 对比场景的 forbidden golden 自相矛盾

**现象**：22 个 test 场景单 trial 预检达到 21/22；唯一失败回答正确推荐国贸店，但因对比时引用望京店而被判失败。

**根因**：问题明确要求对比两家店，grounded 对比必须引用两家证据；golden 却把被比较的望京店放入 `forbidden_shop_ids`。

**修复**：保留 `allowed_shop_ids=[26]`，要求回答至少命中正确推荐；移除“不得引用被比较对象”的矛盾约束。禁止集合仍只用于真正不能出现在回答中的难负样本。

**验证**：重新生成 golden 后完整 test split 通过；groundedness 仍要求比较中出现的每个引用都来自本轮证据。

## 17. Trial 之间的 filter capture 串场

**现象**：无 filter 的下一场报告偶尔显示上一场的区域、类型和预算。

**根因**：评测 runner 复用了 `capturingSearch`，但每个 trial 开始时没有清理累计状态。

**修复**：每个 trial 开始显式重置 filter、degraded 标志和原因；同时把 loop 的重复工具调用计数透传到正式报告。

**验证**：增加 trial reset 单测；最终正式报告中无约束场景未再携带前一场 filter，8 个关键场景的全部 trial 通过率为 100%。

## 18. JSON-only 过滤器偶发带解释前缀

**现象**：Hybrid RAG 某个 trial 返回一段中文说明后再给 JSON，旧解析器整段反序列化失败并把 filter 降为 nil。

**根因**：兼容层只处理纯 JSON 或 Markdown code fence，没有处理“说明 + 单个 JSON 对象”。

**修复**：在不存在/移除 code fence 后提取首个 `{` 到最后一个 `}` 的外层对象再严格反序列化；解析失败仍显式记录 warning。

**验证**：新增“解释前缀 + JSON”单测；最终 Hybrid baseline 使用修复后的解析器重跑，38 trial 的基础设施错误率为 0%。

## 19. 语义偏好被误抽成硬类别

**现象**：正式 Retrieval 的 task success 为 100%，但两题 filter field accuracy 只有 5/6：模型把“家庭聚餐”“学生平价”推断成了用户没有明说的店铺类别。

**根因**：LLM filter 允许基于常识补全可选硬字段；一旦作为 RediSearch pre-filter，会在排序前错误排除其他类别的合法语义候选。

**修复**：新增 hard-filter sanitizer：区域、类别、预算只接受用户原文明确出现的有限 catalog 词表/数字；语义偏好继续留在查询文本参与 dense + text RRF 排序。评分和评论数仍由明确措辞提取。

**验证**：新增“家庭聚餐不得推断美食”和“明确咖啡/50 元需规范化”单测；正式 Retrieval 的 filter field accuracy 从 96.67% 提升到 100%，同任务 Hybrid 也使用相同 sanitizer 重跑。

## 20. 同任务对照不能混用 Retrieval HitRate

**现象**：初版报告试图用 Agent outcome 减 Retrieval HitRate，得到一个看似直观但不可解释的“提升”。

**根因**：两者的题目、样本数、trial 数和成功定义不同；Retrieval 命中相关文档不等于完成多轮记忆、比较、纠正或无结果任务。

**修复**：让 Hybrid RAG 直接运行 Agent 的同一份 test 场景和同一份 trial 计划，并复用相同 outcome/groundedness/composite grader；报告写入数据哈希、场景 ID 和 trial 序列，比较前逐项校验。

**当时验证**：双方均运行 22 个场景、38 个 trial，数据哈希一致，infra error 均为 0%。该轮曾报告 Agent 100%、Hybrid 81.58%、+18.42pp；后续第 40～42 项发现多轮遥测漏计和 grader 过松，因此这个分数已作废，不再用于结论。

## 21. 成功率、引用和稳定性被一个分数混在一起

**现象**：初版 tag 指标无法区分“任务没做成”和“任务做成但轨迹/引用不合格”；错误命名的 `pass_at_k` 也会让人误以为是“至少一次成功”。

**根因**：报告只聚合单一 pass，且没有说明多 trial 分母。

**修复**：分开输出 outcome、groundedness、trajectory 和 composite；tag 级别分别提供 outcome 与 composite；将旧字段改为 `all_trials_pass_rate`，只统计至少 3 trial 的关键场景，要求该场景所有 trial 都通过。

**当时验证**：JSON 中已不存在 `pass_at_k`，三个维度可独立解释；当时的 100% 数字后来被更严格 grader 推翻。现正式 v2 Agent outcome/composite/all-trials 为 84.21%/84.21%/62.50%，v3 challenge 为 52.08%/45.83%/30.00%。

## 22. 首轮分数不能直接作为项目结论

**现象**：首轮 Agent 只有 26.3% composite，通过失败日志才发现其中既有真实模型失败，也有 harness、golden、查询语法和记录逻辑错误。

**根因**：项目此前从未端到端运行，评测器本身没有先经过定点回归和可观测性验证。

**修复**：保留每个失败 trial 的回答、步骤、工具、Token、stop reason 和 grader 原因；按“单测 → 定点真实调用 → 单 trial test → 关键场景三 trial → 完整正式评测”逐层收敛，修复后重新生成并覆盖正式报告。

**当时验证**：Retrieval 60/60 task success，Agent 38/38 composite。后续严格 allowed-only 和全 turn 累计发现旧 Agent 报告仍有漏判，因此 38/38 已被新正式报告替代；首轮 26.3% 和旧 100% 都只保留为过程记录。

## 23. 流式 API 不接受标准 Bearer 头

**现象**：空卷 API smoke 的普通鉴权接口全部通过，RAG/Agent SSE 改用标准 `Authorization: Bearer <token>` 后却返回 401。

**根因**：JWT 中间件把整个 header 当作 JWT 解析，只兼容前端历史使用的裸 token；解析失败日志还会输出完整 token。

**修复**：鉴权入口同时兼容裸 token 与大小写不敏感的 Bearer scheme；解析错误只记录错误类型，不再把 token 写入日志。

**验证**：增加 token normalization 单测，并用最终 Docker 镜像重跑登录、RAG SSE、Agent SSE 和引用检查。

## 24. API smoke 第二次执行在秒杀接口提前退出

**现象**：空卷首次 API smoke 可以通过；重复执行时，测试用户已下过同一秒杀单，接口返回预期的“已抢购”业务 4xx，但脚本在输出判定前直接退出。

**根因**：脚本启用了 `set -e`，秒杀请求又使用 `curl -f`；任何 HTTP 4xx 都会先触发 shell 失败，后面的“已抢购/库存/429 均可接受”逻辑实际上走不到。

**修复**：同时采集响应体与 HTTP 状态，不对该请求使用 `-f`；明确接受首次 200、重复购买/库存业务响应和 429，其他状态仍失败。

**验证**：在不清理订单的情况下连续运行 Docker API smoke，并继续执行后面的 RAG/Agent SSE 与引用校验。

## 25. 宿主机 smoke 找错 Redis 容器名

**现象**：Compose 服务正常、验证码也已写入 Redis，但从宿主机运行脚本仍退回固定验证码并登录失败。

**根因**：脚本能模糊发现 `local-review-redis-1`，实际读取时却硬编码执行不存在的 `local-review-redis`。

**修复**：通过 Compose service 名执行 Redis CLI，不再依赖 Compose 自动生成的具体容器名；密码仍来自环境变量。

**验证**：宿主机与 `api-smoke` 容器均能读取刚生成的验证码并完成登录。

## 26. SSE 分片让引用检查产生假失败

**现象**：RAG 实际输出了 `[shop:26]`，脚本却报告缺少引用。

**根因**：模型流式响应把引用拆成 `[`、`shop`、`:`、`26`、`]` 多个 `data:` frame；直接在原始 SSE 文本上匹配连续字符串必然失败。

**修复**：按 SSE event 类型拼接所有 `message` 的 data payload，再在重建后的最终回答中检查引用；API smoke 和三轮 memory demo 共用同一解析规则。

**验证**：保持小粒度流式分片不变，重建后的回答可以稳定识别 `[shop:id]`。

## 27. RAG 上下文有评价摘要时丢失结构化事实

**现象**：检索结果包含人均 42 元，但 RAG 回答称“没有价格信息”。

**根因**：上下文构造使用互斥分支：只要 `text_content` 非空，就只输出 embedding 文本，丢掉人均、区域、类型、评分、评论数和销量。

**修复**：每个候选始终输出结构化事实，再追加明确标为“不可信数据”的检索评价摘要和实际评论；新增上下文单测。

**验证**：最终 Docker RAG smoke 同时核验 SSE 完成事件、`[shop:id]` 引用和价格事实可用。

## 28. 记忆 Demo 自己构造了不可满足的条件

**现象**：正式 memory golden 已通过，但三轮 Demo 的“海淀、100 元、安静咖啡”连续两轮诚实返回无结果，脚本又要求第二轮必须有引用。

**根因**：固定数据中没有同时满足海淀、咖啡和安静办公评价证据的店；Demo 问题没有复用已验证的 golden 条件。

**修复**：前两轮复用正式 critical memory 场景：先写入“海淀、预算 80”，再按偏好查适合学生的店；第三轮清空预算、切换丰台区并查已标注的家庭聚餐候选。

**验证**：三轮均要求完整 SSE；后两轮要求 grounded citation；最终 Redis profile 必须同时满足预算已清空和丰台区已记录。

## 29. 整目录脚本挂载覆盖了镜像 entrypoint 权限

**现象**：为了避免每次改 smoke 脚本都重建 Go 镜像，把整个 `script/` 目录只读挂载进容器后，容器启动报 `docker-entrypoint.sh: permission denied`。

**根因**：宿主机脚本是普通 0644；目录挂载覆盖了镜像构建阶段已 `chmod +x` 的 entrypoint。

**修复**：只挂载 API smoke 或 memory demo 当前需要的单个脚本文件；镜像内 entrypoint 保持可执行，任务脚本仍由 `bash` 显式执行。

**验证**：Compose 配置检查通过，两个 profile 任务无需重建镜像即可读取最新 harness。

## 30. Demo 固定用户残留偏好污染下一次运行

**现象**：修正 Demo 题目后仍被上一轮遗留的“咖啡、预算 80”硬过滤，导致第三轮丰台区家庭聚餐无候选。

**根因**：Demo 固定使用同一测试用户和 session，但只重置业务 seed，没有重置 MySQL profile 事实源、Redis profile cache、session 和限流窗口。

**修复**：增加只针对 demo 手机号的 `memory-demo-reset` 初始化任务，清除该用户的 MySQL profile/event；Demo 登录取得 user id 后同步删除该用户的 Redis legacy/cache、固定 session 和限流 key。不会清除其他用户记忆。

**验证**：连续执行三轮 Demo 时，每次都从空偏好开始，最终 profile 仍严格检查预算清空和丰台区纠正。

## 31. “忘掉预算”在本轮回答之后才生效

**现象**：即使 Demo 已从空 profile 开始，第三轮明确说“忘掉预算”，搜索仍沿用上一轮 80 元上限，把所有已标注的丰台区家庭聚餐候选提前过滤掉。

**根因**：推荐流程先加载旧 profile 并完成搜索/回答，最后才调用模型抽取并持久化 profile patch；当前轮纠正无法影响当前轮检索。模型偶发输出完整 profile 而非 patch 时，严格解析又会让纠正完全丢失。

**修复**：把明确的“忘掉/清空预算”先应用到本轮 effective profile；回答后仍持久化 `budget_max=0`。对有限区域、类别和明确预算的“以后优先/改为”语言增加确定性 patch，与合法模型 patch 合并；若模型 JSON 结构错误，只有这些用户明说字段可以兜底。

**验证**：增加“本轮预算抑制”和“丰台替换海淀、预算清空、不推断家庭聚餐类别”的单测；最终三轮 Demo 再验证 Redis/MySQL profile 状态。

## 32. RocketMQ 健康检查反复启动 JVM

**现象**：Broker 日志已经显示 boot success，但 Compose 长时间停在 `health: starting`；CPU 超过 300%，内存接近 1 GB。

**根因**：健康检查每 5 秒执行一次 Java `mqadmin clusterList`，在 4 GB Docker Desktop 上单次超过 8 秒，超时后立即重试，反而拖慢 Broker。

**修复**：健康检查改为 Broker 10911 端口的轻量 TCP 探测；`rocketmq-init` 仍使用真实 `mqadmin updateTopic`，因此 topic 功能验证没有被端口检查替代。

**验证**：从停止状态重建 Broker 后应快速 healthy，三个 topic 初始化仍 exit 0。

## 33. 精确店名查询被长期区域偏好排除

**现象**：三轮 Demo 把偏好切到丰台区后，RAG 查询明确的「静巷咖啡·国贸店」却只返回丰台候选，目标店 26 在检索前被排除。

**根因**：RAG/Agent 都把 profile 用作空字段默认值，但没有区分“泛化推荐”和“用户精确点名”；精确店名问题没有显式区域，因此被旧区域补空。

**修复**：识别闭合的 `「店名」` 精确查找意图，RAG 和 Agent 本轮均不继承区域、类别、预算 profile；检索结果本身仍受引用和事实校验。

**验证**：新增普通查询、闭合精确店名和未闭合引号单测；在完成记忆 Demo 后再次运行 API smoke。

## 34. 否定句中的旧区域抢先成为硬过滤条件

**现象**：场景写明“改成海淀区，预算 50，不要沿用朝阳区”，检索却连续两次使用朝阳区。

**根因**：有限区域扫描按固定词表取第一个命中，没有区分纠正目标与被否定的旧值。

**修复**：优先解析“改成/改为/换成”的区域；普通扫描明确跳过“不要沿用”的区域。硬约束仍只来自用户原话。

**验证**：增加同时包含新旧区域和预算的回归测试，要求最终过滤为海淀区、50 元。

正式复跑中 a2-04 的 3 个关键 trial 全部通过，检索过滤均使用海淀区和 50 元上限。

## 35. 模型在已观察候选之外补出一个熟悉品牌门店

**现象**：答案同时引用两个真实观察候选和一个未观察店铺，Outcome 通过但 Groundedness 失败。

**根因**：原有有界修复只处理“漏写引用”，未知店铺引用直接失败；即使现有证据已经足够，也没有一次删除幻觉内容的机会。

**修复**：对 `grounding_unknown_shop` 增加一次无工具、白名单限定的重写；要求删除未知店名、事实和引用，随后重新执行完整 verifier。事实冲突和语义无证据仍不自动修复。

**验证**：单测固定输入 `[shop:1]` 与 `[shop:999]`，修复只能保留 `[shop:1]`，模型调用增加一次且工具调用保持为零。

补充发现修复后的答案虽已通过 verifier，`LoopResult` 仍保留第一次失败的 `Err/GroundingCode`。现已在最终校验成功时同步清空旧错误状态，避免上层日志出现“GroundingOK=true 但错误码仍失败”的矛盾记录。

正式复跑中原先混入未知店铺的 a2-16 同时通过 Outcome 和 Groundedness；完整报告的成功回答 groundedness 为 100%。

## 36. 语义证据候选被普通类别文本挤出工具 TopK

**现象**：“适合家庭聚餐并照顾孩子”的场景检索到若干名称含“亲子”的普通店，但正式标注的家庭评价证据候选未进入工具返回，最终只能诚实报无结果。

**根因**：向量/关键词融合在有限 TopK 内偏爱类别和店名词重合；最终 verifier 要求评价证据，但排序阶段没有复用同一证据规则。

**修复**：语义任务先取 20 个硬过滤后的候选，使用与 grounding 完全相同的有限语义规则把有评价证据的候选稳定前置，再裁剪回模型请求的 TopK；店铺对比任务不重排。

**验证**：新增共享语义证据规则单测，并在正式 test 场景上复跑验证，避免只优化离线 Retrieval golden。

当时复跑中 a2-20 从 Outcome 失败恢复为通过；这证明了该定点修复，但随后第 40～42 项升级 harness/grader 后，完整 38/38 结论不再有效。

## 40. 多轮 Agent 遥测只保留最后一轮

**现象**：多轮场景的报告看起来模型调用、工具调用、Token 和 observed shop 很少，旧成本与成功率无法和实际链路对应。

**根因**：runner 在每一轮覆盖 trial actual，只保留最终回答对应的一轮；前序轮的调用与证据丢失。

**修复**：所有 turn 累加 model calls、tool calls、duplicate calls、Token 和 observed IDs；只有最终 answer、filter、route 和 trace 使用最后一轮结果，并增加多轮 runner 回归测试。

**验证**：正式 v2 Agent 平均 4.45 model calls / 2.45 tool calls / 5361 Token；v3 为 5.35 / 2.06 / 6131，报告与真实多轮链路一致。旧 +18.42pp 和旧成本数字因此废弃。

## 41. Groundedness 只检查“见过”，没有检查“该不该推荐”

**现象**：旧正式报告的某个 trial 引用了真实观察过但与目标无关的店，仍被判通过。

**根因**：grader 只验证 citation ⊆ observed，没有 `allowed_only`、允许/禁止店集合及 claim coverage。

**修复**：增加 `allowed_only`、`permitted_shop_ids`、required/forbidden answer、required claim 和 required tools；groundedness 分开报告 citation legality 与 claim coverage。

**验证**：严格重跑后 v2 Agent 不再是 100%，而是 84.21% trial-micro；v3 challenge 为 52.08%。成功回答 groundedness 仍为 100%。

## 42. 工具事实单位和字段语义错误

**现象**：工具把数据库 score=47 展示成 47 分；评价点赞字段又被命名为 score，模型可能生成错误事实。

**根因**：数据库评分使用十分制整数，工具层未归一化；blog DTO 沿用了含糊字段名。

**修复**：评分统一转换为 4.7 形式；评价字段改名为 `liked`；verifier 新增评分、地址和营业时间校验，事实冲突只能基于白名单证据修订。

**验证**：单位和字段单测通过，成功回答在 v2/v3 的 groundedness 均为 100%。

## 43. 正式 Router test 暴露同义改写召回不足

**现象**：dev 12/12，但冻结 test 只有 38/48；多步比较、记忆修改的同义表达常被路由到 RAG。

**根因**：Router 是高精度关键词/规则 fast path，paraphrase 覆盖有限；quoted injection 文本还可能含路由触发词。

**修复**：冻结前仅补充明确 intent suffix、无历史歧义澄清和轻量 HasHistory；正式 test 后不再按个例补关键词。

**当时验证**：test accuracy 79.17%、infra error 0；错误完整保存。该 test 后续只作为 regression，不再称为未见集。

## 44. v3 challenge 推翻“回归集 100% 等于上线可用”

**现象**：v2 Retrieval task success 100%，v3 challenge 降至 64.17%；Agent v3 也只有 52.08% trial-micro。

**根因**：v2 已参与修复，表达较规则；v3 增加错别字、否定纠正、OOD 同义表达、未知 taxonomy 和更难拒答。

**修复**：不针对 v3 个例改行为；保留失败，作为下一版 regression 输入。

**验证**：v3 Retrieval 120、Agent/Hybrid 各 48 trials 全部真实执行，infra error 0；no-result accuracy 12.5% 被列为最高优先级限制。

## 45. Demo profile 复用已删除 Docker network

**现象**：第一次 reset 成功，随后 memory demo 报 `network ... not found`。

**根因**：旧 profile 一次性容器残留，Compose 自动拉起依赖时尝试复用已经被 `down` 删除的 network。

**修复**：`docker-demo` 显式用 `--no-deps` 分别运行 `memory-demo-reset` 和 `memory-demo`，不复用陈旧 profile 容器；`docker-reset` 同时启用所有 profile 再执行 `down -v --remove-orphans`，确保下次空卷验证会清理这些一次性容器。

**验证**：重跑 3/3 SSE 成功，第 2/3 轮含引用，预算清空且丰台区纠正写入 profile version 3。

## 46. Challenge manifest 字段容易误读

**现象**：challenge 已正式执行，但生成 manifest 仍显示 `formal_evaluation_executed=false`，容易被误认为报告未运行。

**根因**：manifest 是确定性生成产物，字段实际表达“生成命令没有调用模型”，命名却像运行状态。

**修复**：改名为 `formal_evaluation_executed_at_generation=false`；正式执行状态由带 started_at、git commit、dataset hash 的 report 证明。

**验证**：challenge 三个生成文件仍可 `--check` 字节复现，正式报告保留冻结 commit。

## 47. 博客变化没有刷新检索向量

**现象**：新评价或点赞改变 Top reviews 后，向量文档仍可能保留旧文本；保存博客无关注者时还曾返回 ID 0。

**根因**：BlogLogic 未注入 shop-update producer；DB commit 后 follower 查询失败被当作整体失败；返回值走了无关注者早退路径。

**修复**：认证用户覆盖请求体 userId，提交后始终返回真实 ID；follower 查询失败只告警；保存和 like/unlike 通知向量刷新，并用注入 Redis/MQ 依赖补测试。

**验证**：保存/点赞触发刷新测试通过。剩余限制是 MQ 发送失败缺少 transactional outbox，需 reconciliation 兜底。

## 48. 临时验收命令硬编码了错误的数据库与索引名

**现象**：最终服务全部 healthy，但人工只读计数先后报 MySQL database 不存在、Redis 未认证和 RediSearch index 不存在。

**根因**：临时命令沿用了猜测的 `local_review`、`idx:shop` 和容器内不存在的 Redis 密码变量；项目实际配置为 `local_review_go`、`idx:shop:vector`，Redis 密码属于应用容器环境。

**修复**：验收命令从 MySQL/应用容器已有环境读取凭据，并使用代码定义的规范数据库与索引名，不把密码写入日志或文档。

**验证**：最终只读计数为 MySQL `shops=200`、`blogs=1000`，RediSearch 索引文档数为 `200`。

## 49. Docker build cache 挤满宿主磁盘

**现象**：最后一次镜像构建长时间停在 Go 编译且没有新输出；宿主数据盘只剩约 1.5 GB。

**根因**：多轮镜像重建留下 13.91 GB 可回收 BuildKit cache，同时运行 App 和 RocketMQ 又加剧了 4 GB Docker Desktop 的资源竞争。

**修复**：先停止当前卡住的构建和无状态 App/RocketMQ 服务，仅执行 `docker builder prune -f` 清理可重建缓存；MySQL、Redis 数据卷和正式报告均未删除。释放 13.91 GB 后进行一次冷构建。

**验证**：五个 Go 二进制与最终镜像重新构建成功，随后 App、MySQL、Redis、RocketMQ Broker/NameServer 全部恢复 healthy，初始化任务仍为 exit 0。

## 50. zsh 的 `status` 只读变量让健康等待命令假失败

**现象**：Compose 已启动 App，但最终健康等待命令以 `read-only variable: status` 退出。

**根因**：验收命令在 zsh 中把循环变量命名为 `status`；该名称是 zsh 的只读特殊参数。

**修复**：改用 `app_health`，健康判断本身不变。

**验证**：最终镜像的 `/health` 返回成功，`docker compose ps -a` 显示四个长期服务和两个 RocketMQ 组件均 healthy，迁移、MySQL seed、Redis seed、向量初始化和 topic 初始化均 exit 0。

## 51. 严格 golden 与工具约束产生假低分

**现象**：v3 失败中混有三类并非模型能力的问题：合法基础店未进入允许集合；事实查询引用了店铺却被当成“错误推荐”；`allowed_only` 只检查截断后的候选，遗漏其他满足硬条件的店。部分 trial 还因单轮工具调用数量没有受控而轨迹失败。

**根因**：推荐结果、事实引用和检索候选在评分器中没有分层；golden 的允许集合不是从完整 catalog 计算；总工具预算存在，但单轮模型可以一次请求过多工具。

**修复**：回答增加结构化 `推荐结果：` 区域，将“引用店铺”和“推荐店铺”分开评分；事实查询只要求引用合法，不强制视作推荐；允许集合从完整 catalog 穷举计算；增加每轮最多 3 个工具的限制，并补充 required cited IDs、claims 和 evidence gate。

**验证**：相关 grader、golden generator 和 per-turn budget 单测通过；v3 保持不变，修复只进入 v3.1 regression 和新 seed v4。

## 52. 先回归再冻结 v4，避免继续针对 challenge 调参

**现象**：如果直接在 v3 逐题修复并反复重跑，最终高分无法区分系统改进和对测试集记忆。

**根因**：首版 challenge 已被查看，继续沿用同一版本会造成开发集与测试集泄漏。

**修复**：保持 v3 文件和原始报告不可变；将已确认失败模式放入 v3.1 regression；使用独立 seed 和重新措辞生成 v4。按当前范围移除错别字题，但保留语义偏好、冲突、拒答、证据、多轮记忆和安全场景。

**验证**：v3、v3.1、v4 生成器均通过字节级 `--check`；v4 有 8 dev / 28 challenge、48 challenge trials，Agent 与 Hybrid 报告的数据哈希和 trial 序列完全一致。

## 53. v4 正式运行保留唯一失败，不再继续调分

**现象**：v4 Agent 通过 47/48 trials；`a4-20` 的一个长多轮指代 trial 中，profile 和工具调用正确，但最终回答沿用前文“暂不推荐”，没有输出推荐。

**处理**：将该失败原样保留，不再按逐题结果修改 prompt、harness 或 golden。正式结论使用 scenario-macro 96.43% 和 trial-micro 97.92%，并同时给出 Wilson 95% 区间约 89.10%–99.63%。

**验证**：成功回答 groundedness 100%、trajectory 100%、critical 全 trial 通过率 100%、infra error 0%；同任务 one-shot Hybrid scenario-macro 为 46.43%，固定路由 Agent 的差异为 +50.00 个百分点。

**剩余限制**：v4 是仓库可见的合成 holdout，且多轮记忆/证据型任务占比高；结果不能外推成线上全流量提升。

## 54. 最终空卷验证暴露冒烟题、订单零日期和 Demo 初始化问题

**现象**：空卷服务与初始化成功，但 Agent smoke 使用宽泛语义题时无法保证命中有评价证据的候选；秒杀异步入库在 MySQL strict mode 下拒绝 Go 零时间；API smoke 消耗随机验证码后，记忆 Demo 再用固定验证码会登录失败。

**根因**：冒烟测试错误地依赖语义排序而不是验证接口契约；未支付订单的 `time.Time` 被序列化成 `0000-00-00`；Demo 没有在自身启动前恢复确定性验证码。

**修复**：Agent smoke 改用精确店名和已知评价证据，只验证 SSE、引用和事实链路；订单的支付、核销、退款时间改为 `*time.Time`，未发生时写 SQL NULL；`docker-demo` 先幂等运行 `redis-seed`，再清理指定 demo 用户的 profile/session。

**验证**：从空 volume 启动后，200 家店、1000 条评价和 200 个向量均正确；健康检查、登录、API、RAG、Agent、引用、秒杀和三轮记忆 Demo 全部通过；重复 seed 和重复 Demo 均可执行。

## 55. Router v1 规则精度高但同义意图召回不足

**现象**：v1 test 只有 38/48（79.17%）；`agent_multistep` recall 61.54%，`agent_memory` recall 72.73%。规则经常把“分别有什么优缺点”“开到几点”“把人均限制删掉”“上一轮第二家”等请求回落到 RAG，引用文本中的“忘掉预算”还会误触发 memory。

**根因**：旧 Router 使用少量完整关键词，没有将请求抽象成比较、详情、证据核验、偏好变更和历史指代五类特征；真实意图后缀只支持少数固定连接词。

**修复**：先提交并冻结 12 dev / 52 challenge 的 `router.v2`，再把 Router 改为分层规则：force override、真实意图边界、偏好字段×变更动作、多步意图族、session follow-up 和缺失上下文澄清。`NeedsSessionHistory` 使用同一历史指代规则，普通 RAG 请求不读取 session。

**验证**：rules v2 在 v1 regression 为 48/48；在冻结 v2 challenge 首次运行 52/52，macro-F1 100%，相对隔离复跑的 rules v1 29/52 提升 44.23 个百分点。两份 v2 报告哈希均为 `sha256:4a04e1fd...b97fe9b92`，运行 commit 分别为 `a55f026` 与 `82cfe52`，Git 均 clean。

**剩余限制**：v2 是人工设计、类别平衡且仓库可见的集合，规则和标签体系来自同一项目；高分不能替代真实流量、独立标注、混合意图与错别字评测，也不能宣称线上路由 100%。

## 56. 临时 HTTP 冒烟没有 `set -e` 产生假通过

**现象**：第一次宿主机 Router 冒烟中，本机缺少 `redis-cli`，登录与推荐请求依次返回 400/401，但命令最后仍打印 `Router HTTP clarify smoke passed`。

**根因**：临时多行 shell 没有启用 fail-fast；每个 `grep` 失败后仍继续执行，最后一个 `printf` 以 0 退出，掩盖了前序错误。

**修复**：验收命令使用 `set -euo pipefail`，通过 Compose Redis 容器读取验证码，并对验证码、token、SSE message/done、`route=clarify` 和澄清正文逐项断言；正式 `api-test.sh` 同步升级为 `set -euo pipefail`。

**验证**：真实登录返回 200，`POST /api/recommend` 返回 200 SSE，包含确定性澄清消息和 `route=clarify`；服务日志同时保留了第一次 400/401 与第二次 200/200，便于确认假阳性已被推翻。

## 57. Docker 缓存构建受本机代理与资源竞争阻塞

**现象**：首次构建在通过 `docker.m.daocloud.io` 获取 `golang:1.24-alpine` 元数据时连接本机 `127.0.0.1:7897` 超时；禁止 pull 后成功进入 Go 编译，但 Docker Desktop 4GB 环境同时运行 App、RocketMQ 和三个分布式 Go 实例，构建层长期没有进度。

**处理**：只停止本项目无状态容器释放资源，保留 MySQL/Redis volumes；在确认继续等待无收益后取消构建，再恢复原服务。Compose 全 profile 配置检查已通过，宿主机全量 Go 测试和真实 HTTP Router 冒烟通过。

**剩余限制**：本轮没有把新镜像完整构建到结束，因此不能宣称此次 Router 改动完成了新的空卷 Docker 验收。阻塞点是本机 Docker 代理/资源，不是 Go 编译错误；在资源充足或代理正常的环境中仍应执行 `docker compose build app && make docker-verify`。
