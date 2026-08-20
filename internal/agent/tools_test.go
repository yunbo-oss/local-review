package agent

import (
	"context"
	"strings"
	"testing"
)

type normalizedScoreSearch struct{}

func (normalizedScoreSearch) SearchShops(context.Context, string, string, string, *int64, *int64, int) ([]ShopHit, error) {
	return []ShopHit{{ShopID: 29, Name: "无界餐厅", Area: "东城区", TypeName: "美食", AvgPrice: 128, Score: 47}}, nil
}

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

func TestStrictDecode_TrailingJSON(t *testing.T) {
	t.Parallel()
	var a searchArgs
	err := strictDecode(`{"query":"a"}{"query":"b"}`, &a)
	if err == nil {
		t.Fatal("expected trailing JSON reject")
	}
}

func TestTruncateUTF8(t *testing.T) {
	t.Parallel()
	s := truncateUTF8("你好世界测试", 2)
	if s != "你好…[truncated]" {
		t.Fatalf("got %q", s)
	}
}

func TestToolExecutor_NormalizesShopScoreForModel(t *testing.T) {
	t.Parallel()
	exec := &ToolExecutor{Search: normalizedScoreSearch{}, Ledger: NewEvidenceLedger()}
	out, err := exec.Execute(context.Background(), ToolSearchShops, `{"query":"无界餐厅"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"score":4.7`) || strings.Contains(out, `"score":47`) {
		t.Fatalf("score should be user-facing 4.7: %s", out)
	}
}

func TestToolExecutorStructuredPreservesFailureAndMetadata(t *testing.T) {
	t.Parallel()
	exec := &ToolExecutor{Ledger: NewEvidenceLedger()}
	result := exec.ExecuteStructured(context.Background(), ToolGetShop, `{"shop_id":7}`)
	if result.Status != ActionFailed || result.ErrorCode != ErrToolNotAllowed {
		t.Fatalf("structured result=%+v", result)
	}
	if result.Tool != ToolGetShop || result.ArgsHash == "" || result.LatencyMs < 0 {
		t.Fatalf("missing audit metadata: %+v", result)
	}
}

func TestToolExecutorStructuredReportsCandidatesAndCount(t *testing.T) {
	t.Parallel()
	exec := &ToolExecutor{Search: normalizedScoreSearch{}, Ledger: NewEvidenceLedger()}
	result := exec.ExecuteStructured(context.Background(), ToolSearchShops, `{"query":"无界餐厅"}`)
	if result.Status != ActionSucceeded || result.ResultCount != 1 {
		t.Fatalf("structured result=%+v", result)
	}
	if len(result.CandidateIDs) != 1 || result.CandidateIDs[0] != 29 {
		t.Fatalf("candidate metadata=%v", result.CandidateIDs)
	}
}
