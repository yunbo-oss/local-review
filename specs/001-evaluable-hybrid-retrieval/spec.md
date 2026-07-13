# Feature Specification: Evaluable Hybrid Shop Retrieval

**Feature Branch**: `001-evaluable-hybrid-retrieval`

**Created**: 2026-07-12

**Status**: Draft

**Input**: User description: "完成 docs/plans/2026-07-11-recommend-agent-eval.md 中的改动；若不能一次写完则拆分。本规格覆盖计划中「可评测智能搜索 / Hybrid 检索」主线（Task 0–3），不含推荐 Agent 与记忆。"

**Source plan**: `docs/plans/2026-07-11-recommend-agent-eval.md`（§0.4 前置问题、§1.1 共享检索、§1.6 Retrieval eval、§1.8 实验协议、Tasks 0–3）

**Follow-on feature (out of scope here)**: Recommend Agent + structured memory + Agent eval（计划 Task 4–8）→ 另开 `002-*` 规格

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 可信检索评测基线 (Priority: P1)

作为维护者 / 面试演示者，我需要在**与线上推荐提问相同的过滤与检索决策路径**上，用一份人工核验过的题目集，跑出可复现的检索质量报告（含查询级成功率、多相关店召回/精确率、排序质量、过滤正确性、基础设施失败率），并保存为冻结基线；旧的「冒烟小样本」不得再被当成对外宣称的正式成绩。

**Why this priority**: 计划明确「先修再记录 baseline」；没有可信评测，后续 Hybrid 与 Agent 无法用数字证明收益，面试叙事站不住。

**Independent Test**: 在固定种子店铺数据与固定题目集上执行正式评测命令，得到含实验元数据与上述指标的报告；换机器复跑报告中的配置说明可得到同口径结果（允许基础设施失败被单独统计）。

**Acceptance Scenarios**:

1. **Given** 已发布的正式题目集（含冻结的 test 划分）与当前线上一致的「先解析过滤条件再检索」路径，**When** 以「诊断用：不过滤 + 纯语义」以外的正式模式跑评测，**Then** 报告同时给出 HitRate@5、Recall@5、Precision@5、MRR、nDCG@5、过滤相关指标与基础设施错误率，且不得把 HitRate 标成 Recall。
2. **Given** 同一题目集，**When** 分别以「人工给定过滤条件」与「由助手自动抽取过滤条件」跑评测，**Then** 两种模式都可独立出报告，便于隔离「检索器」与「过滤抽取」的贡献。
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

### User Story 3 - 混合检索可对照 Dense (Priority: P2)

作为维护者，我希望在同一题目集与同一过滤模式下，对比「纯语义检索」与「语义 + 文本相关性融合」两种策略，保存对照报告；无论提升、持平或下降，都要有失败样本可读，禁止无实验元数据的百分比宣传。

**Why this priority**: 计划规定 Hybrid 优先于昂贵的生成式重排；但必须先有 US1 基线才有对照意义。

**Independent Test**: 固定 filter 模式与题目集，分别跑 dense 与 hybrid，产出两份可并排比较的报告，并附失败案例分析。

**Acceptance Scenarios**:

1. **Given** 已冻结的正式题目集与 dense 基线报告，**When** 启用混合检索策略跑同一评测矩阵中的对应单元，**Then** 生成 hybrid 对照报告，指标口径与 dense 一致。
2. **Given** 某次对照显示 hybrid 在部分查询上变差，**When** 查看报告附属分析，**Then** 能指出失败查询类型（如纯语义 / 硬过滤 / 词面匹配）而非只给总分。
3. **Given** 对外描述「提升了 X%」，**When** 核对报告，**Then** 同时包含题目集版本与内容指纹、数据/索引版本、模型与维度、过滤模式、TopK、融合参数、`n_total` / `n_evaluated` / `n_infra_error`，并以百分点（pp）与相对提升同时表述。

---

### User Story 4 - 线上提问与评测共用同一检索入口 (Priority: P2)

作为用户，我通过现有智能点评对话提问时，得到的候选店铺应与评测台在相同策略下会检索到的逻辑一致，避免「线上一路、评测一路」。

**Why this priority**: 共享入口是可信评测的产品含义；可在 US1 指标正确后落地接线。

**Independent Test**: 对同一问题与同一显式过滤条件，线上对话路径与评测台在同一策略下返回的候选店铺集合一致（允许展示文案不同）。

**Acceptance Scenarios**:

1. **Given** 同一用户问题与相同显式过滤条件及同一检索策略，**When** 分别走线上智能点评检索与正式评测检索，**Then** 候选店铺 ID 列表一致。
2. **Given** 用户未给全过滤条件，**When** 系统自动补全过滤条件，**Then** 评测「自动过滤」模式使用同一套补全规则，而不是评测侧跳过过滤。

---

### Edge Cases

- 题目标注的相关店铺列表为空：正式集加载时必须拒绝，不得计入指标。
- 基础设施失败（向量服务超时、索引不可用等）：计入基础设施错误率，不进入质量指标分母；正式评测入口对不可接受的失败率以非成功状态退出。
- 文本相关性子检索失败：评测与对照实验中不得静默退化成纯语义却仍标注为 hybrid（避免口径漂移）；行为须在报告中可区分。
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
- **FR-004**: 正式评测 MUST 支持正交组合：过滤模式（不过滤诊断 / 人工给定过滤 / 自动抽取过滤）× 检索策略（纯语义 / 混合）；其中「不过滤 + 纯语义」仅作诊断，不得作为对外生产基线。
- **FR-005**: 线上智能点评检索与正式评测 MUST 共享同一套「解析过滤条件 → 按策略检索」决策路径。
- **FR-006**: 向量期望维度 MUST 有唯一配置真相来源；维度非法或与模型返回不一致时 MUST 失败，禁止静默回退。
- **FR-007**: 店铺向量正式写入路径 MUST 携带检索过滤所需的结构化元数据；不完整入库旁路 MUST 移除或不可用。
- **FR-008**: 支撑向量检索的运行依赖镜像 MUST 使用已验证的固定版本标识，并写入可复现实验说明。
- **FR-009**: 系统 MUST 支持语义检索与文本相关性结果的融合策略（混合检索），并与纯语义策略在同一题目集上对照。
- **FR-010**: 每次正式评测报告 MUST 包含实验元数据：题目集版本与内容指纹、种子/索引版本、模型与维度、过滤模式、策略、TopK、融合参数、样本量与基础设施失败数。
- **FR-011**: 质量指标分母 MUST 使用成功完成评测的样本数；基础设施失败单独统计。
- **FR-012**: 本规格范围 MUST NOT 包含：多步推荐助手、会话/长期偏好记忆、助手专用评测集、事实向量记忆、外部编排框架或更换向量数据库。

### Key Entities

- **RetrievalCase（检索评测题）**: 问题文本、相关店铺集合、人工过滤条件、dev/test 划分、标签、证据说明。
- **RetrievalReport（检索评测报告）**: 指标汇总、逐题结果、实验元数据、内容指纹、失败样本摘要。
- **FilterConditions（过滤条件）**: 区域、类型、价格上限等硬约束；来源可以是用户显式、自动抽取或评测人工给定。
- **ShopCandidate（店铺候选）**: 检索返回的店铺标识与排序位置；是 HitRate/Recall/MRR 的计算对象。
- **ExperimentBaseline（实验基线）**: 针对某题目集版本 + 过滤模式 + 策略冻结的报告快照，用于后续对照。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 维护者能在一小时内，用文档中的步骤对正式题目集跑完至少一种「生产口径」评测（自动过滤 + 纯语义），并得到含全部规定指标与实验元数据的报告。
- **SC-002**: 在同一正式 test 集上，HitRate@5、Recall@5 与 Precision@5 在存在多相关店铺或 TopK 含噪声的题目上可以不同（证明指标未混用）。
- **SC-003**: 对同一问题与显式过滤条件，线上对话检索与评测台在同一策略下的候选店铺 ID 列表一致率 100%（抽样至少 10 题）。
- **SC-004**: 配置维度与模型返回不一致时，相关操作 100% 失败且不产生错误维度的索引写入。
- **SC-005**: 正式题目集规模在 25～35 题之间，且每题均有人工核验记录（证据字段非空）；test 集内容指纹被记录并可在报告中核对。
- **SC-006**: 完成 dense 与 hybrid 至少各一份对照报告；若对外宣称提升，必须同时给出绝对百分点、相对变化与 n；若无提升，仍保留失败分析而非省略报告。
- **SC-007**: 冒烟小样本不再出现在任何「对外基线」或简历数字引用中。

## Assumptions

- 种子店铺与点评目录规模约数十家量级，正式题以现有可核验事实为准，不为凑题量批量同义改写膨胀 test。
- 现有智能点评对话入口继续存在；本规格只要求其检索路径与评测对齐，不改动其产品形态（流式回答等可保持）。
- 计划中的推荐 Agent、结构化记忆、Agent 评测属于下一规格；本规格交付的共享检索入口将被下一规格复用。
- 正式评测以**本地 Docker Compose + 开发机**为主（MySQL + Redis Stack + `cmd/eval-rag`）；边缘设备（如 AidLux 犀牛派）不作为 baseline 录制环境（见 `.cursor/skills/rag-eval/references/local-retrieval-eval.md` §5）
- 混合检索采用「可复现的融合」，不默认引入生成式重排。
- 源计划 `docs/plans/2026-07-11-recommend-agent-eval.md` 作为实现期详细任务与面试话术参考，但验收以本规格为准。

## Out of Scope

| 项 | 去向 |
|----|------|
| 登录态多步推荐助手、3 工具有界循环、SSE 推荐接口 | 规格 `002`（待开） |
| 会话列表 / 长期偏好档案、可纠正合并、并发合并 | 规格 `002` |
| Agent 场景题集与 groundedness / trajectory 评测 | 规格 `002` |
| 用户事实向量记忆、Mem0 / 图编排框架、更换向量库 | 不做（YAGNI） |
| 生成侧完整开放式回答质量 CI | 后置 |

## Interview Value & Knowledge Capture *(mandatory for this project)*

### Interview talking points

- [ ] 可信评测：线上路径一致；HitRate / Recall / Precision / MRR / nDCG 分层；报告带 dataset 指纹
- [ ] 检索优化：先 Hybrid 融合再考虑贵的生成式重排；用对照报告说话，不编数字
- [ ] 工程契约：向量维度单一真相来源；依赖镜像可复现；质量指标与基础设施失败分母分离

### Planned docs/solutions entries

- **interview/**: `RAG检索指标bundle`；`HitRate vs Recall vs Precision`；`评测与线上路径对齐`；`Hybrid 融合 vs 生成式重排取舍`
- **blockers/**: 实现期按需（维度不一致、索引/镜像复现、标注争议等）
