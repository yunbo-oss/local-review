package logic

import (
	"context"
	"testing"

	"local-review-go/internal/agent"
	"local-review-go/internal/llm"
	"local-review-go/internal/memory"
	repoInterfaces "local-review-go/internal/repository/interface"
)

type captureSearch struct {
	lastFilter *repoInterfaces.VectorSearchFilter
}

func (c *captureSearch) Search(ctx context.Context, query string, filter *repoInterfaces.VectorSearchFilter, strategy RetrieverStrategy, topK int) ([]repoInterfaces.ShopSearchResult, error) {
	c.lastFilter = filter
	return nil, nil
}

func (c *captureSearch) SearchWithMeta(ctx context.Context, query string, filter *repoInterfaces.VectorSearchFilter, strategy RetrieverStrategy, topK int, mode SearchMode) (SearchOutcome, error) {
	c.lastFilter = filter
	return SearchOutcome{Results: nil, Strategy: strategy, Mode: mode}, nil
}

type memStub struct {
	prof memory.Profile
}

func (m *memStub) LoadProfile(ctx context.Context, userID int64) (memory.Profile, error) {
	return m.prof, nil
}
func (m *memStub) MergeProfile(ctx context.Context, userID int64, patch memory.ProfilePatch) (memory.Profile, error) {
	return m.prof, nil
}
func (m *memStub) ReplaceProfile(ctx context.Context, userID int64, profile memory.Profile) error {
	return nil
}
func (m *memStub) LoadSession(ctx context.Context, userID int64, sessionID string, limit int) ([]memory.Message, error) {
	return nil, nil
}
func (m *memStub) AppendSession(ctx context.Context, userID int64, sessionID string, messages ...memory.Message) error {
	return nil
}

type toolChatOnce struct {
	turn llm.AssistantTurn
}

func (t *toolChatOnce) ChatWithTools(ctx context.Context, messages []llm.ChatMessage, tools []llm.ToolDefinition) (llm.AssistantTurn, error) {
	return t.turn, nil
}
func (t *toolChatOnce) ChatCompleteTurn(ctx context.Context, messages []llm.ChatMessage) (llm.AssistantTurn, error) {
	return t.turn, nil
}

func TestRecommendAgentLogic_RequireConfigAndIDs(t *testing.T) {
	l := NewRecommendAgentLogic(RecommendAgentLogicDeps{})
	_, err := l.Recommend(context.Background(), 1, "s", "q", "", nil)
	if err == nil {
		t.Fatal("want config error")
	}
	l = NewRecommendAgentLogic(RecommendAgentLogicDeps{
		ToolChat: &toolChatOnce{}, Memory: &memStub{}, Search: &captureSearch{},
		Config: agent.DefaultRunConfig(),
	})
	_, err = l.Recommend(context.Background(), 1, "", "q", "", nil)
	if err == nil {
		t.Fatal("want session required")
	}
}

func TestRecommendAgentLogic_GroundingFailWithoutEvidence(t *testing.T) {
	tc := &toolChatOnce{turn: llm.AssistantTurn{
		Message: llm.ChatMessage{Role: "assistant", Content: "推荐这家 [shop:1]"},
	}}
	l := NewRecommendAgentLogic(RecommendAgentLogicDeps{
		ToolChat: tc, Memory: &memStub{}, Search: &captureSearch{},
		Config: agent.DefaultRunConfig(),
	})
	_, err := l.Recommend(context.Background(), 1, "s1", "海淀咖啡", "", nil)
	if err == nil {
		t.Fatal("want grounding failure without evidence")
	}
}

func TestShopSearchAdapter_ProfileFillEmpty(t *testing.T) {
	t.Parallel()
	cap := &captureSearch{}
	budget := int64(100)
	a := &shopSearchAdapter{
		inner:   cap,
		profile: memory.Profile{PreferredAreas: []string{"海淀"}, BudgetMax: &budget},
	}
	_, _ = a.SearchShops(context.Background(), "咖啡", "", "", nil, 5)
	if cap.lastFilter == nil || cap.lastFilter.Area != "海淀" {
		t.Fatalf("want area 海淀, got %+v", cap.lastFilter)
	}
	if cap.lastFilter.MaxPrice != 100 {
		t.Fatalf("want max_price 100, got %+v", cap.lastFilter.MaxPrice)
	}
}

func TestShopSearchAdapter_ExplicitOverridesProfile(t *testing.T) {
	t.Parallel()
	cap := &captureSearch{}
	budget := int64(100)
	a := &shopSearchAdapter{
		inner:   cap,
		profile: memory.Profile{PreferredAreas: []string{"海淀"}, BudgetMax: &budget},
	}
	mp := int64(200)
	_, _ = a.SearchShops(context.Background(), "咖啡", "朝阳", "", &mp, 5)
	if cap.lastFilter == nil || cap.lastFilter.Area != "朝阳" {
		t.Fatalf("want 朝阳, got %+v", cap.lastFilter)
	}
	if cap.lastFilter.MaxPrice != 200 {
		t.Fatalf("want explicit 200, got %d", cap.lastFilter.MaxPrice)
	}
}
