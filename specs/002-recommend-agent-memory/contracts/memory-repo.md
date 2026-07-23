# Contract: MemoryRepo

**Package**: `internal/repository/interface`  
**Implementation**: `internal/repository/memory_repo.go`  
**Consumers**: `RecommendAgentLogic`, `RAGLogic`/`ShopSearchLogic`（profile 补空）, `cmd/eval-agent` setup  
**Date**: 2026-07-23

## Purpose

持久化短期会话与长期结构化偏好；支持可纠正补丁合并与并发安全（Spec FR-001–004）。

## Go interface (target)

```go
type MemoryRepo interface {
    LoadProfile(ctx context.Context, userID int64) (memory.Profile, error)
    MergeProfile(ctx context.Context, userID int64, patch memory.ProfilePatch) (memory.Profile, error)
    LoadSession(ctx context.Context, userID int64, sessionID string, limit int) ([]memory.Message, error)
    AppendSession(ctx context.Context, userID int64, sessionID string, messages ...memory.Message) error
}
```

## Behavioral contract

| Method | 行为 |
|--------|------|
| `LoadProfile` | 无记录 → 空 Profile（非 error） |
| `MergeProfile` | `WATCH version` → 读 → `MergeProfile` 纯函数 → HSET → version+1；冲突重试 ≤3 |
| `LoadSession` | 返回最近 `limit` 条（时间正序）；无记录 → nil slice |
| `AppendSession` | pipeline: RPUSH → LTRIM(保留最近20) → EXPIRE 7d |

## Keys

MUST use `redisx.AgentSessionKey(userID, sessionID)` and `redisx.AgentProfileKey(userID)`.

## Profile patch semantics

见 [data-model.md](../data-model.md) §3。Repository **不**调用 LLM；patch 由 logic 层 `ExtractProfilePatch` 提供。

## Failure policy

- Redis 不可用 → error 向上；LoadProfile 失败时 logic 层 Warn 并仅用本轮条件（FR-004）
- Merge 失败 → 不覆盖；主回答已成功返回时不 rollback 回答（FR-009）

## Out of scope

- 向量记忆 / Mem0
- 模型直接读写
- 跨用户 profile 访问
