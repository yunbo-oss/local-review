package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"local-review-go/internal/llm"
)

func TestRunReactBridgesRuntimeToVerifiedClaimAnswer(t *testing.T) {
	controller := &decisionControllerStub{decisions: []AgentDecision{
		{Type: DecisionAct, ReasonCode: "INITIAL_SEARCH", Actions: []AgentAction{
			{ID: "search-1", Tool: ToolSearchShops, Args: json.RawMessage(`{"query":"海淀咖啡 retry"}`)},
		}},
		{Type: DecisionFinish, ReasonCode: "EVIDENCE_SUFFICIENT"},
	}}
	client := &scriptedClient{turns: []llm.AssistantTurn{{
		Message: llm.ChatMessage{Role: "assistant", Content: `{"no_result":false,"summary":"找到符合硬条件的候选","recommendations":[{"shop_id":7,"claims":[{"text":"位于海淀区","field":"area","value":"海淀区","evidence_refs":["shop:7.area"]}]}]}`},
		Usage:   llm.TokenUsage{TotalTokens: 17},
	}}}
	exec := &ToolExecutor{
		Search: queryPlanSearch{}, Ledger: NewEvidenceLedger(), Observed: map[int64]struct{}{},
	}
	checkpoint := NewMemoryAgentCheckpointer()
	intent := FallbackIntentSpec("海淀咖啡 retry", "agent")
	res := RunReact(
		context.Background(), client, controller, exec, checkpoint,
		DefaultRunConfig(), DefaultReactRuntimeConfig(),
		[]llm.ChatMessage{{Role: "user", Content: intent.OriginalQuestion}},
		ReactHarnessInput{
			RunID: "run-react-1", TraceID: "trace-react-1", Question: intent.OriginalQuestion,
			Intent: intent, Memory: MemorySnapshot{Policy: MemoryReadOnly},
		},
	)
	if res.Err != nil || !res.GroundingOK {
		t.Fatalf("react result failed: %+v", res)
	}
	if res.RuntimeVersion != RuntimeVersionV2React || res.RuntimeState == nil || res.RuntimeState.Status != RuntimeCompleted {
		t.Fatalf("runtime state missing: %+v", res)
	}
	if res.ToolCalls != 1 || res.ToolAttempts != 1 || res.ModelCalls != 3 || res.Usage.TotalTokens != 41 {
		t.Fatalf("trajectory counters=%+v", res)
	}
	if res.ClaimAnswer == nil || len(res.ObservedShopIDs) != 1 || res.ObservedShopIDs[0] != 7 {
		t.Fatalf("verified evidence missing: %+v", res)
	}
	restored, err := checkpoint.Load(context.Background(), "run-react-1")
	if err != nil || restored.Status != RuntimeCompleted {
		t.Fatalf("terminal checkpoint=%+v err=%v", restored, err)
	}
}

func TestRunReactFallsBackToTypedEvidenceWhenClaimJSONCannotBeRepaired(t *testing.T) {
	controller := &decisionControllerStub{decisions: []AgentDecision{
		{Type: DecisionAct, ReasonCode: "INITIAL_SEARCH", Actions: []AgentAction{
			{ID: "search-1", Tool: ToolSearchShops, Args: json.RawMessage(`{"query":"海淀咖啡 retry"}`)},
		}},
		{Type: DecisionFinish, ReasonCode: "EVIDENCE_SUFFICIENT"},
	}}
	client := &scriptedClient{turns: []llm.AssistantTurn{
		{Message: llm.ChatMessage{Role: "assistant", Content: `not-json`}, Usage: llm.TokenUsage{TotalTokens: 5}},
		{Message: llm.ChatMessage{Role: "assistant", Content: `still-not-json`}, Usage: llm.TokenUsage{TotalTokens: 6}},
	}}
	exec := &ToolExecutor{Search: queryPlanSearch{}, Ledger: NewEvidenceLedger(), Observed: map[int64]struct{}{}}
	checkpoint := NewMemoryAgentCheckpointer()
	intent := FallbackIntentSpec("海淀咖啡 retry", "agent")
	res := RunReact(
		context.Background(), client, controller, exec, checkpoint,
		DefaultRunConfig(), DefaultReactRuntimeConfig(),
		[]llm.ChatMessage{{Role: "user", Content: intent.OriginalQuestion}},
		ReactHarnessInput{RunID: "run-react-fallback", TraceID: "trace", Question: intent.OriginalQuestion, Intent: intent},
	)
	if res.Err != nil || !res.GroundingOK || !res.ClaimFallback {
		t.Fatalf("typed fallback did not recover the run: %+v", res)
	}
	if res.ClaimAnswer == nil || !strings.HasPrefix(res.Answer, "推荐结果：[shop:7]") {
		t.Fatalf("fallback answer missing: %+v", res)
	}
	if res.RuntimeState == nil || res.RuntimeState.Status != RuntimeCompleted || !res.RuntimeState.AnswerVerified {
		t.Fatalf("fallback terminal state not verified: %+v", res.RuntimeState)
	}
}
