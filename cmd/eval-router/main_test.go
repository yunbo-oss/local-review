package main

import (
	"testing"

	"local-review-go/internal/logic"
)

type fixedRouter struct{ decision logic.RouteDecision }

func (f fixedRouter) Route(logic.RouteInput) logic.RouteDecision { return f.decision }

func TestEvaluateMetrics(t *testing.T) {
	file := caseFile{Version: "router.v1", Cases: []routerCase{
		{ID: "1", Split: "test", ExpectedRoute: "rag_oneshot", Tags: []string{"one"}},
		{ID: "2", Split: "test", ExpectedRoute: "agent_multistep", Tags: []string{"two"}},
		{ID: "3", Split: "dev", ExpectedRoute: "rag_oneshot"},
	}}
	report, err := evaluate(file, []byte("dataset"), "test", fixedRouter{logic.RouteDecision{Route: logic.RouteRAGOneshot}})
	if err != nil {
		t.Fatal(err)
	}
	if report.NTotal != 2 || report.NCorrect != 1 || report.Accuracy != 0.5 {
		t.Fatalf("summary=%+v", report)
	}
	if got := report.PerClass["rag_oneshot"]; got.Precision != 0.5 || got.Recall != 1 {
		t.Fatalf("rag metric=%+v", got)
	}
	if got := report.PerClass["agent_multistep"]; got.Recall != 0 || got.Support != 1 {
		t.Fatalf("agent metric=%+v", got)
	}
	if len(report.Errors) != 1 || report.Errors[0].ID != "2" {
		t.Fatalf("errors=%+v", report.Errors)
	}
}

func TestEvaluateRejectsEmptySplit(t *testing.T) {
	_, err := evaluate(caseFile{Version: "router.v1"}, nil, "test", fixedRouter{})
	if err == nil {
		t.Fatal("expected no-cases error")
	}
}
