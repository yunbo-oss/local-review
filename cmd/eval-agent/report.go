package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

const reportVersion = "agent-eval.v1"

// AgentEvalReport 非 stub 评测报告
type AgentEvalReport struct {
	Version      string             `json:"version"`
	DatasetHash  string             `json:"dataset_hash"`
	DatasetVer   string             `json:"dataset_version"`
	Experiment   ExperimentMeta     `json:"experiment"`
	Summary      ReportSummary      `json:"summary"`
	NTotal       int                `json:"n_total"`
	NEvaluated   int                `json:"n_evaluated"`
	NInfraError  int                `json:"n_infra_error"`
	Cases        []CaseReport       `json:"cases"`
	Comparison   *ComparisonSection `json:"comparison,omitempty"`
	TagOutcomes  map[string]float64 `json:"tag_outcome_rates,omitempty"`
}

type ExperimentMeta struct {
	ChatModel       string `json:"chat_model"`
	AgentMaxSteps   int    `json:"agent_max_steps"`
	AgentMaxTools   int    `json:"agent_max_tool_calls"`
	ForceRoute      string `json:"force_route,omitempty"`
	Mode            string `json:"mode"` // inprocess|fake
	PolicyVersion   string `json:"policy_version"`
}

type ReportSummary struct {
	OutcomeRate         float64            `json:"outcome_rate"`
	GroundednessRate    float64            `json:"groundedness_rate"`
	TrajectoryPassRate  float64            `json:"trajectory_pass_rate"`
	TrialConsistency    map[string]float64 `json:"trial_consistency"` // case_id → success_rate；含 pass^k 用 all-pass
	PassAtKRate         float64            `json:"pass_at_k_rate"`     // 全部 trial 通过的 case 占比
	P50LatencyMs        int64              `json:"p50_latency_ms"`
	P95LatencyMs        int64              `json:"p95_latency_ms"`
	AvgToolCalls        float64            `json:"avg_tool_calls"`
	AvgTokens           float64            `json:"avg_tokens"`
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
	TrialIndex int          `json:"trial_index"`
	SessionID  string       `json:"session_id"`
	Route      string       `json:"route,omitempty"`
	TraceID    string       `json:"trace_id,omitempty"`
	Outcome    GradeResult  `json:"outcome"`
	Ground     GradeResult  `json:"groundedness"`
	Traj       GradeResult  `json:"trajectory"`
	Actual     OutcomeActual `json:"actual"`
	InfraError string       `json:"infra_error,omitempty"`
	Pass       bool         `json:"pass"` // 三 grader 全过且无 infra
}

type ComparisonSection struct {
	BaselinePath string             `json:"baseline_path"`
	BaselineHash string             `json:"baseline_hash,omitempty"`
	Notes        string             `json:"notes"`
	PerTagDelta  map[string]float64 `json:"per_tag_outcome_delta,omitempty"` // agent_outcome - baseline_hit_rate
	LatencyNote  string             `json:"latency_note,omitempty"`
	AgentP50Ms   int64              `json:"agent_p50_latency_ms,omitempty"`
	BaselineHit  float64            `json:"baseline_hit_rate_at_k,omitempty"`
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

	var outOK, gOK, tOK, evalN int
	var toolSum, tokSum float64
	var lats []int64
	consistency := map[string]float64{}
	tagPass := map[string]int{}
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
			}
			if td.Ground.Pass {
				gOK++
			}
			if td.Traj.Pass {
				tOK++
			}
			toolSum += float64(td.Actual.ToolCalls)
			tokSum += float64(td.Actual.Tokens)
			if td.Actual.LatencyMs > 0 {
				lats = append(lats, td.Actual.LatencyMs)
			}
			for _, tag := range c.Tags {
				tagN[tag]++
				if td.Pass {
					tagPass[tag]++
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
		sum.AvgToolCalls = toolSum / float64(evalN)
		sum.AvgTokens = tokSum / float64(evalN)
	}
	if passAtKN > 0 {
		sum.PassAtKRate = float64(passAtKOK) / float64(passAtKN)
	}
	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	sum.P50LatencyMs = percentile(lats, 0.50)
	sum.P95LatencyMs = percentile(lats, 0.95)
	rep.Summary = sum

	tagRates := map[string]float64{}
	for tag, n := range tagN {
		if n > 0 {
			tagRates[tag] = float64(tagPass[tag]) / float64(n)
		}
	}
	rep.TagOutcomes = tagRates
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

// compareHybridBaseline Agent vs Hybrid RAG（无 dense 字段）
func compareHybridBaseline(rep *AgentEvalReport, baselinePath string) error {
	raw, err := os.ReadFile(baselinePath)
	if err != nil {
		rep.Comparison = &ComparisonSection{
			BaselinePath: baselinePath,
			Notes:        "baseline missing: " + err.Error() + "; Agent vs Hybrid RAG comparison skipped",
		}
		return nil
	}
	var base struct {
		DatasetSHA256 string  `json:"dataset_sha256"`
		HitRateAtK    float64 `json:"hit_rate_at_k"`
		Retriever     string  `json:"retriever"`
		PerCase       []struct {
			ID      string   `json:"id"`
			HitRate float64  `json:"hit_rate"`
			Tags    []string `json:"tags"`
		} `json:"per_case"`
	}
	if err := json.Unmarshal(raw, &base); err != nil {
		return fmt.Errorf("parse baseline: %w", err)
	}
	if strings.Contains(strings.ToLower(base.Retriever), "dense") && base.Retriever != "hybrid" {
		// 仍允许读文件，但不把 dense 当主对照
	}
	delta := map[string]float64{}
	// 用 Agent tag outcome vs 全局 baseline hit（retrieval 基线无 agent tag 对齐时用整体 HitRate）
	for tag, rate := range rep.TagOutcomes {
		delta[tag] = round4(rate - base.HitRateAtK)
	}
	rep.Comparison = &ComparisonSection{
		BaselinePath: baselinePath,
		BaselineHash: sha256Hex(raw),
		Notes:        "Agent vs Hybrid RAG only (no dense A/B); Δ = agent_tag_outcome_rate - baseline_hit_rate_at_k",
		PerTagDelta:  delta,
		AgentP50Ms:   rep.Summary.P50LatencyMs,
		BaselineHit:  base.HitRateAtK,
		LatencyNote:  "Agent p50 latency in report; Hybrid baseline typically retrieval-only latency not comparable 1:1",
	}
	return nil
}

func round4(f float64) float64 {
	return math.Round(f*10000) / 10000
}

func writeReport(path string, rep AgentEvalReport) error {
	if err := assertNonStubVersion(rep.Version); err != nil {
		return err
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
