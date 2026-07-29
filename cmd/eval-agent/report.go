package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const reportVersion = "task-eval.v2"

// AgentEvalReport 非 stub 评测报告
type AgentEvalReport struct {
	Version               string             `json:"version"`
	DatasetHash           string             `json:"dataset_hash"`
	DatasetVer            string             `json:"dataset_version"`
	Experiment            ExperimentMeta     `json:"experiment"`
	Summary               ReportSummary      `json:"summary"`
	NTotal                int                `json:"n_total"`
	NEvaluated            int                `json:"n_evaluated"`
	NInfraError           int                `json:"n_infra_error"`
	InfraErrorRate        float64            `json:"infra_error_rate"`
	Cases                 []CaseReport       `json:"cases"`
	Comparison            *ComparisonSection `json:"comparison,omitempty"`
	TagOutcomeRates       map[string]float64 `json:"tag_outcome_rates,omitempty"`
	TagCompositePassRates map[string]float64 `json:"tag_composite_pass_rates,omitempty"`
}

type ExperimentMeta struct {
	System        string `json:"system"` // agent|hybrid_rag
	ChatModel     string `json:"chat_model"`
	AgentMaxSteps int    `json:"agent_max_steps"`
	AgentMaxTools int    `json:"agent_max_tool_calls"`
	ForceRoute    string `json:"force_route,omitempty"`
	Mode          string `json:"mode"` // inprocess|fake
	PolicyVersion string `json:"policy_version"`
}

type ReportSummary struct {
	OutcomeRate                float64            `json:"outcome_rate"`
	GroundednessRate           float64            `json:"groundedness_rate"`
	SuccessfulGroundednessRate float64            `json:"successful_groundedness_rate"`
	TrajectoryPassRate         float64            `json:"trajectory_pass_rate"`
	CompositePassRate          float64            `json:"composite_pass_rate"`
	TrialConsistency           map[string]float64 `json:"trial_consistency"`    // case_id → success_rate；含 pass^k 用 all-pass
	AllTrialsPassRate          float64            `json:"all_trials_pass_rate"` // ≥3 trials 的 case 全部通过占比
	P50LatencyMs               int64              `json:"p50_latency_ms"`
	P95LatencyMs               int64              `json:"p95_latency_ms"`
	AvgModelCalls              float64            `json:"avg_model_calls"`
	AvgToolCalls               float64            `json:"avg_tool_calls"`
	AvgTokens                  float64            `json:"avg_tokens"`
	TotalModelCalls            int                `json:"total_model_calls"`
	TotalToolCalls             int                `json:"total_tool_calls"`
	TotalTokens                int                `json:"total_tokens"`
	TotalPromptTokens          int                `json:"total_prompt_tokens"`
	TotalCompletionTokens      int                `json:"total_completion_tokens"`
	EstimatedCostUSDUpperBound float64            `json:"estimated_cost_usd_upper_bound"`
}

type CaseReport struct {
	ID           string         `json:"id"`
	Tags         []string       `json:"tags,omitempty"`
	Trials       TrialAggregate `json:"trials"`
	OutcomePass  float64        `json:"outcome_pass_rate"`
	GroundPass   float64        `json:"groundedness_pass_rate"`
	TrajPass     float64        `json:"trajectory_pass_rate"`
	InfraErrors  int            `json:"infra_errors"`
	TrialDetails []TrialDetail  `json:"trial_details,omitempty"`
}

type TrialDetail struct {
	TrialIndex int           `json:"trial_index"`
	SessionID  string        `json:"session_id"`
	Route      string        `json:"route,omitempty"`
	TraceID    string        `json:"trace_id,omitempty"`
	Outcome    GradeResult   `json:"outcome"`
	Ground     GradeResult   `json:"groundedness"`
	Traj       GradeResult   `json:"trajectory"`
	Actual     OutcomeActual `json:"actual"`
	InfraError string        `json:"infra_error,omitempty"`
	Pass       bool          `json:"pass"` // 三 grader 全过且无 infra
}

type ComparisonSection struct {
	BaselinePath            string             `json:"baseline_path"`
	BaselineHash            string             `json:"baseline_hash,omitempty"`
	Notes                   string             `json:"notes"`
	SameDataset             bool               `json:"same_dataset"`
	SameCasesAndTrials      bool               `json:"same_cases_and_trials"`
	AgentTaskSuccessRate    float64            `json:"agent_task_success_rate"`
	HybridTaskSuccessRate   float64            `json:"hybrid_task_success_rate"`
	TaskSuccessDelta        float64            `json:"task_success_delta"`
	AgentCompositePassRate  float64            `json:"agent_composite_pass_rate"`
	HybridCompositePassRate float64            `json:"hybrid_composite_pass_rate"`
	PerTagTaskSuccessDelta  map[string]float64 `json:"per_tag_task_success_delta,omitempty"`
	AgentP50LatencyMs       int64              `json:"agent_p50_latency_ms"`
	HybridP50LatencyMs      int64              `json:"hybrid_p50_latency_ms"`
	AgentP95LatencyMs       int64              `json:"agent_p95_latency_ms"`
	HybridP95LatencyMs      int64              `json:"hybrid_p95_latency_ms"`
	AgentAvgModelCalls      float64            `json:"agent_avg_model_calls"`
	HybridAvgModelCalls     float64            `json:"hybrid_avg_model_calls"`
	AgentAvgToolCalls       float64            `json:"agent_avg_tool_calls"`
	HybridAvgToolCalls      float64            `json:"hybrid_avg_tool_calls"`
	AgentAvgTokens          float64            `json:"agent_avg_tokens"`
	HybridAvgTokens         float64            `json:"hybrid_avg_tokens"`
	AgentEstimatedCostUSD   float64            `json:"agent_estimated_cost_usd_upper_bound"`
	HybridEstimatedCostUSD  float64            `json:"hybrid_estimated_cost_usd_upper_bound"`
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func assertNonStubVersion(v string) error {
	if v == "" || strings.Contains(strings.ToLower(v), "stub") {
		return fmt.Errorf("report version must not be stub: %q", v)
	}
	return nil
}

func buildReport(datasetVer, datasetHash string, exp ExperimentMeta, cases []CaseReport, nInfra int) (AgentEvalReport, error) {
	rep := AgentEvalReport{
		Version:     reportVersion,
		DatasetHash: datasetHash,
		DatasetVer:  datasetVer,
		Experiment:  exp,
		Cases:       cases,
		NInfraError: nInfra,
		NTotal:      len(cases),
	}
	if err := assertNonStubVersion(rep.Version); err != nil {
		return rep, err
	}

	var outOK, gOK, successfulGroundOK, successfulGroundN, tOK, compositeOK, evalN int
	var modelSum, toolSum, tokSum, promptTokSum, completionTokSum float64
	var lats []int64
	consistency := map[string]float64{}
	tagOutcomePass := map[string]int{}
	tagCompositePass := map[string]int{}
	tagN := map[string]int{}
	passAtKOK := 0
	passAtKN := 0

	for _, c := range cases {
		consistency[c.ID] = c.Trials.SuccessRate
		if c.Trials.Trials >= 3 {
			passAtKN++
			if c.Trials.Successes == c.Trials.Trials {
				passAtKOK++
			}
		}
		for _, td := range c.TrialDetails {
			if td.InfraError != "" {
				continue
			}
			evalN++
			if td.Outcome.Pass {
				outOK++
				successfulGroundN++
				if td.Ground.Pass {
					successfulGroundOK++
				}
			}
			if td.Ground.Pass {
				gOK++
			}
			if td.Traj.Pass {
				tOK++
			}
			if td.Pass {
				compositeOK++
			}
			modelSum += float64(td.Actual.ModelCalls)
			toolSum += float64(td.Actual.ToolCalls)
			tokSum += float64(td.Actual.Tokens)
			promptTokSum += float64(td.Actual.PromptTokens)
			completionTokSum += float64(td.Actual.CompletionTokens)
			if td.Actual.LatencyMs > 0 {
				lats = append(lats, td.Actual.LatencyMs)
			}
			for _, tag := range c.Tags {
				tagN[tag]++
				if td.Outcome.Pass {
					tagOutcomePass[tag]++
				}
				if td.Pass {
					tagCompositePass[tag]++
				}
			}
		}
		rep.NEvaluated = evalN
	}

	sum := ReportSummary{TrialConsistency: consistency}
	if evalN > 0 {
		sum.OutcomeRate = float64(outOK) / float64(evalN)
		sum.GroundednessRate = float64(gOK) / float64(evalN)
		sum.TrajectoryPassRate = float64(tOK) / float64(evalN)
		sum.CompositePassRate = float64(compositeOK) / float64(evalN)
		sum.AvgModelCalls = modelSum / float64(evalN)
		sum.AvgToolCalls = toolSum / float64(evalN)
		sum.AvgTokens = tokSum / float64(evalN)
		sum.TotalModelCalls = int(modelSum)
		sum.TotalToolCalls = int(toolSum)
		sum.TotalTokens = int(tokSum)
		sum.TotalPromptTokens = int(promptTokSum)
		sum.TotalCompletionTokens = int(completionTokSum)
		// DeepSeek V4 Flash official prices on 2026-07-29:
		// cache-miss input $0.14/M, output $0.28/M. Counting every input
		// token as a cache miss makes this a conservative upper bound.
		sum.EstimatedCostUSDUpperBound = round6(
			promptTokSum*0.14/1_000_000 + completionTokSum*0.28/1_000_000,
		)
	}
	if successfulGroundN > 0 {
		sum.SuccessfulGroundednessRate = float64(successfulGroundOK) / float64(successfulGroundN)
	}
	if passAtKN > 0 {
		sum.AllTrialsPassRate = float64(passAtKOK) / float64(passAtKN)
	}
	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	sum.P50LatencyMs = percentile(lats, 0.50)
	sum.P95LatencyMs = percentile(lats, 0.95)
	rep.Summary = sum

	tagOutcomeRates := map[string]float64{}
	tagCompositeRates := map[string]float64{}
	for tag, n := range tagN {
		if n > 0 {
			tagOutcomeRates[tag] = float64(tagOutcomePass[tag]) / float64(n)
			tagCompositeRates[tag] = float64(tagCompositePass[tag]) / float64(n)
		}
	}
	rep.TagOutcomeRates = tagOutcomeRates
	rep.TagCompositePassRates = tagCompositeRates
	if totalTrials := evalN + nInfra; totalTrials > 0 {
		rep.InfraErrorRate = float64(nInfra) / float64(totalTrials)
	}
	return rep, nil
}

func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// compareHybridBaseline compares two systems on the same AgentCase tasks.
// It intentionally rejects retrieval reports and any mismatch in dataset,
// case IDs or trial counts.
func compareHybridBaseline(rep *AgentEvalReport, baselinePath string) error {
	raw, err := os.ReadFile(baselinePath)
	if err != nil {
		return fmt.Errorf("read same-task Hybrid RAG baseline: %w", err)
	}
	var base AgentEvalReport
	if err := json.Unmarshal(raw, &base); err != nil {
		return fmt.Errorf("parse baseline: %w", err)
	}
	if base.Experiment.System != "hybrid_rag" {
		return fmt.Errorf("baseline system=%q, want hybrid_rag same-task report", base.Experiment.System)
	}
	if rep.Experiment.System != "agent" {
		return fmt.Errorf("candidate system=%q, want agent", rep.Experiment.System)
	}
	if rep.DatasetHash != base.DatasetHash || rep.DatasetVer != base.DatasetVer {
		return fmt.Errorf("dataset mismatch: agent=%s/%s hybrid=%s/%s",
			rep.DatasetVer, rep.DatasetHash, base.DatasetVer, base.DatasetHash)
	}
	if err := validateSameCasesAndTrials(rep.Cases, base.Cases); err != nil {
		return err
	}
	delta := map[string]float64{}
	for tag, rate := range rep.TagOutcomeRates {
		if baseRate, ok := base.TagOutcomeRates[tag]; ok {
			delta[tag] = round4(rate - baseRate)
		}
	}
	rep.Comparison = &ComparisonSection{
		BaselinePath: baselinePath, BaselineHash: sha256Hex(raw),
		Notes:       "Agent vs Hybrid RAG on identical agent.v2 tasks and trial counts; task success is the deterministic outcome grader, separate from composite pass.",
		SameDataset: true, SameCasesAndTrials: true,
		AgentTaskSuccessRate: rep.Summary.OutcomeRate, HybridTaskSuccessRate: base.Summary.OutcomeRate,
		TaskSuccessDelta:       round4(rep.Summary.OutcomeRate - base.Summary.OutcomeRate),
		AgentCompositePassRate: rep.Summary.CompositePassRate, HybridCompositePassRate: base.Summary.CompositePassRate,
		PerTagTaskSuccessDelta: delta,
		AgentP50LatencyMs:      rep.Summary.P50LatencyMs, HybridP50LatencyMs: base.Summary.P50LatencyMs,
		AgentP95LatencyMs: rep.Summary.P95LatencyMs, HybridP95LatencyMs: base.Summary.P95LatencyMs,
		AgentAvgModelCalls: rep.Summary.AvgModelCalls, HybridAvgModelCalls: base.Summary.AvgModelCalls,
		AgentAvgToolCalls: rep.Summary.AvgToolCalls, HybridAvgToolCalls: base.Summary.AvgToolCalls,
		AgentAvgTokens: rep.Summary.AvgTokens, HybridAvgTokens: base.Summary.AvgTokens,
		AgentEstimatedCostUSD:  rep.Summary.EstimatedCostUSDUpperBound,
		HybridEstimatedCostUSD: base.Summary.EstimatedCostUSDUpperBound,
	}
	return nil
}

func validateSameCasesAndTrials(agentCases, hybridCases []CaseReport) error {
	if len(agentCases) != len(hybridCases) {
		return fmt.Errorf("case count mismatch: agent=%d hybrid=%d", len(agentCases), len(hybridCases))
	}
	base := make(map[string]int, len(hybridCases))
	for _, c := range hybridCases {
		base[c.ID] = c.Trials.Trials
	}
	for _, c := range agentCases {
		trials, ok := base[c.ID]
		if !ok || trials != c.Trials.Trials {
			return fmt.Errorf("case/trial mismatch for %s: agent=%d hybrid=%d exists=%v",
				c.ID, c.Trials.Trials, trials, ok)
		}
	}
	return nil
}

func round4(f float64) float64 {
	return math.Round(f*10000) / 10000
}

func round6(f float64) float64 {
	return math.Round(f*1_000_000) / 1_000_000
}

func writeReport(path string, rep AgentEvalReport) error {
	if err := assertNonStubVersion(rep.Version); err != nil {
		return err
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
