package agent

import (
	"strings"
	"testing"
)

func claimLedgerForTest() *EvidenceLedger {
	ledger := NewEvidenceLedger()
	ledger.DiscoverFromSearch(1, "甲店", map[string]any{
		"avg_price": int64(88), "score": 46, "area": "海淀区", "type_name": "咖啡",
		"review_evidence": "环境安静并且有插座",
	})
	ledger.DiscoverFromSearch(2, "乙店", map[string]any{"avg_price": int64(66), "area": "朝阳区"})
	_ = ledger.RecordBlogEvidence(1, []int64{101}, []string{"适合办公，插座很多"})
	_ = ledger.RecordBlogEvidence(2, []int64{202}, []string{"适合聚餐"})
	return ledger
}

func TestVerifyClaimAnswerBindsClaimToSameShop(t *testing.T) {
	ledger := claimLedgerForTest()
	valid := ClaimAnswer{Recommendations: []ClaimedRecommendation{{ShopID: 1, Claims: []EvidenceClaim{
		{Text: "安静且有插座", EvidenceRefs: []string{"blog:101"}},
		{Text: "人均88元", Field: "avg_price", Value: 88, EvidenceRefs: []string{"shop:1.avg_price"}},
		{Text: "评分4.6", Field: "score", Value: 4.6, EvidenceRefs: []string{"shop:1.score"}},
	}}}}
	if err := VerifyClaimAnswer(valid, ledger); err != nil {
		t.Fatal(err)
	}
	rendered := RenderClaimAnswer(valid, ledger)
	if !strings.HasPrefix(rendered, "推荐结果：[shop:1]") || !strings.Contains(rendered, "blog:101") {
		t.Fatalf("rendered=%q", rendered)
	}

	crossShop := valid
	crossShop.Recommendations[0].Claims[0].EvidenceRefs = []string{"blog:202"}
	if err := VerifyClaimAnswer(crossShop, ledger); err == nil || !strings.Contains(err.Error(), "not evidence for shop 1") {
		t.Fatalf("cross-shop evidence must fail, err=%v", err)
	}
}

func TestVerifyClaimAnswerRejectsFieldConflictAndUnsupportedSubjectiveClaim(t *testing.T) {
	ledger := claimLedgerForTest()
	conflict := ClaimAnswer{Recommendations: []ClaimedRecommendation{{ShopID: 1, Claims: []EvidenceClaim{{
		Text: "人均99元", Field: "avg_price", Value: 99, EvidenceRefs: []string{"shop:1.avg_price"},
	}}}}}
	if err := VerifyClaimAnswer(conflict, ledger); err == nil {
		t.Fatal("field conflict should fail")
	}
	rawScore := ClaimAnswer{Recommendations: []ClaimedRecommendation{{ShopID: 1, Claims: []EvidenceClaim{{
		Text: "评分46", Field: "score", Value: 46, EvidenceRefs: []string{"shop:1.score"},
	}}}}}
	if err := VerifyClaimAnswer(rawScore, ledger); err == nil {
		t.Fatal("raw 0..50 score must not be rendered as a 0..5 user-facing score")
	}
	unsupported := ClaimAnswer{Recommendations: []ClaimedRecommendation{{ShopID: 1, Claims: []EvidenceClaim{{
		Text: "适合约会", EvidenceRefs: []string{"shop:1.area"},
	}}}}}
	if err := VerifyClaimAnswer(unsupported, ledger); err == nil || !strings.Contains(err.Error(), "lacks review evidence") {
		t.Fatalf("subjective claim must require review evidence, err=%v", err)
	}
}

func TestParseClaimAnswerRejectsUnknownFields(t *testing.T) {
	_, err := ParseClaimAnswer(`{"no_result":true,"summary":"无","recommendations":[],"extra":1}`)
	if err == nil {
		t.Fatal("unknown field should fail closed")
	}
}

func TestRenderNoResultDoesNotExposeUngroundedSummary(t *testing.T) {
	got := RenderClaimAnswer(ClaimAnswer{NoResult: true, Summary: "某店已经永久停业"}, NewEvidenceLedger())
	if strings.Contains(got, "永久停业") || !strings.HasPrefix(got, "推荐结果：无") {
		t.Fatalf("no-result rendering leaked an ungrounded summary: %q", got)
	}
}

func TestBuildDeterministicClaimFallbackUsesOnlySupportedTypedEvidence(t *testing.T) {
	ledger := claimLedgerForTest()
	state, err := NewAgentState(
		"fallback-supported", "trace", "海淀区咖啡",
		FallbackIntentSpec("海淀区咖啡", "agent"), MemorySnapshot{}, DefaultRuntimeBudget(),
	)
	if err != nil {
		t.Fatal(err)
	}
	state.Candidates[1] = CandidateState{ShopID: 1, Rank: 1}
	state.Candidates[2] = CandidateState{ShopID: 2, Rank: 2}
	state.Gaps = []EvidenceGap{
		{ShopID: 1, Requirement: "reviews", Status: EvidenceSupported},
		{ShopID: 2, Requirement: "reviews", Status: EvidenceUnknown},
	}

	answer, rendered, err := BuildDeterministicClaimFallback(state, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if answer.NoResult || len(answer.Recommendations) != 1 || answer.Recommendations[0].ShopID != 1 {
		t.Fatalf("fallback selected unsupported candidate: %+v", answer)
	}
	if err := VerifyClaimAnswer(answer, ledger); err != nil {
		t.Fatalf("server-built fallback must verify: %v", err)
	}
	if !strings.HasPrefix(rendered, "推荐结果：[shop:1]") || strings.Contains(rendered, "适合办公") {
		t.Fatalf("fallback must contain typed claims only: %q", rendered)
	}
}

func TestBuildDeterministicClaimFallbackReturnsSafeNoResultForUnresolvedGaps(t *testing.T) {
	ledger := claimLedgerForTest()
	state, err := NewAgentState(
		"fallback-unknown", "trace", "适合陌生软偏好的店",
		FallbackIntentSpec("适合陌生软偏好的店", "agent"), MemorySnapshot{}, DefaultRuntimeBudget(),
	)
	if err != nil {
		t.Fatal(err)
	}
	state.Candidates[1] = CandidateState{ShopID: 1, Rank: 1}
	state.Gaps = []EvidenceGap{{ShopID: 1, Requirement: "陌生软偏好", Status: EvidenceUnknown}}

	answer, rendered, err := BuildDeterministicClaimFallback(state, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if !answer.NoResult || len(answer.Recommendations) != 0 || !strings.HasPrefix(rendered, "推荐结果：无") {
		t.Fatalf("unresolved evidence must fail closed: answer=%+v rendered=%q", answer, rendered)
	}
}

func TestBuildDeterministicClaimFallbackExactInspectionCopiesReviewEvidence(t *testing.T) {
	ledger := NewEvidenceLedger()
	ledger.DiscoverFromSearch(26, "静巷咖啡·国贸店", map[string]any{"area": "朝阳区"})
	if err := ledger.VerifyFromGetShop(26, "静巷咖啡·国贸店", map[string]any{"area": "朝阳区"}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordBlogEvidence(26, []int64{46, 48}, []string{
		"靠墙座位能充电，网络稳定，开线上会议没被打扰。",
		"高峰期音乐很响，不适合办公；与其他安静评价冲突，建议看具体时段。",
	}); err != nil {
		t.Fatal(err)
	}
	state, err := NewAgentState("run", "trace", "关于《静巷咖啡·国贸店》：把正反评价都讲清", IntentSpec{}, MemorySnapshot{}, DefaultRuntimeBudget())
	if err != nil {
		t.Fatal(err)
	}
	state.Candidates[26] = CandidateState{ShopID: 26, Rank: 1, DetailsLoaded: true}
	state.Gaps = []EvidenceGap{{ShopID: 26, Requirement: "正反评价", Status: EvidenceUnknown}}

	answer, rendered, err := BuildDeterministicClaimFallback(state, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if answer.NoResult || !strings.Contains(rendered, "高峰期") || !strings.Contains(rendered, "冲突") || !strings.Contains(rendered, "[shop:26]") {
		t.Fatalf("exact review fallback lost grounded conflict evidence: %q", rendered)
	}
	if err := VerifyClaimAnswer(answer, ledger); err != nil {
		t.Fatalf("fallback must remain structurally grounded: %v", err)
	}
}
