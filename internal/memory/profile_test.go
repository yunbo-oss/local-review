package memory

import "testing"

func TestMergeProfile_AddRemoveAndBudget(t *testing.T) {
	t.Parallel()
	NowUnix = func() int64 { return 1000 }

	budget100 := int64(100)
	old := Profile{
		PreferredAreas: []string{"朝阳区"},
		PreferredTypes: []string{"美食"},
		BudgetMax:      &budget100,
		Dislikes:       []string{"太辣"},
	}

	t.Run("add_area_keep_old", func(t *testing.T) {
		got, err := MergeProfile(old, ProfilePatch{PreferredAreasAdd: []string{"海淀区"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.PreferredAreas) != 2 {
			t.Fatalf("areas=%v", got.PreferredAreas)
		}
		seen := map[string]bool{}
		for _, a := range got.PreferredAreas {
			seen[a] = true
		}
		if !seen["海淀区"] || !seen["朝阳区"] {
			t.Fatalf("areas=%v", got.PreferredAreas)
		}
	})

	t.Run("remove_dislike", func(t *testing.T) {
		got, err := MergeProfile(old, ProfilePatch{DislikesRemove: []string{"太辣"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Dislikes) != 0 {
			t.Fatalf("dislikes=%v", got.Dislikes)
		}
	})

	t.Run("add_remove_conflict_remove_wins", func(t *testing.T) {
		got, err := MergeProfile(old, ProfilePatch{
			PreferredAreasAdd:    []string{"海淀区"},
			PreferredAreasRemove: []string{"海淀区"},
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range got.PreferredAreas {
			if a == "海淀区" {
				t.Fatal("remove should win")
			}
		}
	})

	t.Run("budget_nil_keeps", func(t *testing.T) {
		got, err := MergeProfile(old, ProfilePatch{})
		if err != nil {
			t.Fatal(err)
		}
		if got.BudgetMax == nil || *got.BudgetMax != 100 {
			t.Fatalf("budget=%v", got.BudgetMax)
		}
	})

	t.Run("budget_zero_clears", func(t *testing.T) {
		z := int64(0)
		got, err := MergeProfile(old, ProfilePatch{BudgetMax: &z})
		if err != nil {
			t.Fatal(err)
		}
		if got.BudgetMax != nil {
			t.Fatalf("want cleared, got %v", *got.BudgetMax)
		}
	})

	t.Run("budget_positive_overrides", func(t *testing.T) {
		v := int64(200)
		got, err := MergeProfile(old, ProfilePatch{BudgetMax: &v})
		if err != nil {
			t.Fatal(err)
		}
		if got.BudgetMax == nil || *got.BudgetMax != 200 {
			t.Fatalf("budget=%v", got.BudgetMax)
		}
	})

	t.Run("summary_truncate", func(t *testing.T) {
		long := stringsRepeat("啊", 100)
		got, err := MergeProfile(old, ProfilePatch{Summary: &long})
		if err != nil {
			t.Fatal(err)
		}
		if len([]rune(got.Summary)) != MaxSummaryRunes {
			t.Fatalf("summary runes=%d", len([]rune(got.Summary)))
		}
	})
}

func stringsRepeat(s string, n int) string {
	out := make([]rune, 0, n)
	r := []rune(s)[0]
	for i := 0; i < n; i++ {
		out = append(out, r)
	}
	return string(out)
}
