package agent

import "testing"

func TestEvidenceLedger_DiscoverAndCiteable(t *testing.T) {
	t.Parallel()
	l := NewEvidenceLedger()
	l.DiscoverFromSearch(1, "店A", map[string]any{"avg_price": int64(80)})
	if !l.IsDiscovered(1) {
		t.Fatal("should be discovered")
	}
	ids := l.CiteableIDs()
	if len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("citeable=%v", ids)
	}
	ev := l.Get(1)
	if ev.Fields["avg_price"].Value.(int64) != 80 {
		t.Fatalf("avg_price=%v", ev.Fields["avg_price"])
	}
}

func TestEvidenceLedger_EmptyBlogsDoesNotWash(t *testing.T) {
	t.Parallel()
	l := NewEvidenceLedger()
	// 未发现店铺：空 blogs 不得创建 citeable
	err := l.RecordBlogs(99, nil)
	if err == nil {
		t.Fatal("expected not allowed")
	}
	if l.IsDiscovered(99) || len(l.CiteableIDs()) != 0 {
		t.Fatal("empty blogs must not grant citeable identity")
	}
}

func TestEvidenceLedger_GetShopRequiresDiscovered(t *testing.T) {
	t.Parallel()
	l := NewEvidenceLedger()
	err := l.VerifyFromGetShop(5, "幽灵店", map[string]any{"avg_price": int64(1)})
	if err == nil {
		t.Fatal("expected not allowed for undiscovered")
	}
	l.DiscoverFromSearch(5, "真店", nil)
	if err := l.VerifyFromGetShop(5, "真店", map[string]any{"avg_price": int64(120)}); err != nil {
		t.Fatal(err)
	}
	ev := l.Get(5)
	if !ev.Verified || ev.Fields["avg_price"].Value.(int64) != 120 {
		t.Fatalf("verified=%v fields=%v", ev.Verified, ev.Fields)
	}
}

func TestEvidenceLedger_NonEmptyBlogsOnDiscovered(t *testing.T) {
	t.Parallel()
	l := NewEvidenceLedger()
	l.DiscoverFromSearch(2, "B", nil)
	if err := l.RecordBlogs(2, []int64{10, 11}); err != nil {
		t.Fatal(err)
	}
	ev := l.Get(2)
	if len(ev.BlogIDs) != 2 {
		t.Fatalf("blogs=%v", ev.BlogIDs)
	}
	// 空列表不清除已 discovered
	if err := l.RecordBlogs(2, nil); err != nil {
		t.Fatal(err)
	}
	if !l.IsDiscovered(2) {
		t.Fatal("still discovered")
	}
}

func TestSemanticEvidenceIDs(t *testing.T) {
	t.Parallel()
	l := NewEvidenceLedger()
	l.DiscoverFromSearch(70, "无障碍店", nil)
	if err := l.RecordBlogEvidence(70, []int64{1}, []string{"入口有坡道，轮椅通行方便"}); err != nil {
		t.Fatal(err)
	}
	got := l.SemanticEvidenceIDs(RequiredSemanticConcepts("找无障碍的店"))
	if len(got) != 1 || got[0] != 70 {
		t.Fatalf("semantic ids=%v", got)
	}
	if got := l.SemanticEvidenceIDs(RequiredSemanticConcepts("找安静办公的店")); len(got) != 0 {
		t.Fatalf("unexpected work evidence=%v", got)
	}
}

func TestSemanticEvidenceIDs_FromSearchReviewSummary(t *testing.T) {
	t.Parallel()
	l := NewEvidenceLedger()
	l.DiscoverFromSearch(70, "无障碍店", map[string]any{
		"review_evidence": "评价摘要：入口有坡道，轮椅通行方便",
	})
	got := l.SemanticEvidenceIDs(RequiredSemanticConcepts("找无障碍的店"))
	if len(got) != 1 || got[0] != 70 {
		t.Fatalf("semantic ids from search summary=%v", got)
	}
}

func TestReviewTextSupportsSemantics(t *testing.T) {
	t.Parallel()
	required := RequiredSemanticConcepts("适合家庭聚餐，也要照顾孩子")
	if !ReviewTextSupportsSemantics("有儿童椅，老人孩子一起家庭聚餐很方便", required) {
		t.Fatal("expected family review evidence to pass")
	}
	if ReviewTextSupportsSemantics("环境整洁，服务正常，未描述特定人群", required) {
		t.Fatal("review without family evidence must not pass")
	}
}
