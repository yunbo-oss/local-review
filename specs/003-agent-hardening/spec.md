# Feature Specification: Agent Hardening + 002 Closure

**Feature Branch**: `003-agent-hardening`

**Created**: 2026-07-26

**Status**: Draft

**Input**: User description: "003 并入加固计划代码项与 002 未完成事项；评测相关也要写进 003，但执行顺序是先完成代码改动再搞评测。"

**Source plans**:
- `docs/plans/2026-07-25-local-review-agent-hardening.md`（可信化 + 路由 + 工程外壳 + 评测收口）
- `specs/002-recommend-agent-memory/`（已交付主体；未完成任务并入本规格）

**Prerequisite**: `001` Hybrid 共享检索；`002` 已落地有界三工具 Agent、SSE、Redis 记忆、graders 骨架与 `agent.v1.json`。

**Delivery order（硬约束）**:

```text
Phase A — 代码与单测/冒烟（US1–US6）
    → 合并门槛：自动化测试绿，不要求正式评测报告
Phase B — 评测、演示与文档收口（US7，含原 002 未完成项）
    → 合并/对外叙事门槛：eval 报告非 stub + 对照 + demo
```

本规格**包含**评测相关需求，但 **Phase B 不得阻塞 Phase A 的代码合并**；对外宣称「Agent 评测闭环完成」必须以 Phase B 为准。

---

## Absorbed from 002（未完成 / 勾选超前）

下列原属 `002` US5 / Polish，**全部由本规格承接**（tasks 编号供追溯）：

| 原任务 | 内容 | 并入 |
|--------|------|------|
| T041 | OTel span 接线到 loop / logic / search | Phase A（US6） |
| T042 | 完整 `eval-agent` harness（真实 trial + graders + SHA-256 + 对照） | Phase B（US7） |
| T043 缺口 | Makefile 仅有 `eval-agent` stub 目标，缺 `demo-agent` | Phase B |
| T044 | `script/agent-demo.sh` 三轮记忆演示 | Phase B |
| T045 | 录制 `hybrid_prod` 基线（001 对照前提） | Phase B |
| T046 | `doc/AGENT_AND_EVAL.md` | Phase B |
| T047 | README + activeContext 收口 | Phase B |
| T049 | quickstart 场景验收 | Phase B |
| T050 | function-calling 不可用时启动/冒烟失败清晰 + 文档 | Phase A 冒烟 + Phase B 文档 |
| T051 | SSE 不泄漏 tool raw / profile / CoT（handler 测试） | Phase A（US3/US6） |
| T052 | 全量回归：`go test` + eval-rag-prod + eval-agent + demo-agent | Phase B |
| T053 | 勾选 002 Interview Value 面试点 | Phase B |
| 勾选超前 | 缺失 `handler/agent_test.go`、`recommend_agent_logic_test.go` | Phase A 补齐 |

`002` 已交付且本规格复用：Memory merge/extract、ToolChatClient、三工具、bounded loop、ID groundedness、SSE 入口、进程内限流（将被分布式替换）、graders 纯函数、`agent.v1.json`。

---

## Gap Snapshot（截至 2026-07-26）

| 能力 | 现状 | 阶段 |
|------|------|------|
| EvidenceLedger + 事实校验 | 仅 ID ⊆ observed；空引用可过；空 blogs 可污染 | A |
| Agent 工具层 profile 强制补空 | RAG 有；Agent search 未强制 | A |
| tool-loop attempt / 严格 JSON / UTF-8 裁剪 | 部分有界；未达加固口径 | A |
| MySQL profile / Run / ToolCall | Redis-only | A |
| Harness + ContextBuilder | 无；Logic 门面 + 20 条裁剪 | A |
| Trace 接线 + SSE `trace_id` | 辅助函数未调用 | A |
| Redis 分布式限流 | 进程内 map | A |
| 检索降级 strict/degraded | 无 | A |
| RecommendRouter | 无；双入口客户端自选 | A |
| SSE / Logic 单测补齐 | 文件缺失 | A |
| `eval-agent` 真实 harness | **stub** | B |
| `demo-agent` / `agent-demo.sh` | 无 | B |
| `hybrid_prod` 基线 | 无 | B |
| `AGENT_AND_EVAL.md` / README 收口 | 无 | B |
| Agent vs Hybrid RAG 对照数字 | 无 | B |

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 可信推荐：证据与事实可追溯 (Priority: P1) — Phase A

作为用户，我需要助手推荐的店铺与关键事实都来自本轮真实工具结果；不能通过空评价或编造引用洗白。

**Why this priority**: 可信底线；相对单次 RAG 对照的前提。

**Independent Test**: 空引用、未观察 ID、空 blogs 洗白、事实冲突、无结果路径的自动化用例。

**Acceptance Scenarios**:

1. **Given** 回答提到店铺但无 `[shop:id]`，**When** 校验，**Then** 不得作为成功推荐发出。
2. **Given** 引用 ID 不在本轮可引用证据中，**When** 校验，**Then** 失败，不发送成功正文。
3. **Given** 对未发现 ID 得到空评价列表，**When** 引用该 ID，**Then** 失败。
4. **Given** 合法引用但均价等与证据冲突，**When** 校验，**Then** 删除冲突事实或整答失败。
5. **Given** 无合适候选，**When** 无结果说明，**Then** 允许无引用且不虚构店铺。

---

### User Story 2 - 偏好在搜索时由系统强制生效 (Priority: P1) — Phase A

作为已保存偏好的用户，未再说明区域时搜索应补上长期偏好；本轮明确另一区域则以本轮为准。

**Independent Test**: 补空 / 覆盖 / 清空预算三类用例。

**Acceptance Scenarios**:

1. **Given** 偏好海淀且本轮未指定区域，**When** 搜索，**Then** 条件含海淀（或等价规范化）。
2. **Given** 偏好海淀且本轮明确朝阳，**When** 搜索，**Then** 使用朝阳。
3. **Given** 已清空预算，**When** 后续搜索，**Then** 不再施加旧预算。

---

### User Story 3 - 有界执行不空转、不泄漏 (Priority: P1) — Phase A

作为维护者，非法/重复/超量工具调用仍消耗预算并停止；评价不得改写策略；SSE 不泄漏 tool raw / 完整偏好 / 思维链。

**Independent Test**: 去重、单轮上限、畸形 JSON、超长截断、注入文案、SSE 事件内容抽检（承接 002 T051）。

**Acceptance Scenarios**:

1. **Given** 相同工具+参数再次请求，**When** 处理，**Then** 拒绝执行且计入尝试预算。
2. **Given** 单轮工具调用数超上限，**When** 执行，**Then** 在上限处停止。
3. **Given** 非法 JSON / 未知字段，**When** 解析，**Then** 拒绝并返回稳定错误类别。
4. **Given** 评价含诱导指令，**When** 进入上下文，**Then** 仅作数据；用户只见公共错误。
5. **Given** 成功或失败的 SSE 流，**When** 检查事件载荷，**Then** 无 tool 原始 JSON、完整 profile 转储或思维链。

---

### User Story 4 - 可审计运行与长期偏好可靠存储 (Priority: P1) — Phase A

作为维护者，每次运行可查询摘要与工具轨迹（无隐私正文）；缓存丢失后偏好可恢复；并发合并不丢已提交变更。

**Acceptance Scenarios**:

1. **Given** 已有长期偏好，**When** 短期缓存清空后再推荐，**Then** 偏好仍生效。
2. **Given** 两路几乎同时更新偏好，**When** 均成功，**Then** 最终包含双方已提交变更（允许重试后一致）。
3. **Given** 运行结束，**When** 按运行标识查询，**Then** 可见终态/计数/路径/校验摘要，无完整原文或评价正文。
4. **Given** 客户端断连且未成功结束，**When** 收口，**Then** 运行为取消/失败终态，且不写长期偏好。

---

### User Story 5 - 简单问走快路径、复杂问走助手 (Priority: P1) — Phase A

系统自动分流：清晰单轮 → 快速检索回答；核实评价/纠偏好/多步 → 完整助手；支持强制路径覆盖（供评测与演示）。

**Acceptance Scenarios**:

1. **Given** 清晰单轮条件且无纠偏好，**When** 统一推荐入口，**Then** 走快速检索回答路径。
2. **Given** 纠偏好或要评价/对比，**When** 路由，**Then** 走完整助手路径。
3. **Given** 强制路径覆盖，**When** 处理，**Then** 忽略自动路由。
4. **Given** 完成，**When** 查看完成元数据，**Then** 含路径与原因类别。

---

### User Story 6 - 编排外壳：上下文预算、Trace 与限流 (Priority: P2) — Phase A

编排阶段清晰；超长历史不挤掉规则与当前问题；观测 span 实际接线；多实例共享限流；检索失败显式降级（承接 002 T041 与加固 B3–B6）。

**Acceptance Scenarios**:

1. **Given** 极长历史消息，**When** 构建上下文，**Then** 规则与当前问题保留且不超过预算。
2. **Given** 一次推荐，**When** 查看完成元数据与观测，**Then** 含运行标识、步骤/工具计数、停止原因；span 可区分工具阶段且无正文泄漏。
3. **Given** 三实例下连续超频，**When** 请求，**Then** 在进入主推理前按共享配额拒绝。
4. **Given** 检索部分失败，**When** 返回，**Then** 显式失败或降级标记，不静默当完整成功。

---

### User Story 7 - 可复现评测、演示与文档收口 (Priority: P1) — Phase B

作为维护者/面试演示者，我需要在代码就绪后：跑正式 Agent 场景集得到非 stub 报告（outcome / groundedness / trajectory / 多 trial / 成本），与 Hybrid 生产口径对照，跑通三轮记忆 demo，并有可复现文档（承接 002 T042–T053 与加固阶段 C）。

**Why this priority**: 002 未闭环的对外验收；但**必须在 Phase A 之后执行**。

**Independent Test**: `make eval-agent` 报告无 stub 标记；关键题 ≥3 trial；`make demo-agent` 三轮；文档可独立复现。

**Acceptance Scenarios**:

1. **Given** 正式 Agent 场景集（约 10～15 条，含记忆/多步/无结果/失败/防循环），**When** 执行评测，**Then** 报告含 outcome、groundedness、trajectory、trial 汇总、成本摘要、数据集指纹与实验元数据；基础设施失败单列。
2. **Given** 至少 5 条关键场景各不少于 3 次 trial，**When** 汇总，**Then** 报告呈现跨 trial 成功率（不得只用单次结果）。
3. **Given** 已录制的 Hybrid 生产口径基线与本次 Agent 报告，**When** 撰写对照，**Then** 叙事为 **Agent vs Hybrid RAG**，含成功率与延迟或调用量至少之一，并固定实验条件；无 dense vs hybrid 数字。
4. **Given** 演示脚本，**When** 执行「陈述偏好 → 同会话追问 → 纠正/清空」，**Then** 第 2 轮体现补全、第 3 轮后偏好终态正确（抽检 3 次全过）；可打印脱敏摘要与运行标识。
5. **Given** 文档步骤，**When** 维护者按文档操作，**Then** 一小时内可跑完正式评测并理解成功/失败样例（观测无隐私正文）。
6. **Given** 路由已启用，**When** 评测可选模式，**Then** 可报告 `force_route` 与自动路由下的分 tag 成功/成本（用于校准，非阻断 Phase A）。

---

### Edge Cases

- 短期缓存不可用、长期偏好可用：仍可推荐。
- 长期偏好加载失败：仅用本轮条件，记录警告。
- 检索部分失败：显式失败/降级。
- 多实例超频：共享配额，推理前拒绝。
- 强制路径覆盖自动路由。
- 评测遇模型/基础设施抖动：计入 infra，不进入质量分母；关键题靠多 trial。
- 聊天模型不支持 function calling：启动或冒烟须失败清晰，不得静默退化成无工具胡荐。

## Requirements *(mandatory)*

### Functional Requirements — Phase A（代码）

- **FR-001**: 系统 MUST 维护本轮证据集合；空评价 MUST NOT 单独授予可引用身份。
- **FR-002**: 成功推荐 MUST 至少含一个合法店铺引用，除非明确无合适结果。
- **FR-003**: 成功推荐引用 MUST ⊆ 本轮可引用证据；校验失败 MUST NOT 发送成功正文。
- **FR-004**: 强事实与证据冲突时 MUST 删除该事实或拒绝整答。
- **FR-005**: 搜索 MUST 按「本轮显式 > 本轮抽取 > 长期偏好仅补空」合并条件。
- **FR-006**: 工具尝试（含失败/重复/未知）MUST 计入预算；达上限 MUST 停止。
- **FR-007**: 工具参数 MUST 严格校验；非法结构 MUST 拒绝并返回稳定错误类别。
- **FR-008**: 工具返回与评价 MUST 视为不可信数据；SSE MUST NOT 泄漏 tool raw / 完整偏好 / 思维链。
- **FR-009**: 长期偏好 MUST 以可恢复长期存储为事实来源；缓存丢失 MUST NOT 导致永久丢失。
- **FR-010**: 偏好合并 MUST 支持纠正/清空，并发下不静默丢已提交变更。
- **FR-011**: 每次运行 MUST 有可查询摘要；MUST NOT 持久化完整用户原文、完整评价或隐藏推理。
- **FR-012**: 未成功结束的运行 MUST NOT 写长期偏好。
- **FR-013**: 系统 MUST 提供路径路由（快速检索回答 vs 完整助手）并支持强制覆盖。
- **FR-014**: 路由决策与原因类别 MUST 写入运行摘要/完成元数据。
- **FR-015**: 多实例下用户推荐频率限制 MUST 共享；超限 MUST 在主推理前拒绝。
- **FR-016**: 上下文构建 MUST 保证系统规则与当前问题不被挤出，并遵守长度预算。
- **FR-017**: 检索降级或失败 MUST 显式标记。
- **FR-018**: 推荐相关自动化测试 MUST 覆盖 handler SSE 契约与主要 Logic 成功/失败路径（补齐 002 缺失单测）。
- **FR-019**: 观测 span MUST 实际接入运行路径（非仅定义未调用）。

### Functional Requirements — Phase B（评测与收口，原 002 未完成）

- **FR-020**: 系统 MUST 提供可执行的 Agent 离线评测入口：对正式场景集跑真实推荐、捕获轨迹与终态、执行 outcome/groundedness/trajectory 评分、输出含数据集指纹的报告；报告 MUST NOT 为「仅加载 golden」的 stub。
- **FR-021**: 关键场景（不少于 5 条）MUST 支持每条不少于 3 次 trial 的汇总成功率写入报告。
- **FR-022**: 评测 MUST 能与 Hybrid 生产口径基线对照，对外叙事 MUST 为 **Agent vs Hybrid RAG**；MUST NOT 使用 dense vs hybrid 作为验收数字。
- **FR-023**: 系统 MUST 提供可复现的多轮记忆演示路径（陈述偏好 → 追问 → 纠正/清空）。
- **FR-024**: 维护者文档 MUST 说明如何复现评测与演示、成功/失败样例解读，以及为何不做 Mem0/记忆工具/dense A/B。
- **FR-025**: 产品说明（README 等）MUST 指向推荐入口、评测与演示方式；规格 002 面试点在文档落地后勾选完成。
- **FR-026**: 全量回归 MUST 覆盖单元测试、检索生产口径评测、Agent 评测与演示目标（在环境具备时）。

### Explicitly deferred（仍不做）

- **FR-027**: 本规格 MUST NOT 包含：通用 query 改写默认开启（HyDE/Multi-Query）、完整 NLU 意图平台、多 Agent 拆分、Mem0/LangGraph/更换向量库、开放式文笔 CI 强门禁。

### Key Entities

- **Evidence Record（本轮）**: 发现/核实状态、可引用字段与来源、评价标识。
- **User Preference Profile**: 区域/品类/预算/忌口/摘要与版本。
- **Recommendation Run**: 终态、路径、计数、校验摘要、用量/延迟、运行标识。
- **Tool Attempt**: 工具名、参数摘要、状态、耗时、错误类别。
- **Route Decision**: 路径与原因类别。
- **Agent Eval Report**: 指标汇总、逐题/trial、实验元数据、与 Hybrid 对照节（Phase B）。
- **Demo Transcript**: 三轮演示的可复现记录（脱敏）。

## Success Criteria *(mandatory)*

### Phase A — 代码验收（可先合并）

- **SC-A01**: 「空引用 / 未观察引用 / 空评价洗白 / 事实冲突 / 无结果」自动化用例上，违规成功推荐检出率 100%。
- **SC-A02**: 「偏好补空 / 本轮覆盖 / 清空预算」搜索条件一致率 100%。
- **SC-A03**: 强制触达预算的用例（≥10 次）100% 停止继续工具调用。
- **SC-A04**: 清空短期偏好缓存后，抽检 ≥5 次长期偏好恢复成功率 100%。
- **SC-A05**: 路由表驱动用例（单轮 / 纠偏好 / 评价对比 / 强制覆盖）通过率 100%；完成元数据含路径。
- **SC-A06**: 未登录与超频 100% 在主推理前拒绝（多实例共享限流后仍成立）。
- **SC-A07**: 成功与失败运行各抽检 ≥3 次，均可查终态摘要且无完整问题/评价正文；观测路径已接线。
- **SC-A08**: SSE 泄漏抽检用例 100% 通过；handler/logic 关键路径单测存在且绿。
- **SC-A09**: Phase A 不以正式 `eval-agent` 非 stub 报告为合并门槛。

### Phase B — 评测与对外收口（含原 002 SC）

- **SC-B01**: 维护者能按文档在一小时内跑完正式 Agent 场景集，得到含 outcome / groundedness / trajectory / 多 trial / 成本 / 实验元数据的**非 stub**报告（原 002 SC-001）。
- **SC-B02**: 正式场景集上，成功回答口径的有据可查率 100%；无结果/工具失败题不以编造计成功（原 002 SC-002，在加固证据规则下仍成立）。
- **SC-B03**: ≥5 条关键场景各 ≥3 trial 的汇总成功率写入报告（原 002 SC-003）。
- **SC-B04**: 三轮演示抽检 3 次全过（原 002 SC-004）。
- **SC-B05**: 同批对照题给出 Agent vs Hybrid RAG 的成功与代价对比及固定实验条件（原 002 SC-005）。
- **SC-B06**: 场景集 10～15 条、关键类别齐全、指纹可核对（原 002 SC-008）。
- **SC-B07**: `make eval-agent`、`make demo-agent`（或等价）与文档步骤可独立复现；README 有入口指针。
- **SC-B08**: 完成前不得对外表述「002/003 Agent 评测闭环已完成」。

## Assumptions

- 执行顺序固定：**先 Phase A 代码，再 Phase B 评测**；评测写在本规格内，但不挡代码合并。
- 复用 `001` Hybrid 入口与 `002` golden/graders；`hybrid_prod` 若缺失则在 Phase B 先录制。
- 第一版路由以规则为主；embedding 级联可在 Phase B 用误路由数据再加强。
- 长期偏好与运行审计用现有关系型存储 + 缓存分层，不新增数据库产品。
- Agent 评测不作 PR 强门禁（模型抖动），以可重复本地/手工回归为主。
- Query 改写暂缓，除非检索评测证明收益。

## Out of Scope

| 项 | 说明 |
|----|------|
| dense vs hybrid 对照 | 明确不做 |
| Mem0 / LangGraph / 多 Agent / 换向量库 | 不做 |
| 默认 HyDE / Multi-Query | 暂缓 |
| 用户事实向量记忆 | Phase 2 / 另开规格 |
| 开放式文笔 CI 强门禁 | 不做 |

## Dependencies

- `001-evaluable-hybrid-retrieval`：Hybrid 检索与（Phase B）`hybrid_prod` 基线。
- `002-recommend-agent-memory`：主体实现与 graders/golden；**未完成任务以本规格为准继续**，002 tasks 中对应项视为已迁移。
- `docs/plans/2026-07-25-local-review-agent-hardening.md`：设计细节参考；验收以本规格 SC 为准。

## Interview Value & Knowledge Capture

### Interview talking points（Phase B 文档落地后勾选）

- [x] 记忆为何不做成模型工具
- [x] 有界 tool-loop 与停止条件
- [x] Evidence / Groundedness：先校验再成功返回
- [x] 评测：outcome + groundedness + trajectory + 多 trial；对照 Agent vs Hybrid RAG
- [x] 生产路由：简单 RAG / 复杂 Agent 的成本与成功率权衡
- [x] 与后端主线：登录、限流、分层、可观测

### Planned docs/solutions entries

- 更新/新增 interview：EvidenceLedger、Router、Agent 评测闭环、Agent vs Hybrid RAG
- 002 已有两篇记忆/有界 loop 文保持有效，Phase B 补齐评测与路由篇
