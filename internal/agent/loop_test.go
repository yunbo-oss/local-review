package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"local-review-go/internal/llm"
)

type scriptedClient struct {
	turns []llm.AssistantTurn
	i     int
}

func (s *scriptedClient) ChatWithTools(ctx context.Context, messages []llm.ChatMessage, tools []llm.ToolDefinition) (llm.AssistantTurn, error) {
	if err := ctx.Err(); err != nil {
		return llm.AssistantTurn{}, err
	}
	if s.i >= len(s.turns) {
		return llm.AssistantTurn{Message: llm.ChatMessage{Role: "assistant", Content: "done"}}, nil
	}
	t := s.turns[s.i]
	s.i++
	return t, nil
}

func (s *scriptedClient) ChatCompleteTurn(ctx context.Context, messages []llm.ChatMessage) (llm.AssistantTurn, error) {
	return s.ChatWithTools(ctx, messages, nil)
}

type fakeSearch struct {
	ids []int64
}

type blockingClient struct{}

func (blockingClient) ChatWithTools(ctx context.Context, _ []llm.ChatMessage, _ []llm.ToolDefinition) (llm.AssistantTurn, error) {
	<-ctx.Done()
	return llm.AssistantTurn{}, ctx.Err()
}

func (blockingClient) ChatCompleteTurn(ctx context.Context, _ []llm.ChatMessage) (llm.AssistantTurn, error) {
	<-ctx.Done()
	return llm.AssistantTurn{}, ctx.Err()
}

// minimal stubs via ToolExecutor with nil Search - we'll inject Observed manually in tests for groundedness

func TestValidateGroundedness(t *testing.T) {
	t.Parallel()
	if !ValidateGroundedness("推荐 [shop:1] 和 [shop:2]", []int64{1, 2, 3}) {
		t.Fatal("should pass")
	}
	if ValidateGroundedness("推荐 [shop:99]", []int64{1, 2}) {
		t.Fatal("should fail")
	}
}

func TestNormalizeCitationSyntax(t *testing.T) {
	t.Parallel()
	got := NormalizeCitationSyntax("推荐 [喜茶](shop:12) 和 [shop:8]")
	if got != "推荐 [shop:12] 和 [shop:8]" {
		t.Fatalf("NormalizeCitationSyntax() = %q", got)
	}
}

func TestParseRecommendedShopIDs_SeparatesEvidenceCitations(t *testing.T) {
	t.Parallel()
	answer := "推荐结果：[shop:26]\n不推荐 [shop:16]，因为它超出预算。"
	ids, found := ParseRecommendedShopIDs(answer)
	if !found || len(ids) != 1 || ids[0] != 26 {
		t.Fatalf("recommended=%v found=%v", ids, found)
	}
	if cited := ParseCitedShopIDs(answer); len(cited) != 2 {
		t.Fatalf("cited=%v", cited)
	}

	ids, found = ParseRecommendedShopIDs("推荐结果：无\n没有满足条件的候选。")
	if !found || len(ids) != 0 {
		t.Fatalf("no-result recommended=%v found=%v", ids, found)
	}

	ids, found = ParseRecommendedShopIDs("可考虑 [shop:29]")
	if found || len(ids) != 1 || ids[0] != 29 {
		t.Fatalf("legacy fallback=%v found=%v", ids, found)
	}
}

func TestNormalizeAnswerContract_MovesMarkdownHeaderToFirstLine(t *testing.T) {
	t.Parallel()
	got := NormalizeAnswerContract("先说明证据。\n\n**推荐结果：[shop:26]**\n[shop:16] 只作反例。")
	want := "推荐结果：[shop:26]\n先说明证据。\n\n[shop:16] 只作反例。"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	ids, found := ParseRecommendedShopIDs(got)
	if !found || len(ids) != 1 || ids[0] != 26 {
		t.Fatalf("recommended=%v found=%v", ids, found)
	}
}

func TestFinalizeGrounding_FactualLookupAddsNoRecommendationHeader(t *testing.T) {
	ledger := NewEvidenceLedger()
	ledger.DiscoverFromSearch(30, "月下餐厅", map[string]any{"area": "朝阳区", "type_name": "美食"})
	res := LoopResult{Answer: "现有材料没有说明预约政策，无法确认。[shop:30]"}
	exec := &ToolExecutor{Ledger: ledger, Observed: map[int64]struct{}{}, FactualLookup: true}

	finalizeGrounding(&res, exec)

	if !strings.HasPrefix(res.Answer, "推荐结果：无\n") {
		t.Fatalf("expected factual no-recommendation header, got %q", res.Answer)
	}
	if !res.GroundingOK {
		t.Fatalf("expected grounded factual answer, got code=%s err=%v", res.GroundingCode, res.Err)
	}
}

func TestRunLoop_DuplicateRejected(t *testing.T) {
	t.Parallel()
	client := &scriptedClient{turns: []llm.AssistantTurn{
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "1", Name: ToolSearchShops, Args: `{"query":"咖啡"}`},
		}}, ToolCalls: []llm.ToolCall{{ID: "1", Name: ToolSearchShops, Args: `{"query":"咖啡"}`}}},
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "2", Name: ToolSearchShops, Args: `{"query":"咖啡"}`},
		}}, ToolCalls: []llm.ToolCall{{ID: "2", Name: ToolSearchShops, Args: `{"query":"咖啡"}`}}},
		{Message: llm.ChatMessage{Role: "assistant", Content: "推荐 [shop:1]"}},
	}}
	exec := &ToolExecutor{
		Ledger:   NewEvidenceLedger(),
		Observed: map[int64]struct{}{},
		// Search nil → first call returns error JSON; duplicate still counted
	}
	// Pre-seed evidence so final answer grounding passes
	exec.Ledger.DiscoverFromSearch(1, "测试店", nil)
	res := RunLoop(context.Background(), client, exec, RunConfig{
		MaxSteps: 3, MaxToolCalls: 5, RunTimeout: DefaultRunTimeout, ToolTimeout: DefaultToolTimeout, MaxToolResultChars: 1000,
	}, []llm.ChatMessage{{Role: "user", Content: "咖啡"}}, nil)
	if res.DuplicateRejected < 1 {
		t.Fatalf("want duplicate reject, got %+v", res)
	}
	if !res.GroundingOK {
		t.Fatalf("want grounding ok after seed, err=%v", res.Err)
	}
}

func TestRunLoop_RequestsMissingRequiredEvidenceTool(t *testing.T) {
	t.Parallel()
	client := &scriptedClient{turns: []llm.AssistantTurn{
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: ToolSearchShops, Args: `{"query":"咖啡"}`}}}, ToolCalls: []llm.ToolCall{{ID: "1", Name: ToolSearchShops, Args: `{"query":"咖啡"}`}}},
		{Message: llm.ChatMessage{Role: "assistant", Content: "推荐结果：[shop:29]"}},
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "2", Name: ToolListShopBlogs, Args: `{"shop_id":29}`}}}, ToolCalls: []llm.ToolCall{{ID: "2", Name: ToolListShopBlogs, Args: `{"shop_id":29}`}}},
		{Message: llm.ChatMessage{Role: "assistant", Content: "推荐结果：[shop:29]"}},
	}}
	exec := &ToolExecutor{
		Search: normalizedScoreSearch{}, Ledger: NewEvidenceLedger(), RequiredTools: []string{ToolSearchShops, ToolListShopBlogs},
	}
	res := RunLoop(context.Background(), client, exec, DefaultRunConfig(), []llm.ChatMessage{{Role: "user", Content: "x"}}, nil)
	if res.ToolCalls != 2 || len(res.ToolNames) != 2 || res.ToolNames[1] != ToolListShopBlogs {
		t.Fatalf("missing required tool was not requested: %+v", res)
	}
}

func TestRunLoop_PrefetchesExactShopEvidenceWithinBound(t *testing.T) {
	t.Parallel()
	client := &scriptedClient{turns: []llm.AssistantTurn{
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: ToolSearchShops, Args: `{"query":"无界餐厅"}`}}}, ToolCalls: []llm.ToolCall{{ID: "1", Name: ToolSearchShops, Args: `{"query":"无界餐厅"}`}}},
		{Message: llm.ChatMessage{Role: "assistant", Content: "推荐结果：[shop:29]"}},
	}}
	exec := &ToolExecutor{
		Search: normalizedScoreSearch{}, Ledger: NewEvidenceLedger(),
		RequiredTools:  []string{ToolSearchShops, ToolGetShop, ToolListShopBlogs},
		TargetShopName: "无界餐厅",
	}
	res := RunLoop(context.Background(), client, exec, DefaultRunConfig(), []llm.ChatMessage{{Role: "user", Content: "x"}}, nil)
	if res.ToolCalls != 3 || strings.Join(res.ToolNames, ",") != strings.Join([]string{ToolSearchShops, ToolGetShop, ToolListShopBlogs}, ",") {
		t.Fatalf("exact evidence plan=%+v", res)
	}
}

func TestRunLoop_MaxSteps(t *testing.T) {
	t.Parallel()
	client := &scriptedClient{turns: []llm.AssistantTurn{
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "1", Name: ToolGetShop, Args: `{"shop_id":1}`},
		}}, ToolCalls: []llm.ToolCall{{ID: "1", Name: ToolGetShop, Args: `{"shop_id":1}`}}},
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "2", Name: ToolGetShop, Args: `{"shop_id":2}`},
		}}, ToolCalls: []llm.ToolCall{{ID: "2", Name: ToolGetShop, Args: `{"shop_id":2}`}}},
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "3", Name: ToolGetShop, Args: `{"shop_id":3}`},
		}}, ToolCalls: []llm.ToolCall{{ID: "3", Name: ToolGetShop, Args: `{"shop_id":3}`}}},
		// ChatCompleteTurn after max steps
		{Message: llm.ChatMessage{Role: "assistant", Content: "无合适结果"}},
	}}
	exec := &ToolExecutor{Observed: map[int64]struct{}{}}
	res := RunLoop(context.Background(), client, exec, RunConfig{
		MaxSteps: 3, MaxToolCalls: 5, RunTimeout: DefaultRunTimeout, ToolTimeout: DefaultToolTimeout, MaxToolResultChars: 600,
	}, []llm.ChatMessage{{Role: "user", Content: "x"}}, nil)
	if res.Steps != 3 {
		t.Fatalf("steps=%d", res.Steps)
	}
}

func TestRunLoop_GroundingFail(t *testing.T) {
	t.Parallel()
	client := &scriptedClient{turns: []llm.AssistantTurn{
		{Message: llm.ChatMessage{Role: "assistant", Content: "试试 [shop:999]"}},
	}}
	exec := &ToolExecutor{Observed: map[int64]struct{}{1: {}}}
	res := RunLoop(context.Background(), client, exec, DefaultRunConfig(), []llm.ChatMessage{{Role: "user", Content: "x"}}, nil)
	if res.GroundingOK || res.Err == nil {
		t.Fatalf("want grounding error, got %+v", res)
	}
}

func TestRunLoop_RepairsUnknownShopWithoutNewTools(t *testing.T) {
	t.Parallel()
	client := &scriptedClient{turns: []llm.AssistantTurn{
		{Message: llm.ChatMessage{Role: "assistant", Content: "推荐已发现的店 [shop:1]，以及不存在的店 [shop:999]。"}},
		{Message: llm.ChatMessage{Role: "assistant", Content: "推荐已发现的店 [shop:1]。"}},
	}}
	exec := &ToolExecutor{Ledger: NewEvidenceLedger(), Observed: map[int64]struct{}{}}
	exec.Ledger.DiscoverFromSearch(1, "已发现的店", map[string]any{"avg_price": int64(50)})
	res := RunLoop(context.Background(), client, exec, DefaultRunConfig(), []llm.ChatMessage{{Role: "user", Content: "推荐一家店"}}, nil)
	if !res.GroundingOK || res.GroundingCode != "" {
		t.Fatalf("repair should pass full verifier: %+v", res)
	}
	if res.ModelCalls != 2 || res.ToolCalls != 0 {
		t.Fatalf("repair must use one model call and no tools: %+v", res)
	}
}

func TestRunLoop_RepairsStructuredFactConflictWithoutNewTools(t *testing.T) {
	t.Parallel()
	client := &scriptedClient{turns: []llm.AssistantTurn{
		{Message: llm.ChatMessage{Role: "assistant", Content: "推荐无界餐厅 [shop:29]，评分：47。"}},
		{Message: llm.ChatMessage{Role: "assistant", Content: "推荐无界餐厅 [shop:29]，评分：4.7。"}},
	}}
	exec := &ToolExecutor{Ledger: NewEvidenceLedger(), Observed: map[int64]struct{}{}}
	exec.Ledger.DiscoverFromSearch(29, "无界餐厅", map[string]any{"score": 47})
	res := RunLoop(context.Background(), client, exec, DefaultRunConfig(), []llm.ChatMessage{{Role: "user", Content: "推荐一家店"}}, nil)
	if !res.GroundingOK || res.GroundingCode != "" {
		t.Fatalf("fact repair should pass full verifier: %+v", res)
	}
	if res.ModelCalls != 2 || res.ToolCalls != 0 {
		t.Fatalf("repair must use one model call and no tools: %+v", res)
	}
}

func TestRunLoop_PerTurnCapAndAttempts(t *testing.T) {
	t.Parallel()
	client := &scriptedClient{turns: []llm.AssistantTurn{
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "1", Name: ToolSearchShops, Args: `{"query":"a"}`},
			{ID: "2", Name: ToolSearchShops, Args: `{"query":"b"}`},
			{ID: "3", Name: ToolSearchShops, Args: `{"query":"c"}`},
			{ID: "4", Name: ToolSearchShops, Args: `{"query":"d"}`},
		}}, ToolCalls: []llm.ToolCall{
			{ID: "1", Name: ToolSearchShops, Args: `{"query":"a"}`},
			{ID: "2", Name: ToolSearchShops, Args: `{"query":"b"}`},
			{ID: "3", Name: ToolSearchShops, Args: `{"query":"c"}`},
			{ID: "4", Name: ToolSearchShops, Args: `{"query":"d"}`},
		}},
		{Message: llm.ChatMessage{Role: "assistant", Content: "暂无合适店铺"}},
	}}
	exec := &ToolExecutor{Ledger: NewEvidenceLedger(), Observed: map[int64]struct{}{}}
	res := RunLoop(context.Background(), client, exec, RunConfig{
		MaxSteps: 2, MaxToolCalls: 10, MaxToolAttempts: 20, MaxToolsPerTurn: 2,
		RunTimeout: DefaultRunTimeout, ToolTimeout: DefaultToolTimeout, MaxToolResultChars: 500,
	}, []llm.ChatMessage{{Role: "user", Content: "x"}}, nil)
	if res.ToolCalls > 2 {
		t.Fatalf("per-turn cap: tool_calls=%d", res.ToolCalls)
	}
	if len(res.ToolNames) != res.ToolCalls {
		t.Fatalf("tool telemetry names=%v calls=%d", res.ToolNames, res.ToolCalls)
	}
	if res.ToolAttempts < 4 {
		t.Fatalf("skipped calls should count as attempts, attempts=%d", res.ToolAttempts)
	}
}

func TestRunLoop_DuplicateConsumesAttempt(t *testing.T) {
	t.Parallel()
	client := &scriptedClient{turns: []llm.AssistantTurn{
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "1", Name: ToolSearchShops, Args: `{"query":"咖啡"}`},
		}}, ToolCalls: []llm.ToolCall{{ID: "1", Name: ToolSearchShops, Args: `{"query":"咖啡"}`}}},
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "2", Name: ToolSearchShops, Args: `{"query":"咖啡"}`},
		}}, ToolCalls: []llm.ToolCall{{ID: "2", Name: ToolSearchShops, Args: `{"query":"咖啡"}`}}},
		{Message: llm.ChatMessage{Role: "assistant", Content: "暂无合适店铺"}},
	}}
	exec := &ToolExecutor{Ledger: NewEvidenceLedger()}
	res := RunLoop(context.Background(), client, exec, RunConfig{
		MaxSteps: 3, MaxToolCalls: 5, MaxToolAttempts: 8, MaxToolsPerTurn: 3,
		RunTimeout: DefaultRunTimeout, ToolTimeout: DefaultToolTimeout, MaxToolResultChars: 500,
	}, []llm.ChatMessage{{Role: "user", Content: "x"}}, nil)
	if res.DuplicateRejected < 1 || res.ToolAttempts < 2 {
		t.Fatalf("dup should consume attempts: %+v", res)
	}
}

func TestRunLoop_RunTimeoutBoundsModelFailure(t *testing.T) {
	start := time.Now()
	res := RunLoop(context.Background(), blockingClient{}, &ToolExecutor{}, RunConfig{
		MaxSteps: 3, MaxToolCalls: 5, MaxToolAttempts: 8, MaxToolsPerTurn: 3,
		RunTimeout: 25 * time.Millisecond, ToolTimeout: time.Second, MaxToolResultChars: 500,
	}, []llm.ChatMessage{{Role: "user", Content: "x"}}, nil)
	if !errors.Is(res.Err, context.DeadlineExceeded) {
		t.Fatalf("error=%v want deadline exceeded", res.Err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("run timeout was not bounded: %v", elapsed)
	}
}

func TestRunLoop_ConcurrentRunsDoNotShareEvidence(t *testing.T) {
	const runs = 20
	var wg sync.WaitGroup
	errs := make(chan error, runs)
	for i := 1; i <= runs; i++ {
		id := int64(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &scriptedClient{turns: []llm.AssistantTurn{{
				Message: llm.ChatMessage{Role: "assistant", Content: "推荐 [shop:" + fmt.Sprint(id) + "]"},
			}}}
			exec := &ToolExecutor{Ledger: NewEvidenceLedger(), Observed: map[int64]struct{}{}}
			exec.Ledger.DiscoverFromSearch(id, "并发测试店", nil)
			res := RunLoop(context.Background(), client, exec, DefaultRunConfig(), []llm.ChatMessage{{Role: "user", Content: "x"}}, nil)
			if !res.GroundingOK || len(res.ObservedShopIDs) != 1 || res.ObservedShopIDs[0] != id {
				errs <- fmt.Errorf("run %d leaked evidence: %+v", id, res)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
