# Contract: Agent Persistence (MySQL + Redis Cache)

**Phase**: A | **Packages**: `AgentRunRepo`, `AgentProfileRepo`（名称以实现为准）

## Profile

| 操作 | 语义 |
|------|------|
| Load | Cache Aside：Redis 命中则用；否则 MySQL → 回填缓存；均空则 zero profile |
| Merge | MySQL `version` 乐观锁；冲突重试 ≤3；成功写 event；再失效/刷新缓存 |
| 迁移 | MySQL 空时可读遗留 Redis Hash，校验后写入 MySQL |

失败：Load 失败 → Warn + 空 profile 继续；Merge 失败 → Warn，不推翻已通过校验的主回答。

## Run / ToolCall

| 操作 | 语义 |
|------|------|
| Begin | 插入 `agent_runs` status=RUNNING，分配 `trace_id` |
| AppendTool（可选批量） | 运行结束事务写入 attempts 摘要 |
| Finalize | status=COMPLETED/FAILED/CANCELLED；写 evidence_summary、route、tokens、latency |

**时序**：校验通过 → MySQL 事务（tools + session 可选 + profile merge + run 终态）→ 提交后刷 Redis → 再 SSE message/done。

断连/取消：Finalize CANCELLED；**不** MergeProfile。

## Privacy

禁止持久化：完整用户原文、完整评价正文、隐藏思维链、凭据。
