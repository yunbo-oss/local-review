package logic

import (
	"context"
	"strings"
	"testing"

	"local-review-go/internal/agent"
	"local-review-go/internal/llm"
	"local-review-go/internal/memory"
	repoInterfaces "local-review-go/internal/repository/interface"
)

type captureSearch struct {
	lastFilter *repoInterfaces.VectorSearchFilter
	lastTopK   int
	results    []repoInterfaces.ShopSearchResult
}

func (c *captureSearch) Search(ctx context.Context, query string, filter *repoInterfaces.VectorSearchFilter, strategy RetrieverStrategy, topK int) ([]repoInterfaces.ShopSearchResult, error) {
	c.lastFilter = filter
	return nil, nil
}

func (c *captureSearch) SearchWithMeta(ctx context.Context, query string, filter *repoInterfaces.VectorSearchFilter, strategy RetrieverStrategy, topK int, mode SearchMode) (SearchOutcome, error) {
	c.lastFilter = filter
	c.lastTopK = topK
	return SearchOutcome{Results: c.results, Strategy: strategy, Mode: mode}, nil
}

type memStub struct {
	prof    memory.Profile
	history []memory.Message
}

func (m *memStub) LoadProfile(ctx context.Context, userID int64) (memory.Profile, error) {
	return m.prof, nil
}
func (m *memStub) MergeProfile(ctx context.Context, userID int64, patch memory.ProfilePatch) (memory.Profile, error) {
	merged, err := memory.MergeProfile(m.prof, patch)
	if err == nil {
		m.prof = merged
	}
	return merged, err
}
func (m *memStub) ReplaceProfile(ctx context.Context, userID int64, profile memory.Profile) error {
	return nil
}
func (m *memStub) LoadSession(ctx context.Context, userID int64, sessionID string, limit int) ([]memory.Message, error) {
	if limit > 0 && len(m.history) > limit {
		return append([]memory.Message(nil), m.history[len(m.history)-limit:]...), nil
	}
	return append([]memory.Message(nil), m.history...), nil
}

func TestRecommendationRouteInputIncludesProfileAndHistorySummaries(t *testing.T) {
	budget := int64(180)
	mem := &memStub{
		prof: memory.Profile{PreferredAreas: []string{"海淀区"}, BudgetMax: &budget},
		history: []memory.Message{
			{Role: "user", Content: "上次想找能安静写材料的店"},
			{Role: "assistant", Content: "我列了两个候选"},
		},
	}
	l := NewRecommendAgentLogic(RecommendAgentLogicDeps{Memory: mem})
	provider, ok := l.(RecommendRouteContextProvider)
	if !ok {
		t.Fatal("recommend logic must expose layered route context")
	}
	got, err := provider.RecommendationRouteInput(context.Background(), 1, "s", "还是前一个", "")
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasHistory || !strings.Contains(got.ProfileSummary, "海淀区") || !strings.Contains(got.HistorySummary, "安静写材料") {
		t.Fatalf("route context lost memory: %+v", got)
	}
}
func (m *memStub) AppendSession(ctx context.Context, userID int64, sessionID string, messages ...memory.Message) error {
	return nil
}

type toolChatOnce struct {
	turn llm.AssistantTurn
}

type layeredMemStub struct {
	*memStub
	saved memory.SessionSummary
	keep  int
}

func (m *layeredMemStub) LoadSessionSummary(context.Context, int64, string) (memory.SessionSummary, error) {
	return m.saved, nil
}
func (m *layeredMemStub) SaveSessionSummary(_ context.Context, _ int64, _ string, summary memory.SessionSummary) error {
	m.saved = summary
	return nil
}
func (m *layeredMemStub) TrimSession(_ context.Context, _ int64, _ string, keep int) error {
	m.keep = keep
	return nil
}

type summaryCapture struct {
	messages []memory.Message
}

func (s *summaryCapture) Summarize(_ context.Context, previous memory.SessionSummary, messages []memory.Message) (memory.SessionSummary, llm.TokenUsage, error) {
	s.messages = append([]memory.Message(nil), messages...)
	return memory.SessionSummary{Content: "摘要", ThroughTs: 1, Version: previous.Version + 1}, llm.TokenUsage{TotalTokens: 10}, nil
}

func TestCompactSessionKeepsUnsummarisedWorkingWindowWithoutTimestampLoss(t *testing.T) {
	mem := &layeredMemStub{memStub: &memStub{}}
	summarizer := &summaryCapture{}
	l := &recommendAgentLogic{memory: mem, summarizer: summarizer}
	history := make([]memory.Message, 12)
	for i := range history {
		history[i] = memory.Message{Role: "user", Content: "历史", Ts: 1}
	}
	usage, calls := l.compactSessionAfterSuccess(context.Background(), 1, "s",
		memory.SessionSummary{Content: "旧摘要", ThroughTs: 1, Version: 1}, history,
		memory.Message{Role: "user", Content: "当前问题", Ts: 1},
		memory.Message{Role: "assistant", Content: "当前回答", Ts: 1},
	)
	if calls != 1 || usage.TotalTokens != 10 || len(summarizer.messages) != 6 {
		t.Fatalf("compaction did not archive the exact prefix: calls=%d usage=%+v archived=%d", calls, usage, len(summarizer.messages))
	}
	if mem.keep != memory.DefaultWorkingMessages {
		t.Fatalf("trim keeps %d messages, want %d", mem.keep, memory.DefaultWorkingMessages)
	}
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

func TestRecommendAgentLogic_PureProfileUpdateUsesDeterministicFastPath(t *testing.T) {
	mem := &memStub{}
	l := NewRecommendAgentLogic(RecommendAgentLogicDeps{
		ToolChat: &toolChatOnce{}, Memory: mem, Search: &captureSearch{},
		Config: agent.DefaultRunConfig(),
	})
	res, err := l.Recommend(context.Background(), 1, "profile-fast-path", "我接下来几次都在东城区活动", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.ModelCalls != 0 || len(res.ProfileAfter.PreferredAreas) != 1 || res.ProfileAfter.PreferredAreas[0] != "东城区" {
		t.Fatalf("result=%+v", res)
	}
	if res.RuntimeVersion != agent.RuntimeVersionV1Plan || res.RuntimeStatus != agent.RuntimeCompleted || !res.AnswerVerified {
		t.Fatalf("profile fast path must be an auditable verified terminal result: %+v", res)
	}
}

func TestRecommendAgentLogic_ClarificationRouteDoesNotRunTools(t *testing.T) {
	search := &captureSearch{}
	understander := understanderStub{spec: agent.IntentSpec{
		Intent: "clarify", Route: "clarify", NeedClarification: true,
		ClarificationQuestion: "你说的是上一轮哪一家？", Confidence: 0.9,
	}}
	l := NewRecommendAgentLogic(RecommendAgentLogicDeps{
		ToolChat: &toolChatOnce{}, Memory: &memStub{}, Search: search,
		Router:         NewRecommendRouter(),
		AdaptiveRouter: NewAdaptiveRecommendRouter(NewRecommendRouter(), understander),
		Config:         agent.DefaultRunConfig(),
	})
	res, err := l.Recommend(context.Background(), 1, "clarify-direct", "还是那家", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Route != string(RouteClarify) || !strings.Contains(res.Answer, "上一轮哪一家") || search.lastTopK != 0 {
		t.Fatalf("clarification should be a no-tool terminal response: %+v", res)
	}
	if res.RuntimeVersion != agent.RuntimeVersionV1Plan || res.RuntimeStatus != agent.RuntimeNeedsClarify || !res.AnswerVerified {
		t.Fatalf("clarification must expose terminal runtime metadata: %+v", res)
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
	_, _ = a.SearchShops(context.Background(), "咖啡", "", "", nil, nil, 5)
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
	_, _ = a.SearchShops(context.Background(), "咖啡", "朝阳", "", &mp, nil, 5)
	if cap.lastFilter == nil || cap.lastFilter.Area != "朝阳区" {
		t.Fatalf("want 朝阳区, got %+v", cap.lastFilter)
	}
	if cap.lastFilter.MaxPrice != 200 {
		t.Fatalf("want explicit 200, got %d", cap.lastFilter.MaxPrice)
	}
}

func TestShopSearchAdapter_NormalizesAreaAndTypeAliases(t *testing.T) {
	t.Parallel()
	cap := &captureSearch{}
	a := &shopSearchAdapter{inner: cap}
	_, _ = a.SearchShops(context.Background(), "安静办公", "朝阳", "咖啡厅", nil, nil, 5)
	if cap.lastFilter == nil || cap.lastFilter.Area != "朝阳区" || cap.lastFilter.TypeName != "咖啡" {
		t.Fatalf("filter=%+v", cap.lastFilter)
	}
}

func TestShopSearchAdapter_SemanticEvidenceReranksBeforeTruncation(t *testing.T) {
	t.Parallel()
	cap := &captureSearch{results: []repoInterfaces.ShopSearchResult{
		{ShopID: 1, Name: "普通候选", TextContent: "环境整洁，服务正常"},
		{ShopID: 2, Name: "证据候选", TextContent: "有儿童椅，适合带孩子家庭聚餐"},
		{ShopID: 3, Name: "另一个普通候选", TextContent: "交通方便"},
	}}
	required := agent.RequiredSemanticConcepts("适合家庭聚餐并照顾孩子")
	a := &shopSearchAdapter{
		inner: cap, requiredSemantics: required, semanticRerank: true,
	}
	got, err := a.SearchShops(context.Background(), "家庭聚餐", "", "", nil, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if cap.lastTopK != 20 {
		t.Fatalf("semantic candidate pool topK=%d, want 20", cap.lastTopK)
	}
	if len(got) != 2 || got[0].ShopID != 2 {
		t.Fatalf("semantic evidence should rank first: %+v", got)
	}
}

func TestInferExplicitFilter(t *testing.T) {
	t.Parallel()
	got := inferExplicitFilter("朝阳区找安静办公的咖啡，人均50以内")
	if got == nil || got.Area != "朝阳区" || got.TypeName != "咖啡" || got.MaxPrice != 50 {
		t.Fatalf("unexpected filter: %+v", got)
	}
	got = inferExplicitFilter("推荐东城区适合约会的餐厅，预算 180 元")
	if got == nil || got.Area != "东城区" || got.TypeName != "美食" || got.MaxPrice != 180 {
		t.Fatalf("unexpected restaurant filter: %+v", got)
	}
	got = inferExplicitFilter("改成海淀区，预算50，不要沿用朝阳区")
	if got == nil || got.Area != "海淀区" || got.MaxPrice != 50 {
		t.Fatalf("correction chose denied area: %+v", got)
	}
}

func TestInferExplicitFilter_NormalizesTyposAndChineseBudget(t *testing.T) {
	t.Parallel()
	got := inferExplicitFilter("海定区，洒店，三百二以内。来一个")
	if got == nil || got.Area != "海淀区" || got.TypeName != "酒店" || got.MaxPrice != 320 {
		t.Fatalf("unexpected normalized filter: %+v", got)
	}
	got = inferExplicitFilter("日料人均至少200但最多20，两个数都不能改")
	if got == nil || got.TypeName != "日料" || got.MinPrice != 200 || got.MaxPrice != 20 {
		t.Fatalf("unexpected contradictory range: %+v", got)
	}
}

func TestRequiredEvidenceTools(t *testing.T) {
	t.Parallel()
	assertTools := func(q string, want ...string) {
		t.Helper()
		got := requiredEvidenceTools(q)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("question=%q tools=%v want=%v", q, got, want)
		}
	}
	assertTools("先不用推荐，我还在确认同行的人")
	assertTools("给我找一家咖啡", agent.ToolSearchShops)
	assertTools("找一家适合安静办公的咖啡", agent.ToolSearchShops, agent.ToolListShopBlogs)
	assertTools("只查《静巷咖啡·国贸店》的地址", agent.ToolSearchShops, agent.ToolGetShop, agent.ToolListShopBlogs)
}

func TestConstrainProfilePatchToExplicitUtterancePreservesUnmentionedFields(t *testing.T) {
	t.Parallel()
	zero := int64(0)
	modelPatch := memory.ProfilePatch{
		PreferredAreasRemove: []string{"海淀区"}, BudgetMax: &zero,
	}
	got := constrainProfilePatchToExplicitUtterance("按更新后的条件来，给一个有依据的", modelPatch)
	if len(got.PreferredAreasRemove) != 0 || got.BudgetMax != nil {
		t.Fatalf("unmentioned profile fields must be preserved: %+v", got)
	}

	got = constrainProfilePatchToExplicitUtterance("记住以后优先丰台区，人均上限270", modelPatch)
	if got.BudgetMax == nil || len(got.PreferredAreasRemove) != 1 {
		t.Fatalf("explicit durable mutation should remain available: %+v", got)
	}
}

func TestPreflightRecommendationGuard(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		question string
		code     string
	}{
		"deferred":      {"先不用推荐，我还在确认同行的人", "recommendation_deferred"},
		"contradiction": {"日料人均至少200但最多20，两个数都不能改", "contradictory_price_range"},
		"taxonomy":      {"朝阳区找电影院，只认这个类别", "unsupported_category"},
		"reference":     {"还是上回那家吧", "unresolved_reference"},
		"insufficient":  {"给我一个合适的，区域、类别和预算我都没说；信息不足就问", "insufficient_constraints"},
	} {
		t.Run(name, func(t *testing.T) {
			code, answer := preflightRecommendationGuard(tc.question, nil, memory.Profile{})
			if code != tc.code || !strings.HasPrefix(answer, "推荐结果：无") {
				t.Fatalf("code=%q answer=%q", code, answer)
			}
		})
	}
	if code, _ := preflightRecommendationGuard("通州区的咖啡，没有就说没有", nil, memory.Profile{}); code != "" {
		t.Fatalf("valid empty retrieval should reach search, code=%s", code)
	}
}

func TestResolveWorkingQuestionCarriesPreviousIntent(t *testing.T) {
	t.Parallel()
	history := []memory.Message{
		{Role: "user", Content: "好了，就按前面那个地点和价位；需要专注敲代码，网速得稳"},
		{Role: "assistant", Content: "我来查找。"},
	}
	got := resolveWorkingQuestion("上句就是最终需求，给出带引用的结论", history)
	if !strings.Contains(got, "敲代码") || len(agent.RequiredSemanticConcepts(got)) == 0 {
		t.Fatalf("working question lost prior semantic intent: %q", got)
	}
}

func TestEffectiveProfileForQuestionClearsBudgetForCurrentSearch(t *testing.T) {
	t.Parallel()
	budget := int64(80)
	old := memory.Profile{PreferredAreas: []string{"海淀区"}, BudgetMax: &budget}
	got := effectiveProfileForQuestion(old, "忘掉预算，改为丰台区")
	if got.BudgetMax != nil {
		t.Fatalf("budget should be suppressed for current search: %+v", got)
	}
	if len(got.PreferredAreas) != 0 {
		t.Fatalf("current-turn area must suppress the old profile area: %+v", got)
	}
	if old.BudgetMax == nil || *old.BudgetMax != 80 {
		t.Fatalf("input profile was mutated: %+v", old)
	}
}

func TestEffectiveProfileForQuestionExactNameOverridesDefaults(t *testing.T) {
	t.Parallel()
	budget := int64(80)
	old := memory.Profile{
		PreferredAreas: []string{"丰台区"},
		PreferredTypes: []string{"美食"},
		BudgetMax:      &budget,
	}
	got := effectiveProfileForQuestion(old, "找「静巷咖啡·国贸店」，说明价格")
	if len(got.PreferredAreas) != 0 || len(got.PreferredTypes) != 0 || got.BudgetMax != nil {
		t.Fatalf("exact-name lookup must not inherit profile defaults: %+v", got)
	}
}

func TestInferDeterministicProfilePatchCorrection(t *testing.T) {
	t.Parallel()
	budget := int64(80)
	old := memory.Profile{
		PreferredAreas: []string{"海淀区"},
		PreferredTypes: []string{"咖啡"},
		BudgetMax:      &budget,
	}
	got := inferDeterministicProfilePatch("忘掉预算，改为丰台区，再推荐一家适合家庭聚餐的店", old)
	if got.BudgetMax == nil || *got.BudgetMax != 0 {
		t.Fatalf("budget clear missing: %+v", got)
	}
	if len(got.PreferredAreasAdd) != 1 || got.PreferredAreasAdd[0] != "丰台区" {
		t.Fatalf("area add missing: %+v", got)
	}
	if len(got.PreferredAreasRemove) != 1 || got.PreferredAreasRemove[0] != "海淀区" {
		t.Fatalf("old area removal missing: %+v", got)
	}
	if len(got.PreferredTypesAdd) != 0 || len(got.PreferredTypesRemove) != 0 {
		t.Fatalf("semantic request must not infer a hard shop type: %+v", got)
	}
}

func TestInferDeterministicProfilePatchWorkingIntentTurns(t *testing.T) {
	t.Parallel()
	areaPatch := inferDeterministicProfilePatch("我接下来几次都在东城区活动", memory.Profile{})
	if len(areaPatch.PreferredAreasAdd) != 1 || areaPatch.PreferredAreasAdd[0] != "东城区" {
		t.Fatalf("area patch=%+v", areaPatch)
	}
	budgetPatch := inferDeterministicProfilePatch("花销按人均250封顶，先记住", memory.Profile{})
	if budgetPatch.BudgetMax == nil || *budgetPatch.BudgetMax != 250 {
		t.Fatalf("budget patch=%+v", budgetPatch)
	}
}

func TestMemoryPolicyIsCapabilityNotSeparateAgent(t *testing.T) {
	t.Parallel()
	if got := memoryPolicyForRequest("海淀咖啡", memory.Profile{}, memory.SessionSummary{}, nil); got != agent.MemoryNone {
		t.Fatalf("empty memory policy=%s", got)
	}
	profile := memory.Profile{PreferredAreas: []string{"海淀区"}}
	if got := memoryPolicyForRequest("推荐安静咖啡", profile, memory.SessionSummary{}, nil); got != agent.MemoryReadOnly {
		t.Fatalf("read policy=%s", got)
	}
	if got := memoryPolicyForRequest("以后优先海淀区，再推荐咖啡", profile, memory.SessionSummary{}, nil); got != agent.MemoryWriteAfterSuccess {
		t.Fatalf("write policy=%s", got)
	}
}

func TestEffectiveProfileDropsFieldsOverriddenThisTurn(t *testing.T) {
	t.Parallel()
	budget := int64(120)
	old := memory.Profile{
		PreferredAreas: []string{"朝阳区"}, PreferredTypes: []string{"火锅"}, BudgetMax: &budget,
	}
	got := effectiveProfileForQuestion(old, "海淀区找咖啡，人均80以内")
	if len(got.PreferredAreas) != 0 || len(got.PreferredTypes) != 0 || got.BudgetMax != nil {
		t.Fatalf("profile overrides leaked into prompt: %+v", got)
	}
}
