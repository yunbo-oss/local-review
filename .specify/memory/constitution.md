<!--
Sync Impact Report
- Version change: 1.1.0 → 1.1.1
- Modified principles: none (governance / doc-boundary wording only)
- Added sections: none
- Removed sections: none
- Doc system: memory-bank slimmed to activeContext.md only; duplicates live in AGENTS.md + this constitution
- Templates / rules:
  - `.cursorrules` ✅
  - `docs/solutions/README.md` ✅
  - `memory-bank/activeContext.md` ✅
- Follow-up TODOs: none
-->

# local-review-go Constitution

## Core Principles

### I. Interview-First Delivery

本项目的首要目标是**秋招后端面试准备**：功能交付必须同时产出可讲述的技术故事
（问题背景 → 方案选型 → 实现取舍 → 踩坑与验证）。

每完成一块有面试价值的能力（缓存、MQ、分布式、RAG、限流等），MUST 能用
2～5 分钟讲清「为什么这样设计」与「和大厂常见问法如何对应」。纯堆功能、
无法复述设计动机的改动视为不合规。

**Rationale**: 作品集价值 = 可演示的系统 + 可背诵的设计叙事；二者缺一不可。

### II. Layered Architecture (NON-NEGOTIABLE)

调用链 MUST 为：`Handler → Logic → Repository（接口）→ Repository（实现）→ DB`。

- Handler：参数解析、校验、调用 logic、返回 `httpx.Result[T]`；禁止业务逻辑与直连 DB/Redis
- Logic：业务规则；依赖 Repo 接口与显式注入的基础设施；禁止直接操作 DB
- Repository：全部数据访问；接口在 `internal/repository/interface/`
- Model：仅实体与 `TableName()`；禁止 DB 操作

Redis key MUST 集中在 `pkg/utils/redisx/`；新增领域按
Model → Repo 接口 → Repo 实现 → Logic → Handler → `cmd/server` 注入顺序落地。

**Rationale**: 分层是面试高频考点，也是本仓库 AGENTS.md 的硬约束。

### III. Plan → Build → Test

执行任务 MUST 遵循：先规划（方案与边界）→ 再实现 → 再验证（单测 / 接口冒烟 /
相关 Makefile 目标）。声称「完成 / 修复 / 通过」前 MUST 有可复查的命令与输出证据。

**Rationale**: 防止未验证交付；与 verification-before-completion 一致。

### IV. Explain-as-You-Build

维护者处于学习阶段。每次实质性代码改动，协作输出 MUST 包含：

1. 改了什么（文件与改动点）
2. 为什么这样改（背景 / 设计）
3. 代码逻辑（调用关系与数据流，面向第一次阅读的人）
4. 如何理解（易混概念：接口与实现、DI、缓存一致性、事务消息等）

被问「某模块如何实现」时，回答 SHOULD 含流程调用图、关键函数表、以及
2～5 个面试可能问点与要点答案（见 AGENTS.md §5.8 / §5.9）。

**Rationale**: 把日常开发变成面试口述训练，而不是只留下黑盒 diff。

### V. Simplicity Over Spectacle

优先选择与本项目阶段匹配、业界验证过的方案；禁止为简历关键词引入
无法讲清、无法运维、无法评测的过度设计（例如未证明收益前引入多 Agent 编排框架、
第二套向量库等）。复杂度引入 MUST 在 plan 的 Complexity Tracking 中写明
「为何更简单方案不够」。

**Rationale**: 面试官更看重取舍能力；堆技术名词而无证据是负分。

### VI. Knowledge Capture to docs/solutions (NON-NEGOTIABLE)

下列内容 MUST 写入 `docs/solutions/`（不得只留在聊天记录里）：

1. **面试有价值的知识**：原理、对比、选型理由、常见追问与标准答法
2. **开发卡点**：现象 → 根因 → 排查步骤 → 最终修复 → 如何避免再犯

**触发场景（任一即适用）**：

- 功能实现 / bug 修复收尾（含 `/speckit-implement`）
- **与维护者在对话中讨论知识点、对比方案、梳理面试答法时**：Agent MUST
  判断本次结论是否可复用；若可复用，SHOULD 当场或本回合结束前新增/更新
  `docs/solutions/` 条目（新建用 `TEMPLATE.md`，已有则补「追问 / 结论 / 关联代码」），
  并更新 `docs/solutions/README.md` 索引。纯澄清且无新结论时可跳过，但 MUST
  在回复中明示「未落盘及原因」。

条目 MUST 使用 `docs/solutions/TEMPLATE.md`；面试类放 `docs/solutions/interview/`，
卡点类放 `docs/solutions/blockers/`；并在 `docs/solutions/README.md` 索引中登记。

完成用户故事或关闭重要 bug 后、合并前，SHOULD 至少新增或更新一条相关记录。
触及上述两类内容却只留在聊天、未落盘，视为未完成。

**Rationale**: 秋招复习依赖可检索的个人知识库；聊天不可复用。讨论本身也是
最高频的知识生产时刻，必须进仓库。

## Interview Knowledge Capture

知识库根目录：`docs/solutions/`。

| 类型 | 目录 | 何时写 |
|------|------|--------|
| 面试知识 | `interview/` | 学到可迁移要点时；**或对话中形成可背诵结论时** |
| 开发卡点 | `blockers/` | 卡点已解决时；**或对话中定位根因并确认修复路径时** |

每条记录最少包含：标题、场景/症状、结论（可面试口述）、细节、关联代码路径、
标签。禁止只贴日志无无结论；禁止把机密（真实密钥、生产地址）写入仓库。

与 Spec Kit 的关系：`/speckit-plan` 的 Constitution Check MUST 确认是否规划了
知识落盘任务；`/speckit-tasks` MUST 为有面试价值的故事加入 Knowledge Capture 任务；
`/speckit-implement` 收尾时 MUST 核对 `docs/solutions` 是否已更新。

**文档边界**：`docs/solutions/` 只承载「可复习的知识与卡点」，**不**承载项目进度看板、
产品简介或与 `AGENTS.md` 重复的工程规范。

| 文档 | 职责 |
|------|------|
| `AGENTS.md` | 工程规范、分层、启动、业务约定 |
| `.specify/memory/constitution.md` | 秋招导向原则与治理 |
| `memory-bank/activeContext.md` | **仅**当前进度 / 近期决策（memory-bank 内唯一文件） |
| `docs/solutions/` | 面试知识与开发卡点 |

## Development Workflow

1. **Constitution**：原则变更走本文件修订与版本号（见 Governance）
2. **Specify → Plan → Tasks → Implement**：功能开发主路径
3. **协作规范**：遵循 `AGENTS.md`、`.cursorrules`、本宪章
4. **技术栈默认**：Go 1.24+、Gin、GORM、MySQL、Redis、JWT、RocketMQ（秒杀异步）、
   可观测性优先 OpenTelemetry；偏离 MUST 在 plan 中说明
5. **重大进度**：更新 `memory-bank/activeContext.md`
6. **知识点讨论**：按原则 VI 视情况更新 `docs/solutions/`

## Governance

本宪章优先于临时口头约定与未经记录的「习惯做法」。与 `AGENTS.md` 冲突时，
以更严格、可检查的那一条为准，并应尽快修订两者之一以消除歧义。

**修订程序**：

1. 提出变更（原则增删改或治理规则调整）
2. 更新本文件，按语义化版本递增：
   - MAJOR：删除/重定义不可兼容的原则
   - MINOR：新增原则或实质性扩展约束
   - PATCH：措辞澄清、笔误、非语义细化
3. 同步检查 `.specify/templates/` 中 Constitution Check / 任务类型是否仍对齐
4. 更新 **Last Amended** 日期；**Ratified** 仅在首次采纳或重新批准时改动

**合规审查**：Plan 阶段跑 Constitution Check；Implement 收尾与知识点讨论收尾核对
分层、验证证据、以及 `docs/solutions` 落盘。复杂度违规必须填 Complexity Tracking，
否则不得合并视为完成。

运行时开发指引：`AGENTS.md`、`.specify/memory/constitution.md`、
`memory-bank/activeContext.md`、`docs/solutions/README.md`。

**Version**: 1.1.1 | **Ratified**: 2026-07-12 | **Last Amended**: 2026-07-12
