package logic

import (
	"context"
	"fmt"
	"strings"

	"local-review-go/internal/agent"
	"local-review-go/internal/llm"
)

const defaultIntentConfidenceThreshold = 0.62

// ContextRecommendRouter extends the deterministic fallback with one bounded
// LLM Query Understanding call. Route remains available for offline rule
// baselines and for callers that cannot perform I/O.
type ContextRecommendRouter interface {
	RecommendRouter
	RouteContext(ctx context.Context, in RouteInput) (RouteDecision, agent.IntentSpec, llm.TokenUsage, error)
}

type adaptiveRecommendRouter struct {
	fallback   RecommendRouter
	understand agent.QueryUnderstander
	threshold  float64
}

func NewAdaptiveRecommendRouter(fallback RecommendRouter, understand agent.QueryUnderstander) ContextRecommendRouter {
	if fallback == nil {
		fallback = NewRecommendRouter()
	}
	return &adaptiveRecommendRouter{
		fallback: fallback, understand: understand, threshold: defaultIntentConfidenceThreshold,
	}
}

func (r *adaptiveRecommendRouter) Route(in RouteInput) RouteDecision {
	return r.fallback.Route(in)
}

func (r *adaptiveRecommendRouter) RouteContext(ctx context.Context, in RouteInput) (RouteDecision, agent.IntentSpec, llm.TokenUsage, error) {
	// Explicit force, empty requests, and structurally invalid requests are
	// deterministic safety decisions and never spend a model call.
	base := r.fallback.Route(in)
	if strings.TrimSpace(in.Question) == "" || r.understand == nil {
		spec := agent.FallbackIntentSpec(in.Question, string(base.Route))
		if base.Forced {
			spec.Source = "forced"
			spec.Confidence = 1
		}
		return base, spec, llm.TokenUsage{}, nil
	}

	spec, usage, err := r.understand.Understand(ctx, agent.QueryUnderstandingInput{
		Question: in.Question, HasHistory: in.HasHistory,
		ProfileSummary: in.ProfileSummary, HistorySummary: in.HistorySummary,
	})
	if err != nil {
		base.Reason = "query_understanding_fallback:" + base.Reason
		return base, agent.FallbackIntentSpec(in.Question, string(base.Route)), usage, err
	}
	if base.Forced {
		// force_route freezes only the route for controlled experiments; Query
		// Understanding still runs so rewrite, filters, evidence needs, and plans
		// are identical to the adaptive production path.
		spec.Route = string(base.Route)
		spec.Source = "llm_forced"
		return base, spec, usage, nil
	}
	// Explicit, non-recommendation preference statements are side effects. The
	// model may occasionally label “接下来都在东城区活动” as a search, but it
	// must still reach the stateful path so the deterministic patch can commit.
	if isPreferenceOnlyUtterance(in.Question) {
		spec.Intent = "preference_update"
		spec.Route = string(RouteAgent)
		spec.NeedClarification = false
	}
	// Query Understanding is probabilistic, but side-effecting preference intents
	// and conversation-resume turns have deterministic routing invariants.
	// Without this guard a valid preference update can be sent to stateless RAG,
	// or “好了，就按前面的条件” can be misread as another update and reach the
	// ReAct controller without a recommendation goal.
	if spec.Intent == "preference_update" && isRecommendationResume(in.Question) {
		spec.Intent = "followup"
		spec.Route = string(RouteAgent)
		spec.NeedClarification = false
	}
	if spec.Intent == "preference_update" {
		spec.Route = string(RouteAgent)
	}
	if spec.Intent == "followup" && in.HasHistory {
		spec.Route = string(RouteAgent)
	}
	if spec.Confidence < r.threshold {
		// Low confidence is actionable ambiguity, not permission to silently pick
		// a route. Preserve a high-confidence deterministic simple route only when
		// the model did not identify a missing reference or contradiction.
		if base.Route == RouteRAGOneshot && !spec.NeedClarification {
			base.Reason = "low_confidence_fast_path"
			return base, spec, usage, nil
		}
		return RouteDecision{
			Route: RouteClarify, Reason: "query_understanding_low_confidence",
			Confidence: spec.Confidence,
		}, spec, usage, nil
	}

	route, ok := parseIntentRoute(spec.Route)
	if !ok {
		base.Reason = "query_understanding_invalid_route:" + base.Reason
		return base, agent.FallbackIntentSpec(in.Question, string(base.Route)), usage,
			fmt.Errorf("invalid understood route %q", spec.Route)
	}
	if spec.NeedClarification {
		route = RouteClarify
	}
	// A historical follow-up without context must not be guessed even if the
	// model accidentally selected the full Agent route.
	if spec.Intent == "followup" && !in.HasHistory {
		route = RouteClarify
		spec.NeedClarification = true
		if strings.TrimSpace(spec.ClarificationQuestion) == "" {
			spec.ClarificationQuestion = "请补充上一轮候选或具体店名。"
		}
	}
	return RouteDecision{
		Route: route, Reason: "query_understanding:" + spec.Intent,
		Confidence: spec.Confidence,
	}, spec, usage, nil
}

func isPreferenceOnlyUtterance(question string) bool {
	q := strings.TrimSpace(question)
	return q != "" && profilePreferenceIntent.MatchString(q) && !recommendRequestIntent.MatchString(q)
}

func isRecommendationResume(question string) bool {
	q := strings.TrimSpace(question)
	for _, marker := range []string{
		"好了", "开始推荐", "现在可以推荐", "就按前面", "按前面那个",
		"最终需求", "给出带引用", "给出结论",
	} {
		if strings.Contains(q, marker) {
			return true
		}
	}
	return false
}

func parseIntentRoute(raw string) (RecommendRoute, bool) {
	switch strings.TrimSpace(raw) {
	case string(RouteRAGOneshot):
		return RouteRAGOneshot, true
	case string(RouteAgent), legacyRouteAgentMultistep, legacyRouteAgentMemory:
		return RouteAgent, true
	case string(RouteClarify):
		return RouteClarify, true
	default:
		return "", false
	}
}
