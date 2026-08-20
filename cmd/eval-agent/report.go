package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"local-review-go/internal/evalmeta"
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
	NCases                int                `json:"n_cases"`
	NTrials               int                `json:"n_trials"`
	NTotal                int                `json:"n_total"`     // deprecated: same as n_cases
	NEvaluated            int                `json:"n_evaluated"` // non-infrastructure trials
	NInfraError           int                `json:"n_infra_error"`
	InfraErrorRate        float64            `json:"infra_error_rate"`
	Cases                 []CaseReport       `json:"cases"`
	Comparison            *ComparisonSection `json:"comparison,omitempty"`
	TagOutcomeRates       map[string]float64 `json:"tag_outcome_rates,omitempty"`
	TagCompositePassRates map[string]float64 `json:"tag_composite_pass_rates,omitempty"`
}

type ExperimentMeta struct {
	System                string           `json:"system"`               // agent|hybrid_rag
	EntryPoint            string           `json:"entrypoint,omitempty"` // agent_direct|router_e2e
	Split                 string           `json:"split"`
	ChatModel             string           `json:"chat_model"`
	ChatTemperature       float32          `json:"chat_temperature"`
	ThinkingMode          string           `json:"thinking_mode"`
	EmbeddingProvider     string           `json:"embedding_provider"`
	EmbeddingModel        string           `json:"embedding_model"`
	EmbeddingDim          int              `json:"embedding_dim"`
	Retriever             string           `json:"retriever"`
	TopK                  int              `json:"top_k"`
	AgentMaxSteps         int              `json:"agent_max_steps"`
	AgentMaxTools         int              `json:"agent_max_tool_calls"`
	AgentMaxToolAttempts  int              `json:"agent_max_tool_attempts"`
	AgentMaxToolsPerTurn  int              `json:"agent_max_tools_per_turn"`
	AgentRunTimeout       string           `json:"agent_run_timeout"`
	AgentToolTimeout      string           `json:"agent_tool_timeout"`
	ForceRoute            string           `json:"force_route,omitempty"`
	Mode                  string           `json:"mode"` // inprocess|fake
	PolicyVersion         string           `json:"policy_version"`
	AgentRuntimeVersion   string           `json:"agent_runtime_version,omitempty"`
	AgentMaxSearchRounds  int              `json:"agent_max_search_rounds,omitempty"`
	AgentMaxReviewPages   int              `json:"agent_max_review_pages_per_shop,omitempty"`
	InputPriceUSDPerMTok  float64          `json:"input_price_usd_per_million_tokens"`
	OutputPriceUSDPerMTok float64          `json:"output_price_usd_per_million_tokens"`
	Runtime               evalmeta.Runtime `json:"runtime"`
}

type ReportSummary struct {
	OutcomeRate                float64            `json:"outcome_rate"`
	GroundednessRate           float64            `json:"groundedness_rate"`
	SuccessfulGroundednessRate float64            `json:"successful_groundedness_rate"`
	TrajectoryPassRate         float64            `json:"trajectory_pass_rate"`
	CompositePassRate          float64            `json:"composite_pass_rate"`
	TrialMicroTaskSuccessRate  float64            `json:"trial_micro_task_success_rate"`
	TrialMicroCompositeRate    float64            `json:"trial_micro_composite_pass_rate"`
	ScenarioMacroTaskSuccess   float64            `json:"scenario_macro_task_success_rate"`
	ScenarioMacroCompositeRate float64            `json:"scenario_macro_composite_pass_rate"`
	OutcomeWilson95            WilsonInterval     `json:"outcome_wilson_95"`
	GroundednessWilson95       WilsonInterval     `json:"groundedness_wilson_95"`
	CompositePassWilson95      WilsonInterval     `json:"composite_pass_wilson_95"`
	TrialConsistency           map[string]float64 `json:"trial_consistency"`    // case_id → per-trial success_rate
	AllTrialsPassRate          float64            `json:"all_trials_pass_rate"` // ≥3 trials 的 case 全部通过占比
	P50LatencyMs               int64              `json:"p50_latency_ms"`
	P95LatencyMs               int64              `json:"p95_latency_ms"`
	AvgModelCalls              float64            `json:"avg_model_calls"`
	AvgToolCalls               float64            `json:"avg_tool_calls"`
	AvgTokens                  float64            `json:"avg_tokens"`
	AvgIntentConfidence        float64            `json:"avg_intent_confidence"`
	AvgRewriteCount            float64            `json:"avg_rewrite_count"`
	AvgPlanVersions            float64            `json:"avg_plan_versions"`
	AvgReplans                 float64            `json:"avg_replans"`
	PlanFallbackRate           float64            `json:"plan_fallback_rate"`
	ClaimFallbackRate          float64            `json:"claim_fallback_rate"`
	AnswerVerifiedRate         float64            `json:"answer_verified_rate"`
	ClaimEvidenceCoverage      float64            `json:"claim_evidence_coverage"`
	AvgRetrievalConfidence     float64            `json:"avg_retrieval_confidence"`
	RetrievalAbstentionRate    float64            `json:"retrieval_abstention_rate"`
	TotalModelCalls            int                `json:"total_model_calls"`
	TotalToolCalls             int                `json:"total_tool_calls"`
	TotalTokens                int                `json:"total_tokens"`
	TotalPromptTokens          int                `json:"total_prompt_tokens"`
	TotalCompletionTokens      int                `json:"total_completion_tokens"`
	EstimatedCostUSDUpperBound float64            `json:"estimated_cost_usd_upper_bound"`
	RouteCounts                map[string]int     `json:"route_counts,omitempty"`
	RouteTaskSuccessRates      map[string]float64 `json:"route_task_success_rates,omitempty"`
	RuntimeStatusCounts        map[string]int     `json:"runtime_status_counts,omitempty"`
}

// WilsonInterval records both the interval and its effective sample size so
// readers do not mistake a perfect point estimate on a small set for certainty.
type WilsonInterval struct {
	Method          string  `json:"method"`
	ConfidenceLevel float64 `json:"confidence_level"`
	Successes       int     `json:"successes"`
	Trials          int     `json:"trials"`
	Lower           float64 `json:"lower"`
	Upper           float64 `json:"upper"`
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
	BaselinePath                         string             `json:"baseline_path"`
	BaselineHash                         string             `json:"baseline_hash,omitempty"`
	Notes                                string             `json:"notes"`
	SameDataset                          bool               `json:"same_dataset"`
	SameCasesAndTrials                   bool               `json:"same_cases_and_trials"`
	AgentTaskSuccessRate                 float64            `json:"agent_task_success_rate"`
	HybridTaskSuccessRate                float64            `json:"hybrid_task_success_rate"`
	TaskSuccessDelta                     float64            `json:"task_success_delta"`
	AgentCompositePassRate               float64            `json:"agent_composite_pass_rate"`
	HybridCompositePassRate              float64            `json:"hybrid_composite_pass_rate"`
	AgentScenarioMacroTaskSuccessRate    float64            `json:"agent_scenario_macro_task_success_rate"`
	HybridScenarioMacroTaskSuccessRate   float64            `json:"hybrid_scenario_macro_task_success_rate"`
	ScenarioMacroTaskSuccessDelta        float64            `json:"scenario_macro_task_success_delta"`
	AgentScenarioMacroCompositePassRate  float64            `json:"agent_scenario_macro_composite_pass_rate"`
	HybridScenarioMacroCompositePassRate float64            `json:"hybrid_scenario_macro_composite_pass_rate"`
	ScenarioMacroCompositePassDelta      float64            `json:"scenario_macro_composite_pass_delta"`
	PerTagTaskSuccessDelta               map[string]float64 `json:"per_tag_task_success_delta,omitempty"`
	AgentP50LatencyMs                    int64              `json:"agent_p50_latency_ms"`
	HybridP50LatencyMs                   int64              `json:"hybrid_p50_latency_ms"`
	AgentP95LatencyMs                    int64              `json:"agent_p95_latency_ms"`
	HybridP95LatencyMs                   int64              `json:"hybrid_p95_latency_ms"`
	AgentAvgModelCalls                   float64            `json:"agent_avg_model_calls"`
	HybridAvgModelCalls                  float64            `json:"hybrid_avg_model_calls"`
	AgentAvgToolCalls                    float64            `json:"agent_avg_tool_calls"`
	HybridAvgToolCalls                   float64            `json:"hybrid_avg_tool_calls"`
	AgentAvgTokens                       float64            `json:"agent_avg_tokens"`
	HybridAvgTokens                      float64            `json:"hybrid_avg_tokens"`
	AgentEstimatedCostUSD                float64            `json:"agent_estimated_cost_usd_upper_bound"`
	HybridEstimatedCostUSD               float64            `json:"hybrid_estimated_cost_usd_upper_bound"`
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
		NCases:      len(cases),
		NTotal:      len(cases),
	}
	if err := assertNonStubVersion(rep.Version); err != nil {
		return rep, err
	}

	var outOK, gOK, successfulGroundOK, successfulGroundN, tOK, compositeOK, evalN int
	var modelSum, toolSum, tokSum, promptTokSum, completionTokSum float64
	var intentConfidenceSum, rewriteSum, planVersionSum, replanSum float64
	var planFallbackN, claimFallbackN, answerVerifiedN, claimCount, claimsWithEvidence int
	var retrievalConfidenceSum float64
	var retrievalAbstentionN int
	var lats []int64
	consistency := map[string]float64{}
	tagOutcomePass := map[string]int{}
	tagCompositePass := map[string]int{}
	tagN := map[string]int{}
	passAtKOK := 0
	passAtKN := 0
	routeCounts := map[string]int{}
	routeOutcomeOK := map[string]int{}
	runtimeStatusCounts := map[string]int{}

	for _, c := range cases {
		caseTrials := len(c.TrialDetails)
		if caseTrials == 0 {
			caseTrials = c.Trials.Trials
		}
		rep.NTrials += caseTrials
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
			intentConfidenceSum += td.Actual.IntentConfidence
			rewriteSum += float64(td.Actual.RewriteCount)
			planVersionSum += float64(td.Actual.PlanVersions)
			replanSum += float64(td.Actual.Replans)
			if td.Actual.PlanFallback {
				planFallbackN++
			}
			if td.Actual.ClaimFallback {
				claimFallbackN++
			}
			if td.Actual.AnswerVerified {
				answerVerifiedN++
			}
			route := strings.TrimSpace(td.Actual.Route)
			if route == "" {
				route = strings.TrimSpace(td.Route)
			}
			if route == "" {
				route = "unknown"
			}
			routeCounts[route]++
			if td.Outcome.Pass {
				routeOutcomeOK[route]++
			}
			status := strings.TrimSpace(td.Actual.RuntimeStatus)
			if status == "" {
				status = "not_applicable"
			}
			runtimeStatusCounts[status]++
			claimCount += td.Actual.ClaimCount
			claimsWithEvidence += td.Actual.ClaimsWithEvidence
			retrievalConfidenceSum += td.Actual.RetrievalConfidence
			if td.Actual.RetrievalDecision == "abstain" {
				retrievalAbstentionN++
			}
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

	routeOutcomeRates := make(map[string]float64, len(routeCounts))
	for route, count := range routeCounts {
		if count > 0 {
			routeOutcomeRates[route] = float64(routeOutcomeOK[route]) / float64(count)
		}
	}
	sum := ReportSummary{
		TrialConsistency: consistency, RouteCounts: routeCounts,
		RouteTaskSuccessRates: routeOutcomeRates, RuntimeStatusCounts: runtimeStatusCounts,
	}
	if evalN > 0 {
		sum.OutcomeRate = float64(outOK) / float64(evalN)
		sum.GroundednessRate = float64(gOK) / float64(evalN)
		sum.TrajectoryPassRate = float64(tOK) / float64(evalN)
		sum.CompositePassRate = float64(compositeOK) / float64(evalN)
		sum.TrialMicroTaskSuccessRate = sum.OutcomeRate
		sum.TrialMicroCompositeRate = sum.CompositePassRate
		sum.AvgModelCalls = modelSum / float64(evalN)
		sum.AvgToolCalls = toolSum / float64(evalN)
		sum.AvgTokens = tokSum / float64(evalN)
		sum.AvgIntentConfidence = intentConfidenceSum / float64(evalN)
		sum.AvgRewriteCount = rewriteSum / float64(evalN)
		sum.AvgPlanVersions = planVersionSum / float64(evalN)
		sum.AvgReplans = replanSum / float64(evalN)
		sum.PlanFallbackRate = float64(planFallbackN) / float64(evalN)
		sum.ClaimFallbackRate = float64(claimFallbackN) / float64(evalN)
		sum.AnswerVerifiedRate = float64(answerVerifiedN) / float64(evalN)
		sum.AvgRetrievalConfidence = retrievalConfidenceSum / float64(evalN)
		sum.RetrievalAbstentionRate = float64(retrievalAbstentionN) / float64(evalN)
		sum.TotalModelCalls = int(modelSum)
		sum.TotalToolCalls = int(toolSum)
		sum.TotalTokens = int(tokSum)
		sum.TotalPromptTokens = int(promptTokSum)
		sum.TotalCompletionTokens = int(completionTokSum)
		// Counting every input token at the configured cache-miss rate makes
		// this a conservative upper bound for providers with prompt caching.
		sum.EstimatedCostUSDUpperBound = round6(
			promptTokSum*exp.InputPriceUSDPerMTok/1_000_000 + completionTokSum*exp.OutputPriceUSDPerMTok/1_000_000,
		)
	}
	if claimCount > 0 {
		sum.ClaimEvidenceCoverage = float64(claimsWithEvidence) / float64(claimCount)
	}
	sum.OutcomeWilson95 = wilson95(outOK, evalN)
	sum.GroundednessWilson95 = wilson95(gOK, evalN)
	sum.CompositePassWilson95 = wilson95(compositeOK, evalN)
	sum.ScenarioMacroTaskSuccess, sum.ScenarioMacroCompositeRate, _ = scenarioMacroRates(cases)
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

func wilson95(successes, trials int) WilsonInterval {
	interval := WilsonInterval{
		Method: "wilson_score", ConfidenceLevel: 0.95,
		Successes: successes, Trials: trials,
	}
	if trials <= 0 {
		return interval
	}
	const z = 1.959963984540054
	n := float64(trials)
	p := float64(successes) / n
	z2 := z * z
	denominator := 1 + z2/n
	center := (p + z2/(2*n)) / denominator
	margin := z * math.Sqrt((p*(1-p)+z2/(4*n))/n) / denominator
	interval.Lower = math.Max(0, center-margin)
	interval.Upper = math.Min(1, center+margin)
	return interval
}

// scenarioMacroRates gives every scenario equal weight, regardless of how
// many stability trials it has. Infrastructure-error trials are reported by
// the separate infrastructure metric and excluded from these quality rates,
// matching the existing trial-micro denominator.
func scenarioMacroRates(cases []CaseReport) (taskSuccess, compositePass float64, evaluatedCases int) {
	for _, c := range cases {
		outcomeOK, compositeOK, evaluatedTrials := 0, 0, 0
		for _, td := range c.TrialDetails {
			if td.InfraError != "" {
				continue
			}
			evaluatedTrials++
			if td.Outcome.Pass {
				outcomeOK++
			}
			if td.Pass {
				compositeOK++
			}
		}
		if evaluatedTrials > 0 {
			taskSuccess += float64(outcomeOK) / float64(evaluatedTrials)
			compositePass += float64(compositeOK) / float64(evaluatedTrials)
			evaluatedCases++
			continue
		}
		// Older reports may omit trial_details. Their case aggregates are the
		// best available backward-compatible source for comparison.
		if len(c.TrialDetails) == 0 && c.Trials.Trials > 0 {
			taskSuccess += c.OutcomePass
			compositePass += c.Trials.SuccessRate
			evaluatedCases++
		}
	}
	if evaluatedCases > 0 {
		taskSuccess /= float64(evaluatedCases)
		compositePass /= float64(evaluatedCases)
	}
	return taskSuccess, compositePass, evaluatedCases
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
	agentScenarioTask, agentScenarioComposite, _ := scenarioMacroRates(rep.Cases)
	hybridScenarioTask, hybridScenarioComposite, _ := scenarioMacroRates(base.Cases)
	rep.Comparison = &ComparisonSection{
		BaselinePath: baselinePath, BaselineHash: sha256Hex(raw),
		Notes:       "Agent vs Hybrid RAG on identical task datasets and trial counts; trial-micro weights every run equally, scenario-macro weights every scenario equally, and task success remains separate from composite pass.",
		SameDataset: true, SameCasesAndTrials: true,
		AgentTaskSuccessRate: rep.Summary.OutcomeRate, HybridTaskSuccessRate: base.Summary.OutcomeRate,
		TaskSuccessDelta:       round4(rep.Summary.OutcomeRate - base.Summary.OutcomeRate),
		AgentCompositePassRate: rep.Summary.CompositePassRate, HybridCompositePassRate: base.Summary.CompositePassRate,
		AgentScenarioMacroTaskSuccessRate:    agentScenarioTask,
		HybridScenarioMacroTaskSuccessRate:   hybridScenarioTask,
		ScenarioMacroTaskSuccessDelta:        round4(agentScenarioTask - hybridScenarioTask),
		AgentScenarioMacroCompositePassRate:  agentScenarioComposite,
		HybridScenarioMacroCompositePassRate: hybridScenarioComposite,
		ScenarioMacroCompositePassDelta:      round4(agentScenarioComposite - hybridScenarioComposite),
		PerTagTaskSuccessDelta:               delta,
		AgentP50LatencyMs:                    rep.Summary.P50LatencyMs, HybridP50LatencyMs: base.Summary.P50LatencyMs,
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
