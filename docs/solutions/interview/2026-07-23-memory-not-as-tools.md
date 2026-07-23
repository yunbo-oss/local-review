# 记忆为何不暴露为模型工具

- **日期**: 2026-07-23
- **类型**: interview
- **标签**: [agent, memory, tool-calling, 安全, 可纠正偏好]
- **关联代码**: `internal/memory/`, `internal/repository/memory_repo.go`, `internal/logic/recommend_agent_logic.go`
- **关联功能/PR**: specs/002-recommend-agent-memory

## 一句话结论（面试口述版）

> 长期偏好和会话是**应用状态**，不是领域知识工具。服务端加载/注入/回合末合并，模型只调用搜店/详情/评价三类只读工具——这样可校验、可限权、可纠正，也避免把幻觉写回记忆。

## 场景或症状

- 背景：推荐 Agent 需要多轮记住区域/预算/忌口，同时支持「忘掉预算」等纠正。
- 常见错误设计：给模型 `read_memory` / `update_memory` 工具，让模型自己决定何时读写 Redis。

## 详细说明

### 原理 / 根因

1. **工具该干什么**：工具应对齐「可验证的领域动作」（搜店、读详情），结果可观测、可评测 groundedness。
2. **记忆是什么**：profile/session 是账号侧状态机（补丁合并、CAS、TTL），语义是产品规则，不是模型自由发挥的知识库。
3. **污染路径**：若用 assistant 输出更新 profile，模型幻觉会变成「永久偏好」。

### 方案与取舍

| 方案 | 优点 | 缺点 | 结论 |
|------|------|------|------|
| 模型 memory 工具 | 灵活 | 调用噪声、难测、难限权、易污染 | 否决 |
| Mem0 等外部记忆框架 | 简历词好看 | 依赖叙事弱、难讲清与业务规则关系 | YAGNI |
| 服务端注入 + 回合末 ExtractPatch | 可测 merge/CAS、纠正语义清晰 | 需自己写抽取与合并 | **采用** |

优先级：**本轮显式条件 > LLM 抽取 > profile 仅补空**。

### 落地要点

- Redis：`agent:sess:{uid}:{sid}` List（7d）、`agent:profile:{uid}` Hash（90d + version CAS）
- `MergeProfile`：`*_remove` 胜于同值 `*_add`；`budget_max` 三态（nil/0/正数）
- Extract 只吃**用户原话**；解析失败 Warn，不覆盖旧 profile
- 断连未完成回合：**不写** profile

## 常见追问（建议至少 2 条）

1. **Q**: 为什么不用向量记忆存「用户喜欢安静」这类软事实？  
   **A**: Phase 1 硬偏好用 Hash 足够；向量记忆等真实失败案例证明结构化不够再上（YAGNI）。

2. **Q**: profile 并发怎么保证？  
   **A**: `WATCH version` + 事务重试（≤3）；冲突不静默丢更新。

## 如何避免再犯 / 复习提示

- 画一张图：模型工具边界 vs 应用状态边界
- 把「可纠正」当成需求，而不是事后补丁

## 参考

- 内部：`specs/002-recommend-agent-memory/research.md` §1–2；`docs/plans/2026-07-11-recommend-agent-eval.md` §1.4
