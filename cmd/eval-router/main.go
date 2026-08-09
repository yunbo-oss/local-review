// Command eval-router evaluates the production RecommendRouter without an LLM.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"local-review-go/internal/evalmeta"
	"local-review-go/internal/logic"
)

const reportVersion = "router-eval.v1"

type caseFile struct {
	Version string       `json:"version"`
	Cases   []routerCase `json:"cases"`
}

type routerCase struct {
	ID             string   `json:"id"`
	Split          string   `json:"split"`
	Question       string   `json:"question"`
	ForceRoute     string   `json:"force_route,omitempty"`
	HasHistory     bool     `json:"has_history,omitempty"`
	ExpectedRoute  string   `json:"expected_route"`
	ExpectedForced bool     `json:"expected_forced"`
	Tags           []string `json:"tags,omitempty"`
}

type classMetric struct {
	Support   int     `json:"support"`
	Predicted int     `json:"predicted"`
	TruePos   int     `json:"true_positive"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
}

type caseResult struct {
	ID             string   `json:"id"`
	Tags           []string `json:"tags,omitempty"`
	ExpectedRoute  string   `json:"expected_route"`
	ActualRoute    string   `json:"actual_route"`
	ExpectedForced bool     `json:"expected_forced"`
	ActualForced   bool     `json:"actual_forced"`
	Reason         string   `json:"reason"`
	Pass           bool     `json:"pass"`
}

type routerReport struct {
	Runtime         evalmeta.Runtime          `json:"runtime"`
	Version         string                    `json:"version"`
	DatasetVersion  string                    `json:"dataset_version"`
	DatasetHash     string                    `json:"dataset_hash"`
	Split           string                    `json:"split"`
	PolicyVersion   string                    `json:"policy_version"`
	NTotal          int                       `json:"n_total"`
	NCorrect        int                       `json:"n_correct"`
	Accuracy        float64                   `json:"accuracy"`
	InfraErrors     int                       `json:"infrastructure_errors"`
	InfraErrorRate  float64                   `json:"infrastructure_error_rate"`
	PerClass        map[string]classMetric    `json:"per_class"`
	ConfusionMatrix map[string]map[string]int `json:"confusion_matrix"`
	TagAccuracy     map[string]float64        `json:"tag_accuracy"`
	Errors          []caseResult              `json:"errors"`
	Cases           []caseResult              `json:"cases"`
}

func main() {
	testSet := flag.String("test-set", "rag-evals/golden/router.v1.json", "router golden JSON")
	split := flag.String("split", "test", "dataset split")
	out := flag.String("out", "", "optional report output path")
	flag.Parse()

	raw, err := os.ReadFile(*testSet)
	if err != nil {
		fatal(err)
	}
	var file caseFile
	if err := json.Unmarshal(raw, &file); err != nil {
		fatal(fmt.Errorf("parse router golden: %w", err))
	}
	report, err := evaluate(file, raw, strings.TrimSpace(*split), logic.NewRecommendRouter())
	if err != nil {
		fatal(err)
	}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fatal(err)
	}
	b = append(b, '\n')
	if strings.TrimSpace(*out) != "" {
		if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
			fatal(err)
		}
		if err := os.WriteFile(*out, b, 0o644); err != nil {
			fatal(err)
		}
	}
	_, _ = os.Stdout.Write(b)
}

func evaluate(file caseFile, raw []byte, split string, router logic.RecommendRouter) (routerReport, error) {
	if strings.TrimSpace(file.Version) == "" || strings.Contains(strings.ToLower(file.Version), "stub") {
		return routerReport{}, fmt.Errorf("invalid dataset version %q", file.Version)
	}
	if split == "" {
		return routerReport{}, fmt.Errorf("split is required")
	}
	if router == nil {
		return routerReport{}, fmt.Errorf("router is required")
	}
	report := routerReport{
		Runtime: evalmeta.Capture(),
		Version: reportVersion, DatasetVersion: file.Version, DatasetHash: sha256Hex(raw),
		Split: split, PolicyVersion: "recommend-router-rules-v1",
		PerClass: map[string]classMetric{}, ConfusionMatrix: map[string]map[string]int{},
		TagAccuracy: map[string]float64{}, Cases: []caseResult{}, Errors: []caseResult{},
	}
	tagTotal, tagPass := map[string]int{}, map[string]int{}
	for _, c := range file.Cases {
		if c.Split != split {
			continue
		}
		decision := router.Route(logic.RouteInput{
			Question: c.Question, ForceRoute: c.ForceRoute, HasHistory: c.HasHistory,
		})
		actual := string(decision.Route)
		passed := actual == c.ExpectedRoute && decision.Forced == c.ExpectedForced
		result := caseResult{
			ID: c.ID, Tags: c.Tags, ExpectedRoute: c.ExpectedRoute, ActualRoute: actual,
			ExpectedForced: c.ExpectedForced, ActualForced: decision.Forced,
			Reason: decision.Reason, Pass: passed,
		}
		report.NTotal++
		if passed {
			report.NCorrect++
		} else {
			report.Errors = append(report.Errors, result)
		}
		report.Cases = append(report.Cases, result)
		if report.ConfusionMatrix[c.ExpectedRoute] == nil {
			report.ConfusionMatrix[c.ExpectedRoute] = map[string]int{}
		}
		report.ConfusionMatrix[c.ExpectedRoute][actual]++
		for _, tag := range c.Tags {
			tagTotal[tag]++
			if passed {
				tagPass[tag]++
			}
		}
	}
	if report.NTotal == 0 {
		return report, fmt.Errorf("no router cases for split %q", split)
	}
	report.Accuracy = float64(report.NCorrect) / float64(report.NTotal)
	report.InfraErrorRate = 0
	report.PerClass = perClass(report.Cases)
	for tag, total := range tagTotal {
		report.TagAccuracy[tag] = float64(tagPass[tag]) / float64(total)
	}
	sort.Slice(report.Errors, func(i, j int) bool { return report.Errors[i].ID < report.Errors[j].ID })
	return report, nil
}

func perClass(cases []caseResult) map[string]classMetric {
	labels := map[string]struct{}{}
	for _, c := range cases {
		labels[c.ExpectedRoute] = struct{}{}
		labels[c.ActualRoute] = struct{}{}
	}
	out := make(map[string]classMetric, len(labels))
	for label := range labels {
		m := classMetric{}
		for _, c := range cases {
			if c.ExpectedRoute == label {
				m.Support++
			}
			if c.ActualRoute == label {
				m.Predicted++
			}
			if c.ExpectedRoute == label && c.ActualRoute == label && c.ExpectedForced == c.ActualForced {
				m.TruePos++
			}
		}
		if m.Predicted > 0 {
			m.Precision = float64(m.TruePos) / float64(m.Predicted)
		}
		if m.Support > 0 {
			m.Recall = float64(m.TruePos) / float64(m.Support)
		}
		if m.Precision+m.Recall > 0 {
			m.F1 = 2 * m.Precision * m.Recall / (m.Precision + m.Recall)
		}
		out[label] = m
	}
	return out
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
