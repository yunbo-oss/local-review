package agent

import (
	"context"
	"testing"

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
