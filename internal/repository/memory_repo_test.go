package repository

import (
	"context"
	"testing"

	"local-review-go/internal/memory"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestMemoryRepo(t *testing.T) (*memoryRepo, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return &memoryRepo{rdb: rdb}, mr
}

func TestMemoryRepo_SessionAndProfile(t *testing.T) {
	repo, _ := newTestMemoryRepo(t)
	ctx := context.Background()
	memory.NowUnix = func() int64 { return 1700000000 }

	t.Run("load_empty_profile", func(t *testing.T) {
		p, err := repo.LoadProfile(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if p.Version != 0 {
			t.Fatalf("want empty, got %+v", p)
		}
	})

	t.Run("merge_and_cas", func(t *testing.T) {
		budget := int64(100)
		got, err := repo.MergeProfile(ctx, 1, memory.ProfilePatch{
			PreferredAreasAdd: []string{"海淀区"},
			BudgetMax:         &budget,
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.Version != 1 || len(got.PreferredAreas) != 1 || got.BudgetMax == nil {
			t.Fatalf("%+v", got)
		}
		got2, err := repo.MergeProfile(ctx, 1, memory.ProfilePatch{
			PreferredAreasAdd: []string{"朝阳区"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got2.Version != 2 || len(got2.PreferredAreas) != 2 {
			t.Fatalf("%+v", got2)
		}
		z := int64(0)
		got3, err := repo.MergeProfile(ctx, 1, memory.ProfilePatch{BudgetMax: &z})
		if err != nil {
			t.Fatal(err)
		}
		if got3.BudgetMax != nil {
			t.Fatalf("budget should clear: %+v", got3)
		}
	})

	t.Run("session_rpush_ltrim", func(t *testing.T) {
		msgs := make([]memory.Message, 0, 25)
		for i := 0; i < 25; i++ {
			msgs = append(msgs, memory.Message{Role: "user", Content: "m" + string(rune('a'+i%26))})
		}
		if err := repo.AppendSession(ctx, 1, "s1", msgs...); err != nil {
			t.Fatal(err)
		}
		loaded, err := repo.LoadSession(ctx, 1, "s1", 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(loaded) != 20 {
			t.Fatalf("want 20, got %d", len(loaded))
		}
	})
}
