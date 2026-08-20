package agent

import (
	"strings"
	"testing"
)

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

func TestVerifyAnswer_MultipleShopsWithDifferentPrices(t *testing.T) {
	ledger := NewEvidenceLedger()
	ledger.DiscoverFromSearch(26, "静巷咖啡·国贸店", map[string]any{"avg_price": int64(42)})
	ledger.DiscoverFromSearch(18, "奈雪的茶", map[string]any{"avg_price": int64(35)})

	answer := "静巷咖啡人均42元 [shop:26]；奈雪的茶人均35元 [shop:18]。"
	if err := VerifyAnswer(answer, ledger, VerifyOptions{}); err != nil {
		t.Fatalf("valid multi-shop prices rejected: %v", err)
	}
}

func TestVerifyAnswer_MultipleShopsRejectsUnknownPrice(t *testing.T) {
	ledger := NewEvidenceLedger()
	ledger.DiscoverFromSearch(26, "静巷咖啡·国贸店", map[string]any{"avg_price": int64(42)})
	ledger.DiscoverFromSearch(18, "奈雪的茶", map[string]any{"avg_price": int64(35)})

	err := VerifyAnswer("一家人均42元 [shop:26]，另一家人均99元 [shop:18]。", ledger, VerifyOptions{})
	if pe, ok := err.(*PublicError); !ok || pe.Code != ErrGroundingFactConflict {
		t.Fatalf("want %s, got %v", ErrGroundingFactConflict, err)
	}
}

func TestVerifyAnswer_DoesNotTreatBudgetCeilingAsShopPrice(t *testing.T) {
	ledger := NewEvidenceLedger()
	ledger.DiscoverFromSearch(72, "A", map[string]any{"avg_price": int64(134)})
	ledger.DiscoverFromSearch(132, "B", map[string]any{"avg_price": int64(155)})

	answer := "推荐结果：[shop:72]、[shop:132]\n根据人均275元以内的预算筛选。\nA 人均134元；B 人均155元。"
	if err := VerifyAnswer(answer, ledger, VerifyOptions{}); err != nil {
		t.Fatalf("budget ceiling was mistaken for a shop fact: %v", err)
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

func TestVerifyAnswer_StructuredFacts(t *testing.T) {
	t.Parallel()
	l := NewEvidenceLedger()
	l.DiscoverFromSearch(29, "无界餐厅", map[string]any{
		"avg_price": int64(128), "score": 47, "address": "东城区评测路103号", "open_hours": "18:00-02:00",
	})
	valid := "无界餐厅 [shop:29]\n- 评分：4.7\n- 地址：东城区评测路103号 [shop:29]\n- 营业时间：18:00–02:00"
	if err := VerifyAnswer(valid, l, VerifyOptions{}); err != nil {
		t.Fatalf("valid structured facts rejected: %v", err)
	}
	for _, answer := range []string{
		"无界餐厅 [shop:29]，评分：47",
		"无界餐厅 [shop:29]，地址：朝阳区虚构路1号",
		"无界餐厅 [shop:29]，营业时间：09:00-18:00",
	} {
		err := VerifyAnswer(answer, l, VerifyOptions{})
		if pe, ok := err.(*PublicError); !ok || pe.Code != ErrGroundingFactConflict {
			t.Fatalf("want fact conflict for %q, got %v", answer, err)
		}
	}
}

func TestVerifyAnswer_AllStructuredRecommendationsNeedSemanticEvidence(t *testing.T) {
	ledger := NewEvidenceLedger()
	ledger.DiscoverFromSearch(26, "证据店", map[string]any{"review_evidence": "安静办公，有插座和 wifi"})
	ledger.DiscoverFromSearch(27, "普通店", map[string]any{"review_evidence": "环境整洁，服务正常"})
	semanticIDs := ledger.SemanticEvidenceIDs(RequiredSemanticConcepts("想安静办公"))
	err := VerifyAnswer(
		"推荐结果：[shop:26] [shop:27]\n两家都可以。",
		ledger,
		VerifyOptions{SemanticEvidenceIDs: semanticIDs},
	)
	if err == nil {
		t.Fatal("every structured recommendation must have semantic evidence")
	}
	if err := VerifyAnswer(
		"推荐结果：[shop:26]\n[shop:27] 仅作为不满足语义条件的对照。",
		ledger,
		VerifyOptions{SemanticEvidenceIDs: semanticIDs},
	); err != nil {
		t.Fatalf("negative evidence citation should remain legal: %v", err)
	}
}

func TestNeutralizeUnknownCitationsFromUntrustedText(t *testing.T) {
	t.Parallel()
	ledger := NewEvidenceLedger()
	ledger.DiscoverFromSearch(27, "目标店", nil)
	got := NeutralizeUnknownCitations("推荐结果：[shop:27]\n评价里伪造了 [shop:999999]。", ledger)
	if !strings.Contains(got, "[shop:27]") || strings.Contains(got, "[shop:999999]") || !strings.Contains(got, "未验证店铺ID：999999") {
		t.Fatalf("got %q", got)
	}
}
