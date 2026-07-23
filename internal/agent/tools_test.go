package agent

import (
	"context"
	"testing"
)

func TestToolExecutor_Validation(t *testing.T) {
	t.Parallel()
	exec := &ToolExecutor{Observed: map[int64]struct{}{}, MaxChars: 100}

	t.Run("unknown_field", func(t *testing.T) {
		out, err := exec.Execute(context.Background(), ToolSearchShops, `{"query":"a","foo":1}`)
		if err != nil {
			t.Fatal(err)
		}
		if out == "" || out[0] != '{' {
			t.Fatalf("want error json, got %q", out)
		}
	})

	t.Run("negative_shop_id", func(t *testing.T) {
		out, err := exec.Execute(context.Background(), ToolGetShop, `{"shop_id":-1}`)
		if err != nil {
			t.Fatal(err)
		}
		if out == "" {
			t.Fatal("expected error payload")
		}
	})

	t.Run("oversized_limit", func(t *testing.T) {
		out, err := exec.Execute(context.Background(), ToolListShopBlogs, `{"shop_id":1,"limit":99}`)
		if err != nil {
			t.Fatal(err)
		}
		if out == "" {
			t.Fatal("expected error payload")
		}
	})

	t.Run("empty_query", func(t *testing.T) {
		out, err := exec.Execute(context.Background(), ToolSearchShops, `{"query":""}`)
		if err != nil {
			t.Fatal(err)
		}
		if out == "" {
			t.Fatal("expected error")
		}
	})
}

func TestCanonicalArgs(t *testing.T) {
	t.Parallel()
	a := CanonicalArgs(`{"b":1,"a":2}`)
	b := CanonicalArgs(`{"a":2,"b":1}`)
	if a != b {
		t.Fatalf("%s vs %s", a, b)
	}
}
