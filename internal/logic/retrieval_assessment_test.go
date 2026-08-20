package logic

import (
	"testing"

	repoInterfaces "local-review-go/internal/repository/interface"
)

func TestAssessRetrievalAbstainsOnUniformlyIrrelevantCandidates(t *testing.T) {
	candidates := []repoInterfaces.ShopSearchResult{
		{ShopID: 1, TextContent: "有直接评价"},
		{ShopID: 2, TextContent: "另一条评价"},
	}
	got := AssessRetrieval(candidates, map[int64]float64{1: 0.1, 2: 0.12}, true)
	if got.Decision != RetrievalAbstain || got.Reason != "reranker_uniformly_irrelevant" {
		t.Fatalf("unexpected assessment: %+v", got)
	}
}

func TestAssessRetrievalRequiresEvidenceForSoftPreference(t *testing.T) {
	candidates := []repoInterfaces.ShopSearchResult{{ShopID: 1}, {ShopID: 2}}
	got := AssessRetrieval(candidates, map[int64]float64{1: 0.8, 2: 0.7}, true)
	if got.Decision != RetrievalVerify {
		t.Fatalf("unexpected assessment: %+v", got)
	}
}

func TestAssessRetrievalAcceptsRelevantGroundedCandidates(t *testing.T) {
	candidates := []repoInterfaces.ShopSearchResult{{ShopID: 1, TextContent: "安静且有电源"}}
	got := AssessRetrieval(candidates, map[int64]float64{1: 0.9}, true)
	if got.Decision != RetrievalAccept || got.Confidence < 0.8 {
		t.Fatalf("unexpected assessment: %+v", got)
	}
}
