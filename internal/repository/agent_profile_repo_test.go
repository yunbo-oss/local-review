package repository

import (
	"context"
	"testing"

	"local-review-go/internal/memory"
	"local-review-go/internal/model"
	"local-review-go/pkg/utils/redisx"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func newTestProfileRepo(t *testing.T) (*agentProfileRepo, *miniredis.Miniredis, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserAgentProfile{}, &model.UserAgentProfileEvent{}); err != nil {
		t.Fatal(err)
	}
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return &agentProfileRepo{db: db, rdb: rdb}, mr, db
}

func TestAgentProfileRepo_CacheAsideAndCAS(t *testing.T) {
	repo, mr, _ := newTestProfileRepo(t)
	ctx := context.Background()
	memory.NowUnix = func() int64 { return 1700000000 }

	t.Run("empty", func(t *testing.T) {
		p, err := repo.LoadProfile(ctx, 9)
		if err != nil || p.Version != 0 {
			t.Fatalf("%+v %v", p, err)
		}
	})

	t.Run("merge_cas_and_event", func(t *testing.T) {
		budget := int64(80)
		p, err := repo.MergeProfileForRun(ctx, 9, 100, memory.ProfilePatch{
			PreferredAreasAdd: []string{"海淀区"},
			BudgetMax:         &budget,
		})
		if err != nil || p.Version != 1 {
			t.Fatalf("%+v %v", p, err)
		}
		// 清缓存后仍能从 MySQL 恢复
		mr.Del(redisx.AgentProfileCacheKey(9))
		p2, err := repo.LoadProfile(ctx, 9)
		if err != nil || p2.Version != 1 || len(p2.PreferredAreas) != 1 {
			t.Fatalf("cache aside fail: %+v %v", p2, err)
		}
		p3, err := repo.MergeProfile(ctx, 9, memory.ProfilePatch{PreferredAreasAdd: []string{"朝阳区"}})
		if err != nil || p3.Version != 2 || len(p3.PreferredAreas) != 2 {
			t.Fatalf("%+v %v", p3, err)
		}
	})

	t.Run("legacy_redis_backfill", func(t *testing.T) {
		legacy := &memoryRepo{rdb: redis.NewClient(&redis.Options{Addr: mr.Addr()})}
		if err := legacy.ReplaceProfile(ctx, 42, memory.Profile{
			PreferredAreas: []string{"西城区"}, Version: 3, UpdatedAt: 1,
		}); err != nil {
			t.Fatal(err)
		}
		p, err := repo.LoadProfile(ctx, 42)
		if err != nil || p.Version != 3 || p.PreferredAreas[0] != "西城区" {
			t.Fatalf("backfill: %+v %v", p, err)
		}
		// 再次 Load 走 MySQL/cache，不依赖遗留 Hash
		mr.Del(redisx.AgentProfileKey(42))
		mr.Del(redisx.AgentProfileCacheKey(42))
		p2, err := repo.LoadProfile(ctx, 42)
		if err != nil || p2.Version != 3 {
			t.Fatalf("mysql persist: %+v %v", p2, err)
		}
	})
}
