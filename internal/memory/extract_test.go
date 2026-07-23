package memory

import "testing"

func TestParseProfilePatchJSON(t *testing.T) {
	t.Parallel()

	t.Run("ok", func(t *testing.T) {
		raw := `{"preferred_areas_add":["海淀区"],"budget_max":100,"summary":"喜欢安静"}`
		p, err := ParseProfilePatchJSON(raw)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.PreferredAreasAdd) != 1 || p.BudgetMax == nil || *p.BudgetMax != 100 {
			t.Fatalf("%+v", p)
		}
	})

	t.Run("unknown_field_reject", func(t *testing.T) {
		_, err := ParseProfilePatchJSON(`{"preferred_areas_add":["海淀区"],"foo":1}`)
		if err == nil {
			t.Fatal("expected unknown field error")
		}
	})

	t.Run("negative_budget_reject", func(t *testing.T) {
		_, err := ParseProfilePatchJSON(`{"budget_max":-1}`)
		if err == nil {
			t.Fatal("expected budget error")
		}
	})

	t.Run("fence_stripped", func(t *testing.T) {
		raw := "```json\n{\"dislikes_remove\":[\"太辣\"]}\n```"
		p, err := ParseProfilePatchJSON(raw)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.DislikesRemove) != 1 {
			t.Fatalf("%+v", p)
		}
	})
}
