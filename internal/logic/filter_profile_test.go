package logic

import (
	"testing"

	"local-review-go/internal/memory"
	repoInterfaces "local-review-go/internal/repository/interface"
)

func TestMergeFilterWithProfile(t *testing.T) {
	t.Parallel()
	budget := int64(100)
	p := memory.Profile{
		PreferredAreas: []string{"海淀区", "朝阳区"},
		PreferredTypes: []string{"咖啡"},
		BudgetMax:      &budget,
	}

	t.Run("fill_empty", func(t *testing.T) {
		got := MergeFilterWithProfile(nil, p)
		if got == nil || got.Area != "海淀区" || got.TypeName != "咖啡" || got.MaxPrice != 100 {
			t.Fatalf("got=%+v", got)
		}
	})

	t.Run("explicit_not_overwritten", func(t *testing.T) {
		explicit := &repoInterfaces.VectorSearchFilter{Area: "西城区", MaxPrice: 50}
		got := MergeFilterWithProfile(explicit, p)
		if got.Area != "西城区" || got.MaxPrice != 50 {
			t.Fatalf("got=%+v", got)
		}
		if got.TypeName != "咖啡" {
			t.Fatalf("type fill failed: %+v", got)
		}
	})
}
