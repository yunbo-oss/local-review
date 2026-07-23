# Contract: Agent Recommend API (SSE)

**Endpoint**: `POST /api/agent/recommend`  
**Auth**: Required (`AuthRequired` middleware)  
**Consumer**: Web/mobile 客户端、demo 脚本、`cmd/eval-agent` harness  
**Date**: 2026-07-23

## Purpose

登录用户多轮店铺推荐：服务端加载 session/profile，执行有界三工具 Agent，SSE 回传状态与**已校验 groundedness** 的最终回答（Spec FR-005 / FR-008）。

## Request

```json
{
  "question": "海淀安静一点的咖啡，推荐两家",
  "session_id": "s-demo-1"
}
```

| 字段 | 类型 | 必填 | 约束 |
|------|------|------|------|
| `question` | string | yes | 非空 |
| `session_id` | string | yes | 非空；同用户下标识会话 |

**Errors (JSON, non-SSE)**:
- `401` 未登录
- `400` 缺字段或空字符串
- `429` 用户级 Agent 限流（不进 LLM）
- `503` Agent 未配置

## Response (SSE)

`Content-Type: text/event-stream`

| Event | Payload | 说明 |
|-------|---------|------|
| `status` | `searching` \| `reading_shop` \| `reading_blogs` | 进度提示；可多次 |
| `message` | string | **完整最终回答**（groundedness 校验通过后）；可单条发送 |
| `done` | JSON string | `{"trace_id":"...","steps":2,"tool_calls":3,"tokens":...}` |
| `error` | string | 公开错误信息（预算耗尽、grounding 失败、infra 等） |

**MUST NOT** 出现在 SSE 中：system prompt、profile 全文、raw tool JSON、chain-of-thought。

## Server-side flow (behavioral)

1. `GetUserInfo` → userID
2. `LoadSession` + `LoadProfile`（profile 失败 Warn，继续）
3. 组装 messages（system 含 profile 摘要 + history + question）
4. `RecommendAgentLogic.Run` bounded loop
5. 最终回答 groundedness 校验 → 通过则 `message` + `done`
6. 成功路径：`AppendSession`；`ExtractProfilePatch` → `MergeProfile`（失败 Warn）

**Cancel**: 客户端断连 → context cancel → 不写 profile（未完成回合）

## Groundedness

回答中店铺引用格式：`[shop:{id}]`（id 为 int64）。所有引用 ∈ 本轮 `observed_shop_ids`。否则 `error` 事件，无 `message` 成功路径。

## Out of scope

- 记忆读写 API 暴露给客户端直接改 profile
- 流式逐 token 且未校验的 message
- 非登录态试用
