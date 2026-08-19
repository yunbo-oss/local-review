package repository

import (
	"context"

	"local-review-go/internal/memory"
	repoInterfaces "local-review-go/internal/repository/interface"

	"github.com/redis/go-redis/v9"
)

// hybridMemoryRepo：会话仍 Redis；偏好走 MySQL AgentProfileRepo
type hybridMemoryRepo struct {
	session *memoryRepo
	profile repoInterfaces.AgentProfileRepo
}

// NewHybridMemoryRepo 生产推荐路径用：profile=MySQL，session=Redis
func NewHybridMemoryRepo(rdb *redis.Client, profile repoInterfaces.AgentProfileRepo) repoInterfaces.MemoryRepo {
	return &hybridMemoryRepo{
		session: &memoryRepo{rdb: rdb},
		profile: profile,
	}
}

func (r *hybridMemoryRepo) LoadProfile(ctx context.Context, userID int64) (memory.Profile, error) {
	return r.profile.LoadProfile(ctx, userID)
}

func (r *hybridMemoryRepo) MergeProfile(ctx context.Context, userID int64, patch memory.ProfilePatch) (memory.Profile, error) {
	return r.profile.MergeProfile(ctx, userID, patch)
}

// MergeProfileForRun 带 run 审计的 merge（供 Recommend 成功路径调用）
func (r *hybridMemoryRepo) MergeProfileForRun(ctx context.Context, userID, runID int64, patch memory.ProfilePatch) (memory.Profile, error) {
	return r.profile.MergeProfileForRun(ctx, userID, runID, patch)
}

func (r *hybridMemoryRepo) ReplaceProfile(ctx context.Context, userID int64, profile memory.Profile) error {
	return r.profile.ReplaceProfile(ctx, userID, profile)
}

func (r *hybridMemoryRepo) LoadSession(ctx context.Context, userID int64, sessionID string, limit int) ([]memory.Message, error) {
	return r.session.LoadSession(ctx, userID, sessionID, limit)
}

func (r *hybridMemoryRepo) AppendSession(ctx context.Context, userID int64, sessionID string, messages ...memory.Message) error {
	return r.session.AppendSession(ctx, userID, sessionID, messages...)
}

func (r *hybridMemoryRepo) LoadSessionSummary(ctx context.Context, userID int64, sessionID string) (memory.SessionSummary, error) {
	return r.session.LoadSessionSummary(ctx, userID, sessionID)
}

func (r *hybridMemoryRepo) SaveSessionSummary(ctx context.Context, userID int64, sessionID string, summary memory.SessionSummary) error {
	return r.session.SaveSessionSummary(ctx, userID, sessionID, summary)
}

func (r *hybridMemoryRepo) TrimSession(ctx context.Context, userID int64, sessionID string, keepRecent int) error {
	return r.session.TrimSession(ctx, userID, sessionID, keepRecent)
}
