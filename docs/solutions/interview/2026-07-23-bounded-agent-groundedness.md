# 有界 Agent：步数/工具预算与 Groundedness

- **日期**: 2026-07-23
- **类型**: interview
- **标签**: [agent, tool-loop, groundedness, SSE, 成本防护]
- **关联代码**: `internal/agent/loop.go`, `internal/agent/tools.go`, `internal/logic/recommend_agent_logic.go`, `internal/handler/agent.go`
- **关联功能/PR**: specs/002-recommend-agent-memory

## 一句话结论（面试口述版）

> 自研 bounded tool-loop：最多 3 步、5 次工具、单工具 10s、整次 45s；同参去重；最终回答先完整生成并校验 `[shop:id]` ⊆ 本轮观测集，再 SSE 发出——避免空转烧钱和幻觉不可撤回。

## 场景或症状

- 背景：相对单次 Hybrid RAG，Agent 要动态「搜 → 详情/评价」。
- 风险：无限循环、重复同参调用、流式边发边幻觉无法撤回。

## 详细说明

### 原理 / 根因

1. **停止条件必须在系统侧**：不能指望模型自律。
2. **Groundedness**：成功口径下引用必须来自工具结果；校验失败走 error，不发成功 message。
3. **分层**：`agent` 包只依赖窄接口 `ShopSearcher`，由 `logic` 适配 `ShopSearchLogic`，避免 import cycle。

### 方案与取舍

| 方案 | 结论 |
|------|------|
| LangGraph / Deep Agents | YAGNI，叙事弱于自研可测闭环 |
| 流式边生成边校验 | 否决：token 已发无法撤回 |
| 纯文本 ReAct 标签 | 解析 fragile；用原生 function calling |

### 落地要点

- 三工具：`search_shops` / `get_shop` / `list_shop_blogs`（只读）
- Dedupe key：`toolName + canonicalJSON(args)`
- SSE：`status` / `message` / `done` / `error`；禁止 CoT 与 raw tool JSON
- 评测：outcome + groundedness + trajectory + 多 trial

## 常见追问

1. **Q**: 为什么只有三个工具？  
   **A**: 搜索发现候选、详情权威字段、评价按需加载；边界清晰才能讲清动态工具选择。

2. **Q**: 和单次 RAG 怎么比？  
   **A**: 对照叙事是 **Agent vs Hybrid RAG**（任务成功与代价），不是 dense vs hybrid。

## 如何避免再犯 / 复习提示

- 先写 fake LLM 的 loop 单测，再接线真实模型
- 简历数字必须带 dataset hash 与 trial 设置

## 参考

- `specs/002-recommend-agent-memory/research.md` §3–8
- Anthropic: Building effective agents / Demystifying evals
