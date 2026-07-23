package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"local-review-go/internal/memory"
	repoInterfaces "local-review-go/internal/repository/interface"
	"local-review-go/pkg/utils/redisx"

	"github.com/redis/go-redis/v9"
)

const (
	sessionTTL  = 7 * 24 * time.Hour
	profileTTL  = 90 * 24 * time.Hour
	sessionCap  = 20
	casMaxRetry = 3
)

type memoryRepo struct {
	rdb *redis.Client
}

// NewMemoryRepo 创建 Redis 记忆仓储
func NewMemoryRepo(rdb *redis.Client) repoInterfaces.MemoryRepo {
	return &memoryRepo{rdb: rdb}
}

func (r *memoryRepo) LoadProfile(ctx context.Context, userID int64) (memory.Profile, error) {
	key := redisx.AgentProfileKey(userID)
	m, err := r.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return memory.Profile{}, err
	}
	if len(m) == 0 {
		return memory.Profile{}, nil
	}
	return hashToProfile(m)
}

func (r *memoryRepo) ReplaceProfile(ctx context.Context, userID int64, profile memory.Profile) error {
	if profile.Version <= 0 {
		profile.Version = 1
	}
	if profile.UpdatedAt == 0 {
		profile.UpdatedAt = memory.NowUnix()
	}
	key := redisx.AgentProfileKey(userID)
	pipe := r.rdb.TxPipeline()
	pipe.HSet(ctx, key, profileToHash(profile))
	pipe.Expire(ctx, key, profileTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *memoryRepo) MergeProfile(ctx context.Context, userID int64, patch memory.ProfilePatch) (memory.Profile, error) {
	key := redisx.AgentProfileKey(userID)
	var last memory.Profile
	for attempt := 0; attempt < casMaxRetry; attempt++ {
		err := r.rdb.Watch(ctx, func(tx *redis.Tx) error {
			m, err := tx.HGetAll(ctx, key).Result()
			if err != nil {
				return err
			}
			old := memory.Profile{}
			if len(m) > 0 {
				old, err = hashToProfile(m)
				if err != nil {
					return err
				}
			}
			merged, err := memory.MergeProfile(old, patch)
			if err != nil {
				return err
			}
			merged.Version = old.Version + 1
			if merged.Version == 0 {
				merged.Version = 1
			}
			pipe := tx.TxPipeline()
			pipe.HSet(ctx, key, profileToHash(merged))
			pipe.Expire(ctx, key, profileTTL)
			_, err = pipe.Exec(ctx)
			if err != nil {
				return err
			}
			last = merged
			return nil
		}, key)
		if err == nil {
			return last, nil
		}
		if err == redis.TxFailedErr {
			continue
		}
		return memory.Profile{}, err
	}
	return memory.Profile{}, fmt.Errorf("MergeProfile CAS conflict after %d retries", casMaxRetry)
}

func (r *memoryRepo) LoadSession(ctx context.Context, userID int64, sessionID string, limit int) ([]memory.Message, error) {
	if limit <= 0 {
		limit = sessionCap
	}
	key := redisx.AgentSessionKey(userID, sessionID)
	// 取最新 limit 条：LRANGE -limit -1
	start := int64(-limit)
	vals, err := r.rdb.LRange(ctx, key, start, -1).Result()
	if err != nil {
		return nil, err
	}
	out := make([]memory.Message, 0, len(vals))
	for _, v := range vals {
		var msg memory.Message
		if err := json.Unmarshal([]byte(v), &msg); err != nil {
			continue
		}
		out = append(out, msg)
	}
	_ = r.rdb.Expire(ctx, key, sessionTTL).Err()
	return out, nil
}

func (r *memoryRepo) AppendSession(ctx context.Context, userID int64, sessionID string, messages ...memory.Message) error {
	if len(messages) == 0 {
		return nil
	}
	key := redisx.AgentSessionKey(userID, sessionID)
	args := make([]any, 0, len(messages))
	for _, msg := range messages {
		if msg.Ts == 0 {
			msg.Ts = memory.NowUnix()
		}
		b, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		args = append(args, string(b))
	}
	pipe := r.rdb.TxPipeline()
	pipe.RPush(ctx, key, args...)
	pipe.LTrim(ctx, key, -int64(sessionCap), -1)
	pipe.Expire(ctx, key, sessionTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func profileToHash(p memory.Profile) map[string]any {
	areas, _ := json.Marshal(p.PreferredAreas)
	types, _ := json.Marshal(p.PreferredTypes)
	dislikes, _ := json.Marshal(p.Dislikes)
	budget := ""
	if p.BudgetMax != nil {
		budget = strconv.FormatInt(*p.BudgetMax, 10)
	}
	return map[string]any{
		"preferred_areas": string(areas),
		"preferred_types": string(types),
		"budget_max":      budget,
		"dislikes":        string(dislikes),
		"summary":         p.Summary,
		"version":         p.Version,
		"updated_at":      p.UpdatedAt,
	}
}

func hashToProfile(m map[string]string) (memory.Profile, error) {
	p := memory.Profile{
		Summary: m["summary"],
	}
	if v := m["version"]; v != "" {
		n, _ := strconv.ParseInt(v, 10, 64)
		p.Version = n
	}
	if v := m["updated_at"]; v != "" {
		n, _ := strconv.ParseInt(v, 10, 64)
		p.UpdatedAt = n
	}
	if v := strings.TrimSpace(m["budget_max"]); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			p.BudgetMax = &n
		}
	}
	_ = json.Unmarshal([]byte(orEmptyArray(m["preferred_areas"])), &p.PreferredAreas)
	_ = json.Unmarshal([]byte(orEmptyArray(m["preferred_types"])), &p.PreferredTypes)
	_ = json.Unmarshal([]byte(orEmptyArray(m["dislikes"])), &p.Dislikes)
	if p.PreferredAreas == nil {
		p.PreferredAreas = []string{}
	}
	if p.PreferredTypes == nil {
		p.PreferredTypes = []string{}
	}
	if p.Dislikes == nil {
		p.Dislikes = []string{}
	}
	return p, nil
}

func orEmptyArray(s string) string {
	if strings.TrimSpace(s) == "" {
		return "[]"
	}
	return s
}
