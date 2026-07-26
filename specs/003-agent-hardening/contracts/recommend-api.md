# Contract: Recommend HTTP / SSE（003 扩展）

**Phase**: A | **Base**: `002` `POST /api/agent/recommend`（JWT）

## Endpoints

| Method | Path | 说明 |
|--------|------|------|
| POST | `/api/agent/recommend` | Agent 路径；可先经 Router（若 force/agent_*） |
| POST | `/api/rag/chat` | 保留；Router 的 `rag_oneshot` 可复用其 Logic |
| POST | `/api/recommend` | **可选**统一入口：内部调用 Router 再分流 |

## Request（Agent / 统一入口）

```json
{
  "question": "string",
  "session_id": "string",
  "force_route": "rag_oneshot|agent_multistep|agent_memory"
}
```

`force_route` 可选；非法值 → 400。

## SSE Events

| Event | 载荷 |
|-------|------|
| `status` | `searching` / `reading_shop` / `reading_blogs`（仅 Agent 路径） |
| `message` | 完整终答（**仅**校验通过后） |
| `done` | JSON：`steps`, `tool_calls`, `tokens`, **`trace_id`**, **`route`**, `route_reason`, `degraded`? |
| `error` | 公共错误文案（无 stack / 无 tool raw） |

## 禁止出现在 SSE

- 隐藏思维链 / CoT  
- 原始 tool JSON  
- 完整 profile 转储  
- 完整系统 prompt  

## Auth / Limit

- 未登录 → 401  
- 缺 question/session_id → 400  
- 超限 → 429（Redis 共享配额，进 LLM 前）
