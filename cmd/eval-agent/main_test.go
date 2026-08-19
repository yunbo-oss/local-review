package main

import (
	"context"
	"encoding/json"
	"math"
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
	if loaded["n_cases"] != float64(len(reports)) || loaded["n_trials"] != float64(len(reports)*3) {
		t.Fatalf("ambiguous case/trial counts: n_cases=%v n_trials=%v", loaded["n_cases"], loaded["n_trials"])
	}
	if strings.Contains(string(b), "pass_at_k") {
		t.Fatal("misnamed pass_at_k must not appear")
	}
	summary := loaded["summary"].(map[string]any)
	if _, ok := summary["all_trials_pass_rate"]; !ok {
		t.Fatal("missing all_trials_pass_rate")
	}
	for _, key := range []string{
		"trial_micro_task_success_rate", "scenario_macro_task_success_rate",
		"outcome_wilson_95", "groundedness_wilson_95", "composite_pass_wilson_95",
	} {
		if _, ok := summary[key]; !ok {
			t.Fatalf("missing explicit metric %q", key)
		}
	}
}

func TestBuildReportDistinguishesTrialMicroAndScenarioMacro(t *testing.T) {
	passing := func(outcome, ground, composite bool) TrialDetail {
		return TrialDetail{
			Outcome: GradeResult{Pass: outcome},
			Ground:  GradeResult{Pass: ground},
			Traj:    GradeResult{Pass: composite},
			Pass:    composite,
		}
	}
	cases := []CaseReport{
		{
			ID: "three-trial-case", Trials: TrialAggregate{Trials: 3, Successes: 2, SuccessRate: 2.0 / 3.0},
			TrialDetails: []TrialDetail{
				passing(true, true, true),
				passing(true, true, true),
				passing(true, true, false),
			},
		},
		{
			ID: "one-trial-case", Trials: TrialAggregate{Trials: 1},
			TrialDetails: []TrialDetail{passing(false, false, false)},
		},
	}
	rep, err := buildReport("agent.test", "sha256:test", ExperimentMeta{System: "agent"}, cases, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rep.NCases != 2 || rep.NTrials != 4 || rep.NEvaluated != 4 {
		t.Fatalf("counts=%+v", rep)
	}
	if rep.Summary.TrialMicroTaskSuccessRate != 0.75 || rep.Summary.OutcomeRate != 0.75 {
		t.Fatalf("trial-micro task success=%v", rep.Summary.TrialMicroTaskSuccessRate)
	}
	if rep.Summary.ScenarioMacroTaskSuccess != 0.5 {
		t.Fatalf("scenario-macro task success=%v, want 0.5", rep.Summary.ScenarioMacroTaskSuccess)
	}
	if rep.Summary.TrialMicroCompositeRate != 0.5 {
		t.Fatalf("trial-micro composite=%v, want 0.5", rep.Summary.TrialMicroCompositeRate)
	}
	if diff := rep.Summary.ScenarioMacroCompositeRate - 1.0/3.0; diff < -1e-12 || diff > 1e-12 {
		t.Fatalf("scenario-macro composite=%v, want 1/3", rep.Summary.ScenarioMacroCompositeRate)
	}
	assertWilson := func(name string, got WilsonInterval, successes int) {
		t.Helper()
		if got.Method != "wilson_score" || got.ConfidenceLevel != 0.95 || got.Successes != successes || got.Trials != 4 {
			t.Fatalf("%s metadata=%+v", name, got)
		}
		if !(got.Lower < float64(successes)/4 && got.Upper > float64(successes)/4) {
			t.Fatalf("%s interval does not contain point estimate: %+v", name, got)
		}
	}
	assertWilson("outcome", rep.Summary.OutcomeWilson95, 3)
	assertWilson("groundedness", rep.Summary.GroundednessWilson95, 3)
	assertWilson("composite", rep.Summary.CompositePassWilson95, 2)
	if math.Abs(rep.Summary.OutcomeWilson95.Lower-0.30064184258240184) > 1e-12 ||
		math.Abs(rep.Summary.OutcomeWilson95.Upper-0.9544127391902995) > 1e-12 {
		t.Fatalf("outcome is not a Wilson 95%% interval: %+v", rep.Summary.OutcomeWilson95)
	}
}

func TestBuildReportAggregatesAdaptiveArchitectureMetrics(t *testing.T) {
	cases := []CaseReport{{
		ID: "adaptive", Trials: TrialAggregate{Trials: 1, Successes: 1, SuccessRate: 1},
		TrialDetails: []TrialDetail{{
			Outcome: GradeResult{Pass: true}, Ground: GradeResult{Pass: true}, Traj: GradeResult{Pass: true}, Pass: true,
			Actual: OutcomeActual{
				Route: "agent_multistep", RuntimeStatus: "COMPLETED", AnswerVerified: true,
				IntentConfidence: 0.8, RewriteCount: 2, PlanVersions: 2, Replans: 1,
				PlanFallback: true, ClaimFallback: true, ClaimCount: 4, ClaimsWithEvidence: 3,
				RetrievalConfidence: 0.7, RetrievalDecision: "abstain",
			},
		}},
	}}
	rep, err := buildReport("agent.test", "sha256:test", ExperimentMeta{System: "agent"}, cases, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := rep.Summary
	if got.AvgIntentConfidence != 0.8 || got.AvgRewriteCount != 2 || got.AvgPlanVersions != 2 ||
		got.AvgReplans != 1 || got.PlanFallbackRate != 1 || got.ClaimEvidenceCoverage != 0.75 ||
		got.ClaimFallbackRate != 1 || got.AnswerVerifiedRate != 1 ||
		got.AvgRetrievalConfidence != 0.7 || got.RetrievalAbstentionRate != 1 ||
		got.RouteCounts["agent_multistep"] != 1 || got.RouteTaskSuccessRates["agent_multistep"] != 1 ||
		got.RuntimeStatusCounts["COMPLETED"] != 1 {
		t.Fatalf("adaptive summary metrics=%+v", got)
	}
}

func TestCompareBaseline_RequiresSameTasks(t *testing.T) {
	agentCases := []CaseReport{
		{ID: "a", OutcomePass: 1, Trials: TrialAggregate{Trials: 3, Successes: 2, SuccessRate: 2.0 / 3.0}},
		{ID: "b", OutcomePass: 0, Trials: TrialAggregate{Trials: 1}},
	}
	hybridCases := []CaseReport{
		{ID: "a", OutcomePass: 1.0 / 3.0, Trials: TrialAggregate{Trials: 3, Successes: 1, SuccessRate: 1.0 / 3.0}},
		{ID: "b", OutcomePass: 0, Trials: TrialAggregate{Trials: 1}},
	}
	rep := AgentEvalReport{
		Version: reportVersion, DatasetHash: "sha256:abc", DatasetVer: "agent.v2",
		Experiment:      ExperimentMeta{System: "agent"},
		TagOutcomeRates: map[string]float64{"memory": 0.5, "search": 0.6},
		Summary:         ReportSummary{OutcomeRate: 0.6, CompositePassRate: 0.5, P50LatencyMs: 100},
		Cases:           agentCases,
	}
	dir := t.TempDir()
	basePath := filepath.Join(dir, "hybrid_task_v2.json")
	base := AgentEvalReport{
		Version: reportVersion, DatasetHash: "sha256:abc", DatasetVer: "agent.v2",
		Experiment:      ExperimentMeta{System: "hybrid_rag"},
		TagOutcomeRates: map[string]float64{"memory": 0.4, "search": 0.3},
		Summary:         ReportSummary{OutcomeRate: 0.4, CompositePassRate: 0.3, P50LatencyMs: 50},
		Cases:           hybridCases,
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
	if rep.Comparison.ScenarioMacroTaskSuccessDelta != 0.3333 || rep.Comparison.ScenarioMacroCompositePassDelta != 0.1667 {
		t.Fatalf("missing scenario-macro comparison deltas: %+v", rep.Comparison)
	}
	raw, _ := json.Marshal(rep.Comparison)
	if strings.Contains(string(raw), "baseline_hit_rate") {
		t.Fatal("same-task comparison must not use retrieval hit rate")
	}
}
