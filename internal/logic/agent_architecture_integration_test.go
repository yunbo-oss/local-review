package logic

import (
	"context"
	"strings"
	"testing"

	"local-review-go/internal/agent"
	"local-review-go/internal/llm"
	"local-review-go/internal/memory"
	"local-review-go/internal/model"
	repoInterfaces "local-review-go/internal/repository/interface"

	"github.com/sashabaranov/go-openai"
)

type architectureMemory struct {
	profile    memory.Profile
	history    []memory.Message
	summary    memory.SessionSummary
	appended   []memory.Message
	mergeCalls int
	saveCalls  int
	trimCalls  int
	trimKeep   int
}

func (m *architectureMemory) LoadProfile(context.Context, int64) (memory.Profile, error) {
	return m.profile, nil
}
func (m *architectureMemory) MergeProfile(_ context.Context, _ int64, patch memory.ProfilePatch) (memory.Profile, error) {
	m.mergeCalls++
	merged, err := memory.MergeProfile(m.profile, patch)
	if err == nil {
		m.profile = merged
	}
	return merged, err
}
func (m *architectureMemory) ReplaceProfile(_ context.Context, _ int64, profile memory.Profile) error {
	m.profile = profile
	return nil
}
func (m *architectureMemory) LoadSession(context.Context, int64, string, int) ([]memory.Message, error) {
	return append([]memory.Message(nil), m.history...), nil
}
func (m *architectureMemory) AppendSession(_ context.Context, _ int64, _ string, messages ...memory.Message) error {
	m.appended = append(m.appended, messages...)
	m.history = append(m.history, messages...)
	return nil
}
func (m *architectureMemory) LoadSessionSummary(context.Context, int64, string) (memory.SessionSummary, error) {
	return m.summary, nil
}
func (m *architectureMemory) SaveSessionSummary(_ context.Context, _ int64, _ string, summary memory.SessionSummary) error {
	m.saveCalls++
	m.summary = summary
	return nil
}
func (m *architectureMemory) TrimSession(_ context.Context, _ int64, _ string, keepRecent int) error {
	m.trimCalls++
	m.trimKeep = keepRecent
	return nil
}

type architectureSearch struct {
	queries []string
	filter  *repoInterfaces.VectorSearchFilter
	results []repoInterfaces.ShopSearchResult
}

func (s *architectureSearch) Search(context.Context, string, *repoInterfaces.VectorSearchFilter, RetrieverStrategy, int) ([]repoInterfaces.ShopSearchResult, error) {
	return append([]repoInterfaces.ShopSearchResult(nil), s.results...), nil
}
func (s *architectureSearch) SearchWithMeta(_ context.Context, _ string, filter *repoInterfaces.VectorSearchFilter, strategy RetrieverStrategy, _ int, mode SearchMode) (SearchOutcome, error) {
	s.filter = cloneVectorFilter(filter)
	return SearchOutcome{Results: append([]repoInterfaces.ShopSearchResult(nil), s.results...), Strategy: strategy, Mode: mode}, nil
}
func (s *architectureSearch) SearchMultiWithMeta(_ context.Context, queries []string, filter *repoInterfaces.VectorSearchFilter, strategy RetrieverStrategy, _ int, mode SearchMode) (SearchOutcome, error) {
	s.queries = append([]string(nil), queries...)
	s.filter = cloneVectorFilter(filter)
	return SearchOutcome{Results: append([]repoInterfaces.ShopSearchResult(nil), s.results...), Strategy: strategy, Mode: mode}, nil
}

func cloneVectorFilter(filter *repoInterfaces.VectorSearchFilter) *repoInterfaces.VectorSearchFilter {
	if filter == nil {
		return nil
	}
	copy := *filter
	return &copy
}

type architecturePlanner struct{}

func (architecturePlanner) Plan(context.Context, agent.PlanInput) (agent.ExecutionPlan, llm.TokenUsage, error) {
	return agent.ExecutionPlan{Version: 1, Goal: "检索并核验安静体验", Steps: []agent.PlanStep{
		{ID: "search", Action: agent.PlanSearchShops, Query: "海淀区安静写材料咖啡"},
		{ID: "reviews", Action: agent.PlanListReviews, DependsOn: []string{"search"}, ParallelGroup: "evidence", TargetCount: 1},
		{ID: "answer", Action: agent.PlanAnswer, DependsOn: []string{"reviews"}},
	}}, llm.TokenUsage{TotalTokens: 13}, nil
}
func (architecturePlanner) Replan(context.Context, agent.ReplanInput) (agent.ExecutionPlan, llm.TokenUsage, error) {
	return agent.ExecutionPlan{}, llm.TokenUsage{}, nil
}

type architectureReranker struct{}

func (architectureReranker) Rerank(_ context.Context, in RerankInput) (RerankResult, error) {
	chosen := in.Candidates[0]
	return RerankResult{
		Results: []repoInterfaces.ShopSearchResult{chosen},
		Scores:  map[int64]float64{chosen.ShopID: 0.92},
		Reasons: map[int64]string{chosen.ShopID: "评价直接覆盖需求"},
		Usage:   llm.TokenUsage{TotalTokens: 17},
	}, nil
}

type architectureBlogRepo struct{}

func (architectureBlogRepo) Create(context.Context, *model.Blog) (int64, error) { return 0, nil }
func (architectureBlogRepo) ListByUserID(context.Context, int64, int) ([]model.Blog, error) {
	return nil, nil
}
func (architectureBlogRepo) ListHots(context.Context, int) ([]model.Blog, error) { return nil, nil }
func (architectureBlogRepo) GetByID(context.Context, int64) (*model.Blog, error) { return nil, nil }
func (architectureBlogRepo) ListByIDs(context.Context, []int64) ([]model.Blog, error) {
	return nil, nil
}
func (architectureBlogRepo) ListByShopID(_ context.Context, shopID int64, _ int) ([]model.Blog, error) {
	if shopID != 7 {
		return nil, nil
	}
	return []model.Blog{{Id: 701, ShopId: 7, Title: "工作日下午", Content: "环境安静，有插座，适合长时间写材料。"}}, nil
}
func (architectureBlogRepo) IncrLike(context.Context, int64) error { return nil }
func (architectureBlogRepo) DecrLike(context.Context, int64) error { return nil }

type architectureProfileChat struct {
	calls int
}

func (c *architectureProfileChat) ChatStream(context.Context, []openai.ChatCompletionMessage, func(string)) error {
	return nil
}
func (c *architectureProfileChat) ChatComplete(context.Context, []openai.ChatCompletionMessage) (string, error) {
	return `{}`, nil
}
func (c *architectureProfileChat) ChatCompleteWithUsage(context.Context, []openai.ChatCompletionMessage) (string, llm.TokenUsage, error) {
	c.calls++
	return `{}`, llm.TokenUsage{TotalTokens: 7}, nil
}

type architectureClaimChat struct {
	outputs []string
	calls   int
}

func (c *architectureClaimChat) ChatWithTools(context.Context, []llm.ChatMessage, []llm.ToolDefinition) (llm.AssistantTurn, error) {
	return llm.AssistantTurn{}, nil
}
func (c *architectureClaimChat) ChatCompleteTurn(context.Context, []llm.ChatMessage) (llm.AssistantTurn, error) {
	index := c.calls
	if index >= len(c.outputs) {
		index = len(c.outputs) - 1
	}
	c.calls++
	return llm.AssistantTurn{
		Message: llm.ChatMessage{Role: "assistant", Content: c.outputs[index]},
		Usage:   llm.TokenUsage{TotalTokens: 19},
	}, nil
}

type architectureSummarizer struct {
	calls    int
	archived []memory.Message
}

func (s *architectureSummarizer) Summarize(_ context.Context, previous memory.SessionSummary, messages []memory.Message) (memory.SessionSummary, llm.TokenUsage, error) {
	s.calls++
	s.archived = append([]memory.Message(nil), messages...)
	return memory.SessionSummary{Content: "用户此前持续寻找安静工作场所。", ThroughTs: 2, Version: previous.Version + 1}, llm.TokenUsage{TotalTokens: 23}, nil
}

func architectureIntent() agent.IntentSpec {
	return agent.IntentSpec{
		Intent: "search", Route: "agent", Source: "llm", Confidence: 0.94,
		OriginalQuestion:     "海淀区找能安静写材料的咖啡，人均100以内",
		HardFilters:          agent.HardFilterSpec{Area: "海淀区", TypeName: "咖啡", MaxPrice: 100},
		SoftPreferences:      []string{"能安静写材料"},
		EvidenceRequirements: []string{"reviews"},
		RewrittenQueries:     []string{"海淀区适合专注工作的咖啡", "海淀区安静有插座咖啡"},
	}
}

func newArchitectureLogic(mem *architectureMemory, claims *architectureClaimChat, profileChat *architectureProfileChat, summarizer *architectureSummarizer, search *architectureSearch) RecommendAgentLogic {
	baseRouter := NewRecommendRouter()
	return NewRecommendAgentLogic(RecommendAgentLogicDeps{
		ToolChat: claims, ChatClient: profileChat, Memory: mem, Search: search,
		BlogRepo: architectureBlogRepo{}, Router: baseRouter,
		AdaptiveRouter: NewAdaptiveRecommendRouter(baseRouter, understanderStub{
			spec: architectureIntent(), usage: llm.TokenUsage{TotalTokens: 11},
		}),
		Reranker: architectureReranker{}, Planner: architecturePlanner{}, Summarizer: summarizer,
		Config: agent.DefaultRunConfig(),
	})
}

func TestAgentArchitectureEndToEndCommitsOnlyVerifiedResult(t *testing.T) {
	history := make([]memory.Message, 12)
	for i := range history {
		history[i] = memory.Message{Role: "user", Content: "此前对话", Ts: int64(i + 1)}
	}
	mem := &architectureMemory{
		profile: memory.Profile{PreferredAreas: []string{"海淀区"}, Version: 3},
		history: history,
		summary: memory.SessionSummary{Content: "此前在海淀活动。", Version: 1},
	}
	search := &architectureSearch{results: []repoInterfaces.ShopSearchResult{{
		ShopID: 7, Name: "静巷咖啡", Area: "海淀区", TypeName: "咖啡",
		AvgPrice: 68, ShopScore: 46, TextContent: "工作日下午安静，有插座。",
	}}}
	claims := &architectureClaimChat{outputs: []string{`{"no_result":false,"summary":"找到有直接评价证据的候选","recommendations":[{"shop_id":7,"claims":[{"text":"工作日下午环境安静且有插座","field":"","value":null,"evidence_refs":["blog:701"]},{"text":"人均68元","field":"avg_price","value":68,"evidence_refs":["shop:7.avg_price"]}]}]}`}}
	profileChat := &architectureProfileChat{}
	summarizer := &architectureSummarizer{}
	logic := newArchitectureLogic(mem, claims, profileChat, summarizer, search)

	result, err := logic.Recommend(context.Background(), 42, "architecture-success", architectureIntent().OriginalQuestion, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Answer, "[shop:7]") || result.ClaimAnswer == nil || len(result.Plans) != 1 {
		t.Fatalf("verified planned answer missing: %+v", result)
	}
	if result.Retrieval.Decision != RetrievalAccept || result.Retrieval.Confidence < 0.9 || result.Replans != 0 {
		t.Fatalf("retrieval/plan trace missing: %+v", result)
	}
	if len(search.queries) < 3 || search.filter == nil || search.filter.Area != "海淀区" || search.filter.TypeName != "咖啡" || search.filter.MaxPrice != 100 {
		t.Fatalf("multi-query or hard filters lost: queries=%v filter=%+v", search.queries, search.filter)
	}
	if len(mem.appended) != 2 || mem.mergeCalls != 0 || mem.saveCalls != 1 || mem.trimCalls != 1 || mem.trimKeep != memory.DefaultWorkingMessages {
		t.Fatalf("success-only layered memory contract failed: %+v", mem)
	}
	if summarizer.calls != 1 || len(summarizer.archived) != 6 || profileChat.calls != 0 {
		t.Fatalf("post-success model stages not exercised: summarizer=%+v profile_calls=%d", summarizer, profileChat.calls)
	}
	if result.ModelCalls != 5 || result.Usage.TotalTokens != 83 {
		t.Fatalf("complete model cost not accounted: calls=%d usage=%+v", result.ModelCalls, result.Usage)
	}
}

func TestAgentArchitectureRejectsInvalidClaimWithoutMemoryCommit(t *testing.T) {
	mem := &architectureMemory{}
	search := &architectureSearch{results: []repoInterfaces.ShopSearchResult{{
		ShopID: 7, Name: "静巷咖啡", Area: "海淀区", TypeName: "咖啡",
		AvgPrice: 68, ShopScore: 46, TextContent: "工作日下午安静，有插座。",
	}}}
	invalid := `{"no_result":false,"summary":"错误借证据","recommendations":[{"shop_id":7,"claims":[{"text":"适合工作","field":"","value":null,"evidence_refs":["blog:999"]}]}]}`
	claims := &architectureClaimChat{outputs: []string{invalid, invalid}}
	profileChat := &architectureProfileChat{}
	summarizer := &architectureSummarizer{}
	logic := newArchitectureLogic(mem, claims, profileChat, summarizer, search)

	result, err := logic.Recommend(context.Background(), 42, "architecture-failure", architectureIntent().OriginalQuestion, "", nil)
	if err == nil || result.Answer != "" || claims.calls != 2 {
		t.Fatalf("invalid claim should fail closed after one repair: result=%+v err=%v calls=%d", result, err, claims.calls)
	}
	if len(mem.appended) != 0 || mem.mergeCalls != 0 || mem.saveCalls != 0 || mem.trimCalls != 0 || profileChat.calls != 0 || summarizer.calls != 0 {
		t.Fatalf("failed run mutated memory: mem=%+v profile_calls=%d summary_calls=%d", mem, profileChat.calls, summarizer.calls)
	}
}
