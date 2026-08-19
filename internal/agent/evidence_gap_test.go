package agent

import "testing"

func TestEvidenceGapEvaluatorRequiresSearchBeforeCandidateTools(t *testing.T) {
	state := newRuntimeTestState(t)
	gaps := (DeterministicEvidenceGapEvaluator{}).Evaluate(state)
	if len(gaps) != 1 || gaps[0].EvidenceType != ToolSearchShops || gaps[0].Status != EvidenceMissing {
		t.Fatalf("gaps=%+v", gaps)
	}
}

func TestEvidenceGapEvaluatorTracksDetailsAndSemanticReviews(t *testing.T) {
	state := newRuntimeTestState(t)
	state.Intent.EvidenceRequirements = []string{"shop_detail", "reviews"}
	state.Intent.SoftPreferences = []string{"安静办公"}
	state.Candidates[7] = CandidateState{ShopID: 7, Rank: 1, SourceActionID: "search-1"}
	state.Evidence = EvidenceSnapshot{Shops: map[int64]ShopEvidenceSnapshot{
		7: {
			ShopID: 7, DiscoveredBy: ToolSearchShops, Verified: true,
			BlogIDs: []int64{91}, BlogTexts: []string{"环境安静，桌边有插座，适合用电脑办公"},
		},
	}}
	state.Gaps = (DeterministicEvidenceGapEvaluator{}).Evaluate(state)
	updateCandidateEvidenceScores(state)
	if len(state.Gaps) != 2 {
		t.Fatalf("gaps=%+v", state.Gaps)
	}
	for _, gap := range state.Gaps {
		if gap.Status != EvidenceSupported {
			t.Fatalf("gap should be supported: %+v", gap)
		}
	}
	if state.Candidates[7].EvidenceScore != 1 {
		t.Fatalf("evidence score=%f", state.Candidates[7].EvidenceScore)
	}
}

func TestEvidenceGapEvaluatorDoesNotTreatSearchSummaryAsRawReviews(t *testing.T) {
	state := newRuntimeTestState(t)
	state.Intent.EvidenceRequirements = []string{"reviews"}
	state.Candidates[7] = CandidateState{ShopID: 7, Rank: 1, SourceActionID: "search-1"}
	state.Evidence = EvidenceSnapshot{Shops: map[int64]ShopEvidenceSnapshot{
		7: {ShopID: 7, DiscoveredBy: ToolSearchShops, Fields: map[string]EvidenceValue{
			"review_evidence": {Value: "检索摘要提到环境安静", Source: ToolSearchShops},
		}},
	}}
	gaps := (DeterministicEvidenceGapEvaluator{}).Evaluate(state)
	if len(gaps) != 1 || gaps[0].Status != EvidenceMissing || gaps[0].ReasonCode != "RAW_REVIEWS_NOT_FETCHED" {
		t.Fatalf("search summary incorrectly satisfied raw-review requirement: %+v", gaps)
	}
}

func TestEvidenceGapEvaluatorLeavesOpenEndedPreferenceForJudge(t *testing.T) {
	state := newRuntimeTestState(t)
	state.Intent.SoftPreferences = []string{"有漂亮的落日景色"}
	state.Candidates[7] = CandidateState{ShopID: 7, Rank: 1, SourceActionID: "search-1"}
	state.Evidence = EvidenceSnapshot{Shops: map[int64]ShopEvidenceSnapshot{
		7: {ShopID: 7, DiscoveredBy: ToolSearchShops, BlogIDs: []int64{91}, BlogTexts: []string{"窗边视野开阔"}},
	}}
	gaps := (DeterministicEvidenceGapEvaluator{}).Evaluate(state)
	if len(gaps) != 1 || gaps[0].Status != EvidenceUnknown || gaps[0].ReasonCode != "SEMANTIC_JUDGE_REQUIRED" {
		t.Fatalf("gaps=%+v", gaps)
	}
}

func TestRecordBlogEvidenceAccumulatesPagesWithoutDuplicates(t *testing.T) {
	ledger := NewEvidenceLedger()
	ledger.DiscoverFromSearch(7, "七号咖啡", nil)
	if err := ledger.RecordBlogEvidence(7, []int64{1, 2}, []string{"第一页一", "第一页二"}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordBlogEvidence(7, []int64{2, 3}, []string{"重复二", "第二页三"}); err != nil {
		t.Fatal(err)
	}
	item := ledger.Get(7)
	if len(item.BlogIDs) != 3 || item.BlogIDs[2] != 3 || len(item.BlogTexts) != 3 || item.BlogTexts[2] != "第二页三" {
		t.Fatalf("accumulated evidence=%+v", item)
	}
}

func TestEvidenceAwareSecondStageReranksSupportedCandidate(t *testing.T) {
	state := newRuntimeTestState(t)
	state.Candidates[7] = CandidateState{ShopID: 7, Rank: 1, RetrievalRank: 1, SourceActionID: "search"}
	state.Candidates[9] = CandidateState{ShopID: 9, Rank: 2, RetrievalRank: 2, SourceActionID: "search"}
	state.Gaps = []EvidenceGap{
		{ShopID: 7, Requirement: "安静", Status: EvidenceUnknown},
		{ShopID: 9, Requirement: "安静", Status: EvidenceSupported},
	}
	updateCandidateEvidenceScores(state)
	if state.Candidates[9].Rank != 1 || state.Candidates[9].EvidenceScore != 1 || state.Candidates[7].RetrievalRank != 1 {
		t.Fatalf("evidence rerank=%+v", state.Candidates)
	}
}
