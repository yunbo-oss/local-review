package agent

import "testing"

func TestVerifyAnswer_NoCitationWhenCandidatesExist(t *testing.T) {
	t.Parallel()
	l := NewEvidenceLedger()
	l.DiscoverFromSearch(1, "A", nil)
	err := VerifyAnswer("附近有不错的店可以去看看", l, VerifyOptions{})
	if err == nil {
		t.Fatal("expected no_citation")
	}
	if pe, ok := err.(*PublicError); !ok || pe.Code != ErrGroundingNoCitation {
		t.Fatalf("got %v", err)
	}
}

func TestVerifyAnswer_UnknownShop(t *testing.T) {
	t.Parallel()
	l := NewEvidenceLedger()
	l.DiscoverFromSearch(1, "A", nil)
	err := VerifyAnswer("推荐 [shop:99]", l, VerifyOptions{})
	if err == nil {
		t.Fatal("expected unknown")
	}
	if pe, ok := err.(*PublicError); !ok || pe.Code != ErrGroundingUnknownShop {
		t.Fatalf("got %v", err)
	}
}

func TestVerifyAnswer_NoResultOK(t *testing.T) {
	t.Parallel()
	l := NewEvidenceLedger()
	if err := VerifyAnswer("暂时没有合适店铺", l, VerifyOptions{AllowNoResult: true}); err != nil {
		t.Fatal(err)
	}
	if err := VerifyAnswer("暂时没有合适店铺", l, VerifyOptions{}); err != nil {
		t.Fatal("empty ledger should allow no citation")
	}
}

func TestVerifyAnswer_FactConflict(t *testing.T) {
	t.Parallel()
	l := NewEvidenceLedger()
	l.DiscoverFromSearch(1, "A", map[string]any{"avg_price": int64(80)})
	err := VerifyAnswer("推荐 [shop:1]，人均200", l, VerifyOptions{})
	if err == nil {
		t.Fatal("expected fact conflict")
	}
	if pe, ok := err.(*PublicError); !ok || pe.Code != ErrGroundingFactConflict {
		t.Fatalf("got %v", err)
	}
}

func TestVerifyAnswer_OK(t *testing.T) {
	t.Parallel()
	l := NewEvidenceLedger()
	l.DiscoverFromSearch(1, "A", map[string]any{"avg_price": int64(80)})
	if err := VerifyAnswer("推荐 [shop:1]，人均80，环境不错", l, VerifyOptions{}); err != nil {
		t.Fatal(err)
	}
}
