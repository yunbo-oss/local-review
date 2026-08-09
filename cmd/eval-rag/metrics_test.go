package main

import (
	"testing"
)

func TestHitRateVsRecall(t *testing.T) {
	t.Parallel()
	// Top5 命中 1/3 relevant：HitRate=1，Recall=1/3
	retrieved := []int64{10, 20, 30, 40, 50}
	relevant := []int64{20, 99, 88}
	hr := HitRateAtK(retrieved, relevant, 5)
	rc := RecallAtK(retrieved, relevant, 5)
	if hr != 1.0 {
		t.Fatalf("HitRate want 1, got %v", hr)
	}
	if rc < 0.33 || rc > 0.34 {
		t.Fatalf("Recall want ~1/3, got %v", rc)
	}
	if hr == rc {
		t.Fatal("HitRate must not equal Recall for this case (forbid mislabeling)")
	}
}

func TestPrecisionAtK(t *testing.T) {
	t.Parallel()
	retrieved := []int64{1, 2, 3, 4, 5}
	relevant := []int64{1, 2}
	p := PrecisionAtK(retrieved, relevant, 5)
	if p != 0.4 {
		t.Fatalf("Precision want 0.4, got %v", p)
	}
}

func TestMRR_SecondRank(t *testing.T) {
	t.Parallel()
	retrieved := []int64{9, 7, 3}
	relevant := []int64{7}
	if m := MRR(retrieved, relevant); m != 0.5 {
		t.Fatalf("MRR want 0.5, got %v", m)
	}
}

func TestValidateRelevantNonEmpty(t *testing.T) {
	t.Parallel()
	if err := ValidateRelevantNonEmpty("r001", nil); err == nil {
		t.Fatal("empty relevant must be rejected")
	}
	if err := ValidateRelevantNonEmpty("r001", []int64{1}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateNoResultCase(t *testing.T) {
	t.Parallel()
	if err := ValidateRetrievalCase(RetrievalCase{ID: "none", ExpectNoResults: true}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRetrievalCase(RetrievalCase{ID: "bad", ExpectNoResults: true, RelevantShopIDs: []int64{1}}); err == nil {
		t.Fatal("no-result case with relevant ids must fail")
	}
}

func TestAggregateQuality_InfraNotInDenominator(t *testing.T) {
	t.Parallel()
	cases := []CaseResult{
		{HitRate: 1, Recall: 1, Precision: 0.2, MRR: 1, NDCG: 1},
		{InfraError: "redis down", HitRate: 0, Recall: 0}, // must not dilute quality
	}
	hit, _, _, _, _ := AggregateQuality(cases, 5)
	if hit != 1.0 {
		t.Fatalf("infra must not enter quality denominator; hit=%v", hit)
	}
}

func TestAggregateQuality_NoResultNotInRankingDenominator(t *testing.T) {
	t.Parallel()
	cases := []CaseResult{
		{HitRate: 1, Recall: 1, Precision: 0.2, MRR: 1, NDCG: 1, TaskSuccess: true},
		{ExpectNoResults: true, NoResultPass: true, TaskSuccess: true},
	}
	hit, recall, _, _, _ := AggregateQuality(cases, 5)
	if hit != 1 || recall != 1 {
		t.Fatalf("ranking metrics diluted by no-result case: hit=%v recall=%v", hit, recall)
	}
	task, noResult := AggregateTaskSuccess(cases)
	if task != 1 || noResult != 1 {
		t.Fatalf("task=%v no_result=%v", task, noResult)
	}
}

func TestTaskSuccessWilsonExcludesInfrastructureErrors(t *testing.T) {
	t.Parallel()
	cases := []CaseResult{
		{TaskSuccess: true},
		{TaskSuccess: false},
		{TaskSuccess: false, InfraError: "redis down"},
	}
	ok, n := TaskSuccessCounts(cases)
	if ok != 1 || n != 2 {
		t.Fatalf("successes/trials=%d/%d", ok, n)
	}
	ci := wilson95(ok, n)
	if ci.Successes != 1 || ci.Trials != 2 || ci.Lower <= 0 || ci.Upper >= 1 {
		t.Fatalf("unexpected Wilson interval: %+v", ci)
	}
}

func TestNDCGAtK_Ideal(t *testing.T) {
	t.Parallel()
	retrieved := []int64{1, 2, 3}
	relevant := []int64{1, 2, 3}
	if n := NDCGAtK(retrieved, relevant, 3); n < 0.999 {
		t.Fatalf("ideal nDCG want 1, got %v", n)
	}
}
