package interfaces

import (
	"context"

	"local-review-go/internal/memory"
)

// AgentProfileRepo PostgreSQL 事实源 + Redis Cache Aside 的长期偏好
type AgentProfileRepo interface {
	LoadProfile(ctx context.Context, userID int64) (memory.Profile, error)
	MergeProfile(ctx context.Context, userID int64, patch memory.ProfilePatch) (memory.Profile, error)
	// MergeProfileForRun 同 Merge，并写入 profile event（runID 可为 0）
	MergeProfileForRun(ctx context.Context, userID, runID int64, patch memory.ProfilePatch) (memory.Profile, error)
	ReplaceProfile(ctx context.Context, userID int64, profile memory.Profile) error
}
