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
		Observed: map[int64]struct{}{},
		// Search nil → first call returns error JSON; duplicate still counted
	}
	// Pre-seed observed so grounding passes
	exec.Observed[1] = struct{}{}
	res := RunLoop(context.Background(), client, exec, RunConfig{
		MaxSteps: 3, MaxToolCalls: 5, RunTimeout: DefaultRunTimeout, ToolTimeout: DefaultToolTimeout, MaxToolResultChars: 1000,
	}, []llm.ChatMessage{{Role: "user", Content: "咖啡"}}, nil)
	if res.DuplicateRejected < 1 {
		t.Fatalf("want duplicate reject, got %+v", res)
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
