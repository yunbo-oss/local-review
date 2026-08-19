package main

import (
	"context"
	"strings"
	"testing"

	"local-review-go/internal/agent"
	"local-review-go/internal/llm"
	"local-review-go/internal/logic"
	"local-review-go/internal/memory"
	repoInterfaces "local-review-go/internal/repository/interface"

	"github.com/sashabaranov/go-openai"
)

type sequenceRecommendLogic struct {
	results     []logic.RecommendResult
	next        int
	onCall      func(int)
	onRecommend func(context.Context, string)
}

func (s *sequenceRecommendLogic) Recommend(ctx context.Context, _ int64, _, _, forceRoute string, _ func(agent.ToolStatus)) (logic.RecommendResult, error) {
	if s.onRecommend != nil {
		s.onRecommend(ctx, forceRoute)
	}
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
	session []memory.Message
}

func (m *runnerMemoryStub) LoadProfile(context.Context, int64) (memory.Profile, error) {
	return m.profile, nil
}

func (m *runnerMemoryStub) MergeProfile(_ context.Context, _ int64, _ memory.ProfilePatch) (memory.Profile, error) {
	return m.profile, nil
}

func (m *runnerMemoryStub) LoadSession(context.Context, int64, string, int) ([]memory.Message, error) {
	return append([]memory.Message(nil), m.session...), nil
}

func (m *runnerMemoryStub) AppendSession(_ context.Context, _ int64, _ string, messages ...memory.Message) error {
	m.session = append(m.session, messages...)
	return nil
}

func (m *runnerMemoryStub) ReplaceProfile(_ context.Context, _ int64, profile memory.Profile) error {
	m.profile = profile
	return nil
}

type staticContextRouter struct {
	decision logic.RouteDecision
	spec     agent.IntentSpec
	usage    llm.TokenUsage
	err      error
}

type routedSearchStub struct {
	results []repoInterfaces.ShopSearchResult
}

func (s routedSearchStub) Search(context.Context, string, *repoInterfaces.VectorSearchFilter, logic.RetrieverStrategy, int) ([]repoInterfaces.ShopSearchResult, error) {
	return append([]repoInterfaces.ShopSearchResult(nil), s.results...), nil
}

func (s routedSearchStub) SearchWithMeta(ctx context.Context, query string, filter *repoInterfaces.VectorSearchFilter, strategy logic.RetrieverStrategy, topK int, mode logic.SearchMode) (logic.SearchOutcome, error) {
	results, err := s.Search(ctx, query, filter, strategy, topK)
	return logic.SearchOutcome{Results: results, Strategy: strategy, Mode: mode}, err
}

type routedChatStub struct {
	answer string
	usage  llm.TokenUsage
}

func (s routedChatStub) ChatStream(_ context.Context, _ []openai.ChatCompletionMessage, onChunk func(string)) error {
	onChunk(s.answer)
	return nil
}

func (s routedChatStub) ChatComplete(context.Context, []openai.ChatCompletionMessage) (string, error) {
	return s.answer, nil
}

func (s routedChatStub) ChatCompleteWithUsage(context.Context, []openai.ChatCompletionMessage) (string, llm.TokenUsage, error) {
	return s.answer, s.usage, nil
}

func (r staticContextRouter) Route(logic.RouteInput) logic.RouteDecision {
	return r.decision
}

func (r staticContextRouter) RouteContext(context.Context, logic.RouteInput) (logic.RouteDecision, agent.IntentSpec, llm.TokenUsage, error) {
	return r.decision, r.spec, r.usage, r.err
}

func TestInProcessRunnerRouterE2EClarifiesWithoutCallingAgent(t *testing.T) {
	logicStub := &sequenceRecommendLogic{results: []logic.RecommendResult{{Answer: "must not run"}}}
	runner := &InProcessRunner{
		Logic: logicStub, Memory: &runnerMemoryStub{}, UserID: 42,
		Router: staticContextRouter{
			decision: logic.RouteDecision{Route: logic.RouteClarify, Reason: "low confidence"},
			spec: agent.IntentSpec{
				Intent: "followup", Route: string(logic.RouteClarify), Confidence: 0.41,
				ClarificationQuestion: "请补充上一轮候选。", Source: "llm",
			},
			usage: llm.TokenUsage{PromptTokens: 11, CompletionTokens: 3, TotalTokens: 14},
		},
	}
	caseDef := AgentCase{ID: "router-clarify"}
	caseDef.Turns = append(caseDef.Turns, struct {
		User string `json:"user"`
	}{User: "还是那家"})

	td, err := runner.RunTrial(context.Background(), caseDef, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if logicStub.next != 0 {
		t.Fatalf("clarify route called Agent %d times", logicStub.next)
	}
	if td.Route != string(logic.RouteClarify) || td.Actual.RuntimeStatus != string(agent.RuntimeNeedsClarify) || !td.Actual.AnswerVerified {
		t.Fatalf("unexpected clarify terminal result: %+v", td)
	}
	if td.Actual.ModelCalls != 1 || td.Actual.Tokens != 14 || !strings.Contains(td.Actual.Answer, "请补充上一轮候选") {
		t.Fatalf("router usage/answer not captured: %+v", td.Actual)
	}
}

func TestInProcessRunnerRouterE2EForwardsIntentToSelectedAgentRoute(t *testing.T) {
	spec := agent.IntentSpec{
		Intent: "compare", Route: string(logic.RouteAgentMultistep), Confidence: 0.91,
		RewrittenQueries: []string{"海淀 咖啡 对比"}, Source: "llm",
	}
	var gotRoute string
	var gotSpec agent.IntentSpec
	logicStub := &sequenceRecommendLogic{
		results: []logic.RecommendResult{{
			Answer: "推荐结果：[shop:7]", Intent: spec, Route: string(logic.RouteAgentMultistep),
			ObservedShopIDs: []int64{7}, ModelCalls: 3,
		}},
		onRecommend: func(ctx context.Context, route string) {
			gotRoute = route
			gotSpec, _ = agent.IntentSpecFromContext(ctx)
		},
	}
	runner := &InProcessRunner{
		Logic: logicStub, Memory: &runnerMemoryStub{}, UserID: 42,
		Router: staticContextRouter{
			decision: logic.RouteDecision{Route: logic.RouteAgentMultistep, Reason: "comparison"},
			spec:     spec, usage: llm.TokenUsage{PromptTokens: 8, CompletionTokens: 2, TotalTokens: 10},
		},
	}
	caseDef := AgentCase{ID: "router-agent"}
	caseDef.Turns = append(caseDef.Turns, struct {
		User string `json:"user"`
	}{User: "对比两家"})

	td, err := runner.RunTrial(context.Background(), caseDef, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if gotRoute != string(logic.RouteAgentMultistep) || gotSpec.Intent != spec.Intent {
		t.Fatalf("router output was not forwarded: route=%q spec=%+v", gotRoute, gotSpec)
	}
	// The production Agent result includes the shared Query Understanding call;
	// the runner must not add that usage a second time on Agent routes.
	if td.Actual.ModelCalls != 3 || td.Actual.Tokens != 0 {
		t.Fatalf("router usage was double-counted outside Agent logic: %+v", td.Actual)
	}
}

func TestHybridRAGRunnerFailsClosedWhenNoRetrievedRecommendationRemains(t *testing.T) {
	runner := &HybridRAGRunner{
		Search: routedSearchStub{results: []repoInterfaces.ShopSearchResult{{
			ShopID: 7, Name: "七号咖啡", Area: "海淀区", TypeName: "咖啡", AvgPrice: 66, ShopScore: 46,
		}}},
		Chat: routedChatStub{answer: "推荐结果：[shop:999]\n这家最好", usage: llm.TokenUsage{TotalTokens: 9}},
	}
	result, err := runner.RunRoutedTurn(context.Background(), "海淀咖啡", memory.Profile{}, agent.FallbackIntentSpec("海淀咖啡", "rag_oneshot"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.AnswerVerified || !result.Degraded || !strings.HasPrefix(result.Answer, "推荐结果：无") {
		t.Fatalf("hallucinated citation was not safely repaired: %+v", result)
	}
	if cited := agent.ParseCitedShopIDs(result.Answer); len(cited) != 0 {
		t.Fatalf("repaired answer still cites unknown shops: %v", cited)
	}
}

func TestInProcessRunnerPersistsRoutedRAGTurnForLaterFollowup(t *testing.T) {
	mem := &runnerMemoryStub{}
	logicStub := &sequenceRecommendLogic{results: []logic.RecommendResult{{Answer: "must not run"}}}
	runner := &InProcessRunner{
		Logic: logicStub, Memory: mem, UserID: 42,
		Router: staticContextRouter{
			decision: logic.RouteDecision{Route: logic.RouteRAGOneshot, Reason: "simple search"},
			spec:     agent.FallbackIntentSpec("海淀咖啡", string(logic.RouteRAGOneshot)),
		},
		RAG: &HybridRAGRunner{
			Search: routedSearchStub{results: []repoInterfaces.ShopSearchResult{{
				ShopID: 7, Name: "七号咖啡", Area: "海淀区", TypeName: "咖啡", AvgPrice: 66, ShopScore: 46,
			}}},
			Chat: routedChatStub{answer: "推荐结果：[shop:7]\n可以考虑这家。"},
		},
	}
	caseDef := AgentCase{ID: "rag-session"}
	caseDef.Turns = append(caseDef.Turns, struct {
		User string `json:"user"`
	}{User: "海淀咖啡"})

	if _, err := runner.RunTrial(context.Background(), caseDef, 0, ""); err != nil {
		t.Fatal(err)
	}
	if len(mem.session) != 2 || mem.session[0].Role != "user" || mem.session[1].Role != "assistant" ||
		!strings.Contains(mem.session[1].Content, "[shop:7]") {
		t.Fatalf("RAG turn was not persisted for follow-up: %+v", mem.session)
	}
}

func TestRepairRAGAnswerPreservesLegalModelSelectionUsingTypedFacts(t *testing.T) {
	shops := []repoInterfaces.ShopSearchResult{{
		ShopID: 7, Name: "七号咖啡", Area: "海淀区", TypeName: "咖啡", AvgPrice: 66, ShopScore: 46,
	}}
	ledger := agent.NewEvidenceLedger()
	ledger.DiscoverFromSearch(7, "七号咖啡", map[string]any{
		"area": "海淀区", "type_name": "咖啡", "avg_price": int64(66), "score": 46,
	})
	got := repairRAGAnswer("推荐结果：[shop:7]\n评分9.9", shops, ledger, false)
	if !strings.HasPrefix(got, "推荐结果：[shop:7]") || strings.Contains(got, "9.9") {
		t.Fatalf("repair did not preserve the legal selection and discard bad prose: %q", got)
	}
	if err := agent.VerifyAnswer(got, ledger, agent.VerifyOptions{}); err != nil {
		t.Fatalf("rebuilt typed answer must verify: %v", err)
	}
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
			Answer:  "second [shop:3]",
			Intent:  agent.IntentSpec{Intent: "compare", Confidence: 0.88, Source: "llm", RewrittenQueries: []string{"改写一", "改写二"}},
			Plans:   []agent.ExecutionPlan{{Version: 1}, {Version: 2}},
			Replans: 1,
			ClaimAnswer: &agent.ClaimAnswer{Recommendations: []agent.ClaimedRecommendation{{ShopID: 3, Claims: []agent.EvidenceClaim{
				{Text: "有依据", EvidenceRefs: []string{"blog:3"}},
			}}}},
			Retrieval:          logic.RetrievalAssessment{Confidence: 0.77, Decision: logic.RetrievalVerify, EvidenceCoverage: 0.5},
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
	if td.Actual.Intent != "compare" || td.Actual.RewriteCount != 2 || td.Actual.PlanVersions != 2 || td.Actual.Replans != 1 ||
		td.Actual.ClaimEvidenceCoverage != 1 || td.Actual.RetrievalDecision != "verify" || td.Actual.RetrievalConfidence != 0.77 {
		t.Fatalf("adaptive architecture metrics were not captured: %+v", td.Actual)
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

func TestIsInfraErrKeepsBoundedTimeoutInQualityDenominator(t *testing.T) {
	if isInfraErr("context deadline exceeded: agent run timeout") {
		t.Fatal("bounded runtime timeout must be an evaluated reliability failure")
	}
	if !isInfraErr("redis connection refused") {
		t.Fatal("dependency outage must remain an infrastructure error")
	}
}
