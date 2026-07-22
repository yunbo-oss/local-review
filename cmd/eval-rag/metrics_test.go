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

func TestNDCGAtK_Ideal(t *testing.T) {
	t.Parallel()
	retrieved := []int64{1, 2, 3}
	relevant := []int64{1, 2, 3}
	if n := NDCGAtK(retrieved, relevant, 3); n < 0.999 {
		t.Fatalf("ideal nDCG want 1, got %v", n)
	}
}
