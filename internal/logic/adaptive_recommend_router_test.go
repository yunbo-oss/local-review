package logic

import (
	"context"
	"testing"

	"local-review-go/internal/agent"
	"local-review-go/internal/llm"
)

type understanderStub struct {
	spec  agent.IntentSpec
	usage llm.TokenUsage
	err   error
}

func (s understanderStub) Understand(context.Context, agent.QueryUnderstandingInput) (agent.IntentSpec, llm.TokenUsage, error) {
	return s.spec, s.usage, s.err
}

func TestAdaptiveRouterUsesStructuredIntent(t *testing.T) {
	router := NewAdaptiveRecommendRouter(NewRecommendRouter(), understanderStub{spec: agent.IntentSpec{
		Intent: "compare", Route: "agent", Confidence: 0.94, Source: "llm",
	}})
	decision, spec, _, err := router.RouteContext(context.Background(), RouteInput{Question: "两家逐项比较"})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Route != RouteAgent || spec.Intent != "compare" || decision.Reason != "query_understanding:compare" {
		t.Fatalf("decision=%+v spec=%+v", decision, spec)
	}
}

func TestAdaptiveRouterClarifiesMissingHistoryAndFallsBack(t *testing.T) {
	router := NewAdaptiveRecommendRouter(NewRecommendRouter(), understanderStub{spec: agent.IntentSpec{
		Intent: "followup", Route: "agent", Confidence: 0.9, Source: "llm",
	}})
	decision, spec, _, err := router.RouteContext(context.Background(), RouteInput{Question: "还是那家"})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Route != RouteClarify || !spec.NeedClarification {
		t.Fatalf("decision=%+v spec=%+v", decision, spec)
	}

	fallback := NewAdaptiveRecommendRouter(NewRecommendRouter(), understanderStub{err: context.DeadlineExceeded})
	decision, spec, _, err = fallback.RouteContext(context.Background(), RouteInput{Question: "海淀咖啡"})
	if err == nil || decision.Route != RouteRAGOneshot || spec.Source != "fallback" {
		t.Fatalf("decision=%+v spec=%+v err=%v", decision, spec, err)
	}
}

func TestAdaptiveRouterEnforcesMemoryIntentCompatibility(t *testing.T) {
	t.Run("preference update cannot go to stateless RAG", func(t *testing.T) {
		router := NewAdaptiveRecommendRouter(NewRecommendRouter(), understanderStub{spec: agent.IntentSpec{
			Intent: "preference_update", Route: "rag_oneshot", Confidence: 0.91, Source: "llm",
		}})
		decision, spec, _, err := router.RouteContext(context.Background(), RouteInput{Question: "花销按人均300封顶，先记住"})
		if err != nil {
			t.Fatal(err)
		}
		if decision.Route != RouteAgent || spec.Route != string(RouteAgent) {
			t.Fatalf("memory update escaped to stateless route: decision=%+v spec=%+v", decision, spec)
		}
	})

	t.Run("explicit preference language overrides a mistaken search intent", func(t *testing.T) {
		router := NewAdaptiveRecommendRouter(NewRecommendRouter(), understanderStub{spec: agent.IntentSpec{
			Intent: "search", Route: "rag_oneshot", Confidence: 0.93, Source: "llm",
		}})
		decision, spec, _, err := router.RouteContext(context.Background(), RouteInput{Question: "我接下来几次都在东城区活动"})
		if err != nil {
			t.Fatal(err)
		}
		if decision.Route != RouteAgent || spec.Intent != "preference_update" {
			t.Fatalf("explicit preference escaped the stateful path: decision=%+v spec=%+v", decision, spec)
		}
	})

	t.Run("resume request is not another preference update", func(t *testing.T) {
		router := NewAdaptiveRecommendRouter(NewRecommendRouter(), understanderStub{spec: agent.IntentSpec{
			Intent: "preference_update", Route: "agent", Confidence: 0.88, Source: "llm",
		}})
		decision, spec, _, err := router.RouteContext(context.Background(), RouteInput{
			Question: "好了，就按前面那个地点和价位，给出带引用的结论", HasHistory: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if decision.Route != RouteAgent || spec.Intent != "followup" || spec.NeedClarification {
			t.Fatalf("resume request was not normalized: decision=%+v spec=%+v", decision, spec)
		}
	})
}
