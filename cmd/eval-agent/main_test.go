package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportForbidsStubVersion(t *testing.T) {
	if err := assertNonStubVersion("agent-eval.v1-stub"); err == nil {
		t.Fatal("stub version must be rejected")
	}
	if err := assertNonStubVersion(reportVersion); err != nil {
		t.Fatal(err)
	}
}

func TestFakeHarness_TrialIsolationAndReportShape(t *testing.T) {
	raw, err := os.ReadFile("../../rag-evals/golden/agent.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var file AgentCaseFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	cases := filterCases(file.Cases, "test")
	if len(cases) < 2 {
		t.Fatal("need test cases")
	}
	cases = cases[:2]
	runner := &FakeRunner{}
	ctx := context.Background()
	var reports []CaseReport
	sessions := map[string]struct{}{}
	for _, c := range cases {
		cr := CaseReport{ID: c.ID, Tags: c.Tags}
		var passes []bool
		for tIdx := 0; tIdx < 3; tIdx++ {
			td, err := runner.RunTrial(ctx, c, tIdx, "agent_multistep")
			if err != nil {
				t.Fatal(err)
			}
			if td.SessionID == "" {
				t.Fatal("empty session")
			}
			if _, dup := sessions[td.SessionID]; dup {
				t.Fatalf("session not isolated: %s", td.SessionID)
			}
			sessions[td.SessionID] = struct{}{}
			gradeTrial(&td, c.Expected)
			passes = append(passes, td.Pass)
			cr.TrialDetails = append(cr.TrialDetails, td)
		}
		cr.Trials = AggregateTrials(passes)
		reports = append(reports, cr)
	}
	rep, err := buildReport(file.Version, sha256Hex(raw), ExperimentMeta{
		ChatModel: "fake", Mode: "fake", AgentMaxSteps: 3, AgentMaxTools: 5, PolicyVersion: "v1",
	}, reports, 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rep.Version, "stub") {
		t.Fatal(rep.Version)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "agent_latest.json")
	if err := writeReport(out, rep); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	var loaded map[string]any
	if err := json.Unmarshal(b, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded["version"] != reportVersion {
		t.Fatalf("%v", loaded["version"])
	}
	if _, ok := loaded["dataset_hash"]; !ok {
		t.Fatal("missing dataset_hash")
	}
	if _, ok := loaded["summary"]; !ok {
		t.Fatal("missing summary")
	}
}

func TestCompareBaseline_NoDenseField(t *testing.T) {
	rep := AgentEvalReport{
		Version:     reportVersion,
		TagOutcomes: map[string]float64{"memory": 0.5, "search": 0.6},
		Summary:     ReportSummary{P50LatencyMs: 100},
	}
	dir := t.TempDir()
	basePath := filepath.Join(dir, "hybrid_prod_v1.json")
	base := map[string]any{
		"dataset_sha256": "sha256:abc",
		"hit_rate_at_k":  0.4,
		"retriever":      "hybrid",
		"per_case":       []any{},
	}
	b, _ := json.Marshal(base)
	_ = os.WriteFile(basePath, b, 0o644)
	if err := compareHybridBaseline(&rep, basePath); err != nil {
		t.Fatal(err)
	}
	if rep.Comparison == nil || rep.Comparison.PerTagDelta["memory"] != 0.1 {
		t.Fatalf("%+v", rep.Comparison)
	}
	raw, _ := json.Marshal(rep.Comparison)
	if strings.Contains(string(raw), `"dense"`) {
		t.Fatal("comparison must not include dense A/B fields")
	}
}
