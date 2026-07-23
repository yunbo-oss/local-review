package interfaces

import (
	"context"

	"local-review-go/internal/memory"
)

// MemoryRepo 会话与长期偏好持久化
type MemoryRepo interface {
	LoadProfile(ctx context.Context, userID int64) (memory.Profile, error)
	MergeProfile(ctx context.Context, userID int64, patch memory.ProfilePatch) (memory.Profile, error)
	LoadSession(ctx context.Context, userID int64, sessionID string, limit int) ([]memory.Message, error)
	AppendSession(ctx context.Context, userID int64, sessionID string, messages ...memory.Message) error
	// ReplaceProfile 评测/demo 用：直接写入完整 profile 快照（覆盖）
	ReplaceProfile(ctx context.Context, userID int64, profile memory.Profile) error
}
