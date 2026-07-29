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
	raw, err := os.ReadFile("../../rag-evals/golden/agent.v2.json")
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
		System: "agent", ChatModel: "fake", Mode: "fake", AgentMaxSteps: 3, AgentMaxTools: 5, PolicyVersion: "v1",
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
	if strings.Contains(string(b), "pass_at_k") {
		t.Fatal("misnamed pass_at_k must not appear")
	}
	summary := loaded["summary"].(map[string]any)
	if _, ok := summary["all_trials_pass_rate"]; !ok {
		t.Fatal("missing all_trials_pass_rate")
	}
}

func TestCompareBaseline_RequiresSameTasks(t *testing.T) {
	cases := []CaseReport{{ID: "a", Trials: TrialAggregate{Trials: 3}}}
	rep := AgentEvalReport{
		Version: reportVersion, DatasetHash: "sha256:abc", DatasetVer: "agent.v2",
		Experiment:      ExperimentMeta{System: "agent"},
		TagOutcomeRates: map[string]float64{"memory": 0.5, "search": 0.6},
		Summary:         ReportSummary{OutcomeRate: 0.6, CompositePassRate: 0.5, P50LatencyMs: 100},
		Cases:           cases,
	}
	dir := t.TempDir()
	basePath := filepath.Join(dir, "hybrid_task_v2.json")
	base := AgentEvalReport{
		Version: reportVersion, DatasetHash: "sha256:abc", DatasetVer: "agent.v2",
		Experiment:      ExperimentMeta{System: "hybrid_rag"},
		TagOutcomeRates: map[string]float64{"memory": 0.4, "search": 0.3},
		Summary:         ReportSummary{OutcomeRate: 0.4, CompositePassRate: 0.3, P50LatencyMs: 50},
		Cases:           cases,
	}
	b, _ := json.Marshal(base)
	_ = os.WriteFile(basePath, b, 0o644)
	if err := compareHybridBaseline(&rep, basePath); err != nil {
		t.Fatal(err)
	}
	if rep.Comparison == nil || rep.Comparison.PerTagTaskSuccessDelta["memory"] != 0.1 {
		t.Fatalf("%+v", rep.Comparison)
	}
	if rep.Comparison.TaskSuccessDelta != 0.2 || !rep.Comparison.SameCasesAndTrials {
		t.Fatalf("%+v", rep.Comparison)
	}
	raw, _ := json.Marshal(rep.Comparison)
	if strings.Contains(string(raw), "baseline_hit_rate") {
		t.Fatal("same-task comparison must not use retrieval hit rate")
	}
}
