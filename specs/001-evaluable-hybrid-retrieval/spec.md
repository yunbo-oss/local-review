# Feature Specification: Evaluable Hybrid Shop Retrieval

**Feature Branch**: `001-evaluable-hybrid-retrieval`

**Created**: 2026-07-12

**Updated**: 2026-07-21 — 取消 dense vs hybrid 对照交付；线上默认 Hybrid；评测服务后续 Agent 对照

**Status**: Ready

**Input**: User description: "完成 docs/plans/2026-07-11-recommend-agent-eval.md 中的改动；若不能一次写完则拆分。本规格覆盖计划中「可评测智能搜索 / Hybrid 检索」主线（Task 0–3），不含推荐 Agent 与记忆。"

**Source plan**: `docs/plans/2026-07-11-recommend-agent-eval.md`（§0.4 前置问题、§1.1 共享检索、§1.6 Retrieval eval、§1.8 实验协议、Tasks 0–3；2026-07-21 取舍：跳过 dense A/B）

**Follow-on feature (out of scope here)**: Recommend Agent + structured memory + Agent eval（计划 Task 4–8）→ 另开 `002-*` 规格

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 可信检索评测基线 (Priority: P1)

作为维护者 / 面试演示者，我需要在**与线上推荐提问相同的过滤与检索决策路径**上，用一份人工核验过的题目集，跑出可复现的 **Hybrid 生产口径**检索质量报告（含查询级成功率、多相关店召回/精确率、排序质量、过滤正确性、基础设施失败率），并保存为冻结基线，供后续 Agent 对照；旧的「冒烟小样本」不得再被当成对外宣称的正式成绩。

**Why this priority**: 没有可信的 Hybrid RAG 基线，后续 Agent「提升了多少」无法归因；dense 对照已主动砍掉以节省面试与排期。

**Independent Test**: 在固定种子店铺数据与固定题目集上执行正式评测命令（默认 hybrid），得到含实验元数据与上述指标的报告。

**Acceptance Scenarios**:

1. **Given** 已发布的正式题目集（含冻结的 test 划分）与「先解析过滤条件再 Hybrid 检索」路径，**When** 以生产口径（自动过滤 + hybrid）跑评测，**Then** 报告同时给出 HitRate@5、Recall@5、Precision@5、MRR、nDCG@5、过滤相关指标与基础设施错误率，且不得把 HitRate 标成 Recall。
2. **Given** 同一题目集，**When** 分别以「人工给定过滤条件」与「由助手自动抽取过滤条件」跑 hybrid 评测，**Then** 两种模式都可独立出报告，便于隔离「过滤抽取」噪声（不要求再跑 dense）。
3. **Given** 历史上仅用于冒烟的小样本题目文件，**When** 查阅文档或默认正式评测入口，**Then** 明确区分 smoke 与正式集，正式基线只引用正式集与其内容指纹。

---

### User Story 2 - 配置与数据契约正确 (Priority: P1)

作为维护者，我需要向量维度、索引与写入路径遵守单一真相来源，避免「评测看起来能跑但结果不可信」；废弃的不完整店铺入库旁路不得继续存在；支撑向量检索的依赖镜像版本可锁定，便于复现实验。

**Why this priority**: 与 US1 同属 P1：前置正确性不过关则 US1 基线无意义。

**Independent Test**: 配置非法或不一致维度时系统拒绝继续；完整写入路径携带价格/评分等过滤元数据；依赖镜像不再使用浮动的 `latest` 标签（或等价不可复现引用）。

**Acceptance Scenarios**:

1. **Given** 配置的期望向量维度与模型实际返回维度不一致，**When** 生成向量或建立索引，**Then** 操作失败并给出可读原因，不得静默截断或改用另一维度。
2. **Given** 店铺向量写入，**When** 走正式入库路径，**Then** 过滤所需的价格、评分、评论数等元数据可被检索侧使用；不完整旁路已移除或不可达。
3. **Given** 本地编排文件中的向量存储服务镜像，**When** 查看配置，**Then** 指向已验证的固定版本标识，并在实验说明中可引用。

---

### User Story 3 - 默认混合检索上线 (Priority: P1)

作为维护者，我希望线上智能点评与评测默认使用「语义 + 文本相关性融合（Hybrid）」；**不要求**与纯语义做正式对照报告或简历叙事。纯语义仅可作为可选诊断开关。

**Why this priority**: 面试位置有限；Hybrid 直接作为生产路径，人日留给规格 `002` Agent。

**Independent Test**: 默认配置下检索走 Hybrid；文本子检索失败时不得静默标成 hybrid 成功；可录制 `hybrid_prod` 基线。

**Acceptance Scenarios**:

1. **Given** 默认配置，**When** 线上对话或正式评测检索，**Then** 使用 Hybrid（向量 KNN + 文本相关性 + 可复现融合），而非仅纯语义。
2. **Given** 文本相关性子检索失败，**When** 本次检索结束，**Then** 请求失败或明确标记失败，不得静默退化成纯语义却仍宣称 hybrid。
3. **Given** 完成正式 hybrid 评测，**When** 写入基线，**Then** 得到 `hybrid_prod` 快照（供后续 Agent 对照），**不要求**同时交付 dense 对照报告。

---

### User Story 4 - 线上提问与评测共用同一检索入口 (Priority: P1)

作为用户，我通过现有智能点评对话提问时，得到的候选店铺应与评测台在相同策略下会检索到的逻辑一致，避免「线上一路、评测一路」。

**Why this priority**: 共享入口是可信评测与后续 Agent 工具复用的前提；与 Hybrid 默认同批交付。

**Independent Test**: 对同一问题与同一显式过滤条件，线上对话路径与评测台在同一策略下返回的有序候选店铺 ID 列表一致（允许展示文案不同）。

**Acceptance Scenarios**:

1. **Given** 同一用户问题与相同显式过滤条件及同一检索策略（默认 hybrid），**When** 分别走线上智能点评检索与正式评测检索，**Then** 有序 TopK 店铺 ID 列表一致。
2. **Given** 用户未给全过滤条件，**When** 系统自动**抽取**过滤条件（本规格不含长期偏好档案补全，那是 `002`），**Then** 评测「自动过滤」模式使用同一套抽取规则，而不是评测侧跳过过滤。

---

### Edge Cases

- 题目标注的相关店铺列表为空：正式集加载时必须拒绝，不得计入指标。
- 基础设施失败（向量服务超时、索引不可用等）：计入基础设施错误率，不进入质量指标分母；正式评测入口对不可接受的失败率以非成功状态退出。
- 文本相关性子检索失败：不得静默退化成纯语义却仍标注为 hybrid。
- 无结果或条件冲突的题目：应保留在正式集中，用于验证「不硬凑结果」。
- 自动过滤抽取与人工给定过滤不一致：通过 Filter 类指标暴露，不得偷偷改写相关店铺标签来「刷分」。
- 题目集发现标注错误：不得为单次策略失败而改 test 标签；须记录变更原因并升级题目集版本。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: 系统 MUST 提供正式检索评测题目集（约 25～35 条），每条含问题、相关店铺、人工给定过滤条件、划分标记（dev/test）、标签与证据说明；test 划分一经冻结不得因单次跑分失败而改标。
- **FR-002**: 系统 MUST 将历史小样本题目仅保留为冒烟用途，并在文档与默认正式评测入口上与正式集区分。
- **FR-003**: 正式评测 MUST 输出检索指标 bundle（业界 RAG 检索层最低集 + 本项目 filter 指标）：
  - **Coverage / sanity**：HitRate@K（默认 K=5，即 Success@K）、Recall@K
  - **Ranking**：MRR、nDCG@K（二元 relevant；无分级标注时用 shop 是否 relevant）
  - **Noise**：Precision@K
  - **Filter**：FilterFieldAccuracy、FilterCompliance@K
  - **Infra**：InfraErrorRate（不进质量分母）
  - 禁止将 HitRate 命名或宣传为 Recall
- **FR-003b**: 生成侧 RAGAS 指标（faithfulness、answer_relevance、context_precision 等）属于规格 `002`，001 不阻塞交付
- **FR-004**: 正式评测 MUST 支持过滤模式（不过滤诊断 / 人工给定过滤 / 自动抽取过滤）；默认检索策略为 **hybrid**。纯语义（dense）MAY 作为诊断开关，**MUST NOT** 作为对外生产基线，**MUST NOT** 要求 dense vs hybrid 对照报告作为本规格验收项。
- **FR-005**: 线上智能点评检索与正式评测 MUST 共享同一套「解析过滤条件 → 按策略检索」决策路径；交付顺序：先共享入口 + hybrid 默认可评测，再接线线上对话（同一入口）。
- **FR-006**: 向量期望维度 MUST 有唯一配置真相来源；维度非法或与模型返回不一致时 MUST 失败，禁止静默回退。
- **FR-007**: 店铺向量正式写入路径 MUST 携带检索过滤所需的结构化元数据；不完整入库旁路 MUST 移除或不可用。
- **FR-008**: 支撑向量检索的运行依赖镜像 MUST 使用已验证的固定版本标识，并写入可复现实验说明。
- **FR-009**: 系统 MUST 默认启用语义检索与文本相关性结果的可复现融合（混合检索）；线上与正式评测默认一致。
- **FR-010**: 每次正式评测报告 MUST 包含实验元数据：题目集版本与内容指纹、种子/索引版本、模型与维度、过滤模式、策略、TopK、融合参数、样本量与基础设施失败数。
- **FR-011**: 质量指标分母 MUST 使用成功完成评测的样本数；基础设施失败单独统计。
- **FR-012**: 本规格范围 MUST NOT 包含：多步推荐助手、会话/长期偏好记忆、助手专用评测集、事实向量记忆、外部编排框架、更换向量数据库、**dense vs hybrid 对照交付与简历叙事**。

### Filter 指标操作定义（验收用）

- **FilterFieldAccuracy**：自动抽取结果与 `oracle_filter` 逐字段比较；字段缺失 vs 空值按「双方皆空算一致」；字符串精确匹配（区域/类型）；数值字段允许实现期约定的相等比较（默认精确）。
- **FilterCompliance@K**：返回的 TopK 中，满足本题硬过滤约束的店铺占比（无硬约束时该题记为 1.0）。

### Key Entities

- **RetrievalCase（检索评测题）**: 问题文本、相关店铺集合、人工过滤条件、dev/test 划分、标签、证据说明。
- **RetrievalReport（检索评测报告）**: 指标汇总、逐题结果、实验元数据、内容指纹、失败样本摘要。
- **FilterConditions（过滤条件）**: 区域、类型、价格上限等硬约束；来源可以是用户显式、自动抽取或评测人工给定（本规格不含 profile 补全）。
- **ShopCandidate（店铺候选）**: 检索返回的店铺标识与排序位置；是 HitRate/Recall/MRR 的计算对象。
- **ExperimentBaseline（实验基线）**: 针对某题目集版本 + 过滤模式 + **hybrid** 策略冻结的报告快照，供 `002` Agent 对照。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 维护者能在一小时内，用文档中的步骤对正式题目集跑完生产口径评测（自动过滤 + **hybrid**），并得到含全部规定指标与实验元数据的报告。
- **SC-002**: 在同一正式 test 集上，HitRate@5、Recall@5 与 Precision@5 在存在多相关店铺或 TopK 含噪声的题目上可以不同（证明指标未混用）。
- **SC-003**: 对同一问题与显式过滤条件，线上对话检索与评测台在同一策略下的**有序** TopK 店铺 ID 列表一致率 100%（抽样至少 10 题；评测期间索引只读）。
- **SC-004**: 配置维度与模型返回不一致时，相关操作 100% 失败且不产生错误维度的索引写入。
- **SC-005**: 正式题目集规模在 25～35 题之间，且每题均有人工核验记录（证据字段非空）；test 集内容指纹被记录并可在报告中核对。
- **SC-006**: 完成至少一份 **hybrid 生产口径**基线报告（`llm + hybrid`）；**不要求** dense 对照报告。对外宣称「相对单次 RAG 的 Agent 提升」属于规格 `002`。
- **SC-007**: 冒烟小样本不再出现在任何「对外基线」或简历数字引用中。

## Assumptions

- 种子店铺与点评目录规模约数十家量级，正式题以现有可核验事实为准，不为凑题量批量同义改写膨胀 test。
- 现有智能点评对话入口继续存在；本规格只要求其检索路径与评测对齐，不改动其产品形态（流式回答等可保持）。
- 计划中的推荐 Agent、结构化记忆、Agent 评测属于下一规格；本规格交付的共享 Hybrid 检索入口将被下一规格复用。
- 正式评测以**本地 Docker Compose + 开发机**为主（MySQL + Redis Stack + `cmd/eval-rag`）；边缘设备（如 AidLux 犀牛派）不作为 baseline 录制环境（见 `.cursor/skills/rag-eval/references/local-retrieval-eval.md` §5）
- 混合检索采用「可复现的融合」，不默认引入生成式重排。
- **2026-07-21 产品取舍**：不做 dense vs hybrid 对照交付与对外叙事；精力优先 Agent（`002`）。
- 源计划 `docs/plans/2026-07-11-recommend-agent-eval.md` 作为实现期详细任务与面试话术参考，但验收以本规格为准。

## Out of Scope

| 项 | 去向 |
|----|------|
| 登录态多步推荐助手、3 工具有界循环、SSE 推荐接口 | 规格 `002`（待开） |
| 会话列表 / 长期偏好档案、可纠正合并、并发合并 | 规格 `002` |
| Agent 场景题集与 groundedness / trajectory 评测；**Agent vs Hybrid RAG** 数字 | 规格 `002` |
| dense vs hybrid 正式对照报告与简历叙事 | 明确不做 |
| 用户事实向量记忆、Mem0 / 图编排框架、更换向量库 | 不做（YAGNI） |
| 生成侧完整开放式回答质量 CI | 后置 |

## Interview Value & Knowledge Capture *(mandatory for this project)*

### Interview talking points

- [ ] Hybrid 默认：词面 + 向量 RRF；不为 dense A/B 占简历
- [ ] 共享检索入口：线上 / eval / 后续 Agent 工具同一路径
- [ ] 工程契约：向量维度单一真相；质量指标与基础设施失败分母分离
- [ ] 下一规格才讲：Agent vs 单次 Hybrid RAG 的评测数字

### Planned docs/solutions entries

- **interview/**: `Hybrid默认与跳过dense对照的取舍`；`评测与线上路径对齐`；`HitRate vs Recall vs Precision`
- **blockers/**: 实现期按需（维度不一致、索引/镜像复现、标注争议等）
