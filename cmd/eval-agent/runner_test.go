package main

import (
	"context"
	"testing"

	"local-review-go/internal/agent"
	"local-review-go/internal/llm"
	"local-review-go/internal/logic"
	"local-review-go/internal/memory"
	repoInterfaces "local-review-go/internal/repository/interface"
)

type sequenceRecommendLogic struct {
	results []logic.RecommendResult
	next    int
	onCall  func(int)
}

func (s *sequenceRecommendLogic) Recommend(_ context.Context, _ int64, _, _, _ string, _ func(agent.ToolStatus)) (logic.RecommendResult, error) {
	if s.onCall != nil {
		s.onCall(s.next)
	}
	result := s.results[s.next]
	s.next++
	return result, nil
}

func (s *sequenceRecommendLogic) HasSessionHistory(context.Context, int64, string) (bool, error) {
	return s.next > 0, nil
}

type runnerMemoryStub struct {
	profile memory.Profile
}

func (m *runnerMemoryStub) LoadProfile(context.Context, int64) (memory.Profile, error) {
	return m.profile, nil
}

func (m *runnerMemoryStub) MergeProfile(_ context.Context, _ int64, _ memory.ProfilePatch) (memory.Profile, error) {
	return m.profile, nil
}

func (m *runnerMemoryStub) LoadSession(context.Context, int64, string, int) ([]memory.Message, error) {
	return nil, nil
}

func (m *runnerMemoryStub) AppendSession(context.Context, int64, string, ...memory.Message) error {
	return nil
}

func (m *runnerMemoryStub) ReplaceProfile(_ context.Context, _ int64, profile memory.Profile) error {
	m.profile = profile
	return nil
}

func TestInProcessRunnerAccumulatesUsageAcrossUserTurns(t *testing.T) {
	capturedSearch := &capturingSearch{}
	logicStub := &sequenceRecommendLogic{results: []logic.RecommendResult{
		{
			Answer:             "first [shop:1]",
			Steps:              2,
			ModelCalls:         2,
			ToolCalls:          1,
			ToolNames:          []string{agent.ToolSearchShops},
			DuplicateToolCalls: 1,
			ObservedShopIDs:    []int64{1, 2},
			Usage:              llm.TokenUsage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14},
			TraceID:            "trace-first",
			Route:              "route-first",
		},
		{
			Answer:             "second [shop:3]",
			Steps:              3,
			ModelCalls:         3,
			ToolCalls:          2,
			ToolNames:          []string{agent.ToolSearchShops, agent.ToolListShopBlogs},
			DuplicateToolCalls: 2,
			ObservedShopIDs:    []int64{2, 3},
			Usage:              llm.TokenUsage{PromptTokens: 20, CompletionTokens: 6, TotalTokens: 26},
			TraceID:            "trace-second",
			Route:              "route-second",
		},
	}, onCall: func(turn int) {
		if turn == 0 {
			capturedSearch.captureFilter(&repoInterfaces.VectorSearchFilter{Area: "海淀区", MaxPrice: 80})
			return
		}
		capturedSearch.captureFilter(&repoInterfaces.VectorSearchFilter{Area: "朝阳区"})
	}}
	runner := &InProcessRunner{Logic: logicStub, Memory: &runnerMemoryStub{}, Search: capturedSearch, UserID: 42}
	caseDef := AgentCase{ID: "multi-turn"}
	caseDef.Turns = append(caseDef.Turns,
		struct {
			User string `json:"user"`
		}{User: "first question"},
		struct {
			User string `json:"user"`
		}{User: "second question"},
	)

	td, err := runner.RunTrial(context.Background(), caseDef, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if td.Actual.ModelCalls != 5 || td.Actual.ToolCalls != 3 || td.Actual.DuplicateToolCalls != 3 {
		t.Fatalf("usage calls were not accumulated: %+v", td.Actual)
	}
	if !td.Actual.ToolTraceAvailable || len(td.Actual.ToolNames) != 3 {
		t.Fatalf("tool trace was not accumulated: %+v", td.Actual)
	}
	if td.Actual.PromptTokens != 30 || td.Actual.CompletionTokens != 10 || td.Actual.Tokens != 40 {
		t.Fatalf("first-turn tokens were not counted: %+v", td.Actual)
	}
	if got, want := td.Actual.ObservedShopIDs, []int64{1, 2, 3}; !equalInt64s(got, want) {
		t.Fatalf("observed shops=%v, want stable union %v", got, want)
	}
	if td.Actual.Answer != "second [shop:3]" || td.Actual.Steps != 3 || td.TraceID != "trace-second" || td.Route != "route-second" {
		t.Fatalf("final outcome must come from last turn: %+v", td)
	}
	if td.Actual.Filter["area"] != "朝阳区" {
		t.Fatalf("filter must come from last turn: %+v", td.Actual.Filter)
	}
	if _, carried := td.Actual.Filter["maxPrice"]; carried {
		t.Fatalf("first-turn filter leaked into final outcome: %+v", td.Actual.Filter)
	}
}

func equalInt64s(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
