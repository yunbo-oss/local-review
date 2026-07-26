package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"local-review-go/internal/memory"
	"local-review-go/internal/model"
	repoInterfaces "local-review-go/internal/repository/interface"
	"local-review-go/pkg/utils/redisx"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const profileCacheTTL = 7 * 24 * time.Hour

type agentProfileRepo struct {
	db  *gorm.DB
	rdb *redis.Client
}

// NewAgentProfileRepo MySQL 事实源 + Redis 缓存 + 遗留 Hash 回填
func NewAgentProfileRepo(db *gorm.DB, rdb *redis.Client) repoInterfaces.AgentProfileRepo {
	return &agentProfileRepo{db: db, rdb: rdb}
}

func (r *agentProfileRepo) LoadProfile(ctx context.Context, userID int64) (memory.Profile, error) {
	if p, ok := r.loadCache(ctx, userID); ok {
		return p, nil
	}
	row, err := r.loadMySQL(ctx, userID)
	if err != nil {
		return memory.Profile{}, err
	}
	if row != nil {
		p, err := decodeProfileJSON(row.ProfileJSON, row.Version)
		if err != nil {
			return memory.Profile{}, err
		}
		_ = r.writeCache(ctx, userID, p)
		return p, nil
	}
	// 遗留 Redis Hash → 校验后写入 MySQL
	legacy, err := r.loadLegacyHash(ctx, userID)
	if err != nil {
		return memory.Profile{}, err
	}
	if legacy.Version == 0 && len(legacy.PreferredAreas) == 0 && len(legacy.PreferredTypes) == 0 &&
		legacy.BudgetMax == nil && len(legacy.Dislikes) == 0 && legacy.Summary == "" {
		return memory.Profile{}, nil
	}
	if legacy.Version <= 0 {
		legacy.Version = 1
	}
	if err := r.persistMySQL(ctx, userID, legacy); err != nil {
		return memory.Profile{}, err
	}
	_ = r.writeCache(ctx, userID, legacy)
	return legacy, nil
}

func (r *agentProfileRepo) MergeProfile(ctx context.Context, userID int64, patch memory.ProfilePatch) (memory.Profile, error) {
	return r.MergeProfileForRun(ctx, userID, 0, patch)
}

func (r *agentProfileRepo) MergeProfileForRun(ctx context.Context, userID, runID int64, patch memory.ProfilePatch) (memory.Profile, error) {
	var last memory.Profile
	for attempt := 0; attempt < casMaxRetry; attempt++ {
		patchRaw, _ := json.Marshal(patch)
		err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			old := memory.Profile{}
			var row model.UserAgentProfile
			qerr := tx.Where("user_id = ?", userID).First(&row).Error
			if errors.Is(qerr, gorm.ErrRecordNotFound) {
				// 事务外遗留回填可能尚未发生：尝试一次 legacy（只读）
				if leg, lerr := r.loadLegacyHash(ctx, userID); lerr == nil {
					old = leg
				}
			} else if qerr != nil {
				return qerr
			} else {
				p, derr := decodeProfileJSON(row.ProfileJSON, row.Version)
				if derr != nil {
					return derr
				}
				old = p
			}
			merged, merr := memory.MergeProfile(old, patch)
			if merr != nil {
				return merr
			}
			newVer := old.Version + 1
			if newVer <= 0 {
				newVer = 1
			}
			merged.Version = newVer
			if merged.UpdatedAt == 0 {
				merged.UpdatedAt = memory.NowUnix()
			}
			raw, jerr := json.Marshal(merged)
			if jerr != nil {
				return jerr
			}
			if errors.Is(qerr, gorm.ErrRecordNotFound) {
				if cerr := tx.Create(&model.UserAgentProfile{
					UserID: userID, ProfileJSON: string(raw), Version: newVer,
				}).Error; cerr != nil {
					return cerr
				}
			} else {
				res := tx.Model(&model.UserAgentProfile{}).
					Where("user_id = ? AND version = ?", userID, old.Version).
					Updates(map[string]any{
						"profile_json": string(raw),
						"version":      newVer,
					})
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected != 1 {
					return fmt.Errorf("profile version conflict")
				}
			}
			if err := tx.Create(&model.UserAgentProfileEvent{
				UserID: userID, RunID: runID, PatchJSON: string(patchRaw),
				OldVersion: old.Version, NewVersion: newVer,
			}).Error; err != nil {
				return err
			}
			last = merged
			return nil
		})
		if err == nil {
			_ = r.invalidateCache(ctx, userID)
			_ = r.writeCache(ctx, userID, last)
			return last, nil
		}
		if err.Error() == "profile version conflict" {
			_ = r.invalidateCache(ctx, userID)
			continue
		}
		return memory.Profile{}, err
	}
	return memory.Profile{}, fmt.Errorf("merge profile cas exceeded retries")
}

func (r *agentProfileRepo) ReplaceProfile(ctx context.Context, userID int64, profile memory.Profile) error {
	if profile.Version <= 0 {
		profile.Version = 1
	}
	if profile.UpdatedAt == 0 {
		profile.UpdatedAt = memory.NowUnix()
	}
	if err := r.persistMySQL(ctx, userID, profile); err != nil {
		return err
	}
	_ = r.invalidateCache(ctx, userID)
	return r.writeCache(ctx, userID, profile)
}

func (r *agentProfileRepo) persistMySQL(ctx context.Context, userID int64, p memory.Profile) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	row := model.UserAgentProfile{
		UserID: userID, ProfileJSON: string(raw), Version: p.Version,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"profile_json", "version", "updated_at"}),
	}).Create(&row).Error
}

func (r *agentProfileRepo) loadMySQL(ctx context.Context, userID int64) (*model.UserAgentProfile, error) {
	var row model.UserAgentProfile
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *agentProfileRepo) loadLegacyHash(ctx context.Context, userID int64) (memory.Profile, error) {
	if r.rdb == nil {
		return memory.Profile{}, nil
	}
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

func (r *agentProfileRepo) loadCache(ctx context.Context, userID int64) (memory.Profile, bool) {
	if r.rdb == nil {
		return memory.Profile{}, false
	}
	raw, err := r.rdb.Get(ctx, redisx.AgentProfileCacheKey(userID)).Result()
	if err != nil || raw == "" {
		return memory.Profile{}, false
	}
	var p memory.Profile
	if json.Unmarshal([]byte(raw), &p) != nil {
		return memory.Profile{}, false
	}
	return p, true
}

func (r *agentProfileRepo) writeCache(ctx context.Context, userID int64, p memory.Profile) error {
	if r.rdb == nil {
		return nil
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return r.rdb.Set(ctx, redisx.AgentProfileCacheKey(userID), string(raw), profileCacheTTL).Err()
}

func (r *agentProfileRepo) invalidateCache(ctx context.Context, userID int64) error {
	if r.rdb == nil {
		return nil
	}
	return r.rdb.Del(ctx, redisx.AgentProfileCacheKey(userID)).Err()
}

func decodeProfileJSON(s string, version int64) (memory.Profile, error) {
	var p memory.Profile
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return memory.Profile{}, err
	}
	if p.Version == 0 {
		p.Version = version
	}
	return p, nil
}
