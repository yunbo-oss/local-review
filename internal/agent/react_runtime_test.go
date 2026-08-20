package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"local-review-go/internal/llm"
)

type decisionControllerStub struct {
	mu        sync.Mutex
	decisions []AgentDecision
	inputs    []ControllerInput
	calls     int
}

type blockingDecisionController struct{}

func (blockingDecisionController) Decide(ctx context.Context, _ ControllerInput) (AgentDecision, llm.TokenUsage, error) {
	<-ctx.Done()
	return AgentDecision{}, llm.TokenUsage{}, ctx.Err()
}

func (c *decisionControllerStub) Decide(_ context.Context, input ControllerInput) (AgentDecision, llm.TokenUsage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inputs = append(c.inputs, input)
	if c.calls >= len(c.decisions) {
		return AgentDecision{}, llm.TokenUsage{}, context.DeadlineExceeded
	}
	decision := c.decisions[c.calls]
	c.calls++
	return decision, llm.TokenUsage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12}, nil
}

func TestReactRuntimeRunsObservationDrivenLoopAndCheckpoints(t *testing.T) {
	state := newRuntimeTestState(t)
	controller := &decisionControllerStub{decisions: []AgentDecision{
		{Type: DecisionAct, ReasonCode: "INITIAL_SEARCH", Actions: []AgentAction{
			{ID: "search-1", Tool: ToolSearchShops, Args: json.RawMessage(`{"query":"海淀咖啡"}`)},
		}},
		{Type: DecisionAct, ReasonCode: "MISSING_EVIDENCE", Actions: []AgentAction{
			{ID: "detail-7", Tool: ToolGetShop, Args: json.RawMessage(`{"shop_id":7}`), DependsOn: []string{"search-1"}},
			{ID: "reviews-7", Tool: ToolListShopBlogs, Args: json.RawMessage(`{"shop_id":7,"limit":5}`), DependsOn: []string{"search-1"}},
		}},
		{Type: DecisionFinish, ReasonCode: "EVIDENCE_SUFFICIENT"},
	}}
	toolRunner := &structuredRunnerStub{delay: 10 * time.Millisecond}
	checkpoint := NewMemoryAgentCheckpointer()
	var events []RuntimeEvent
	runtime := &ReactRuntime{
		Controller: controller,
		Executor: &ParallelActionExecutor{Tools: toolRunner, Config: ParallelExecutorConfig{
			MaxParallel: 3, MaxAttemptsPerAction: 1, ToolTimeout: time.Second,
		}},
		Checkpointer: checkpoint,
		Config: ReactRuntimeConfig{
			RunTimeout: time.Second,
			OnEvent:    func(event RuntimeEvent) { events = append(events, event) },
		},
	}
	result := runtime.Run(context.Background(), state)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if state.Status != RuntimeCompleted || state.StopReason != "EVIDENCE_SUFFICIENT" {
		t.Fatalf("terminal state=%+v", state)
	}
	if len(result.Decisions) != 3 || len(result.Batches) != 2 || result.ModelCalls != 3 || result.Usage.TotalTokens != 36 {
		t.Fatalf("run result=%+v", result)
	}
	if len(controller.inputs) < 2 || len(controller.inputs[1].Candidates) != 2 {
		t.Fatalf("second controller turn did not observe candidates: %+v", controller.inputs)
	}
	restored, err := checkpoint.Load(context.Background(), state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Status != RuntimeCompleted || restored.Revision != state.Revision {
		t.Fatalf("terminal checkpoint=%+v", restored)
	}
	if len(events) == 0 || events[0].Type != RuntimeEventStarted || events[len(events)-1].Type != RuntimeEventCompleted {
		t.Fatalf("runtime events=%+v", events)
	}
}

func TestReactRuntimeRepairsOneInvalidControllerDecision(t *testing.T) {
	state := newRuntimeTestState(t)
	controller := &decisionControllerStub{decisions: []AgentDecision{
		{Type: DecisionFinish, ReasonCode: "TOO_EARLY"},
		{Type: DecisionAct, ReasonCode: "INITIAL_SEARCH", Actions: []AgentAction{{
			ID: "search-repaired", Tool: ToolSearchShops, Args: json.RawMessage(`{"query":"海淀咖啡"}`),
		}}},
		{Type: DecisionFinish, ReasonCode: "EVIDENCE_SUFFICIENT"},
	}}
	runtime := &ReactRuntime{
		Controller: controller,
		Executor:   &ParallelActionExecutor{Tools: &structuredRunnerStub{}, Config: DefaultParallelExecutorConfig()},
		Config:     DefaultReactRuntimeConfig(),
	}
	result := runtime.Run(context.Background(), state)
	if result.Err != nil || state.Status != RuntimeCompleted {
		t.Fatalf("repair did not recover runtime: result=%+v state=%+v", result, state)
	}
	if result.ModelCalls != 3 || state.Budget.Turns != 2 || len(controller.inputs) != 3 {
		t.Fatalf("repair accounting is wrong: calls=%d turns=%d inputs=%d", result.ModelCalls, state.Budget.Turns, len(controller.inputs))
	}
	if controller.inputs[1].ValidationFeedback == "" {
		t.Fatal("repair call did not receive validation feedback")
	}
}

func TestReactRuntimeClarifiesWithoutToolExecution(t *testing.T) {
	state := newRuntimeTestState(t)
	controller := &decisionControllerStub{decisions: []AgentDecision{{
		Type: DecisionClarify, ReasonCode: "USER_CLARIFICATION_REQUIRED", Clarification: "你想找哪个区域？",
	}}}
	toolRunner := &structuredRunnerStub{}
	runtime := &ReactRuntime{
		Controller: controller,
		Executor:   &ParallelActionExecutor{Tools: toolRunner, Config: DefaultParallelExecutorConfig()},
		Config:     DefaultReactRuntimeConfig(),
	}
	result := runtime.Run(context.Background(), state)
	if result.Err != nil || result.Clarification != "你想找哪个区域？" || state.Status != RuntimeNeedsClarify {
		t.Fatalf("clarify result=%+v state=%+v", result, state)
	}
	if toolRunner.callCount() != 0 {
		t.Fatalf("clarify executed tools: %d", toolRunner.callCount())
	}
}

func TestReactRuntimeStopsAtTurnBudget(t *testing.T) {
	state := newRuntimeTestState(t)
	state.Budget.MaxTurns = 1
	controller := &decisionControllerStub{decisions: []AgentDecision{{
		Type: DecisionAct, ReasonCode: "INITIAL_SEARCH", Actions: []AgentAction{
			{ID: "search-1", Tool: ToolSearchShops, Args: json.RawMessage(`{"query":"海淀咖啡"}`)},
		},
	}}}
	runtime := &ReactRuntime{
		Controller: controller,
		Executor: &ParallelActionExecutor{Tools: &structuredRunnerStub{}, Config: ParallelExecutorConfig{
			MaxParallel: 1, MaxAttemptsPerAction: 1, ToolTimeout: time.Second,
		}},
		Config: DefaultReactRuntimeConfig(),
	}
	result := runtime.Run(context.Background(), state)
	if result.Err != nil || state.Status != RuntimeBudgetExhausted || state.StopReason != ErrMaxSteps {
		t.Fatalf("budget result=%+v state=%+v", result, state)
	}
}

func TestReactRuntimeMarksControllerTimeoutAsCancelled(t *testing.T) {
	state := newRuntimeTestState(t)
	runtime := &ReactRuntime{
		Controller: blockingDecisionController{},
		Executor:   &ParallelActionExecutor{Tools: &structuredRunnerStub{}, Config: DefaultParallelExecutorConfig()},
		Config:     ReactRuntimeConfig{RunTimeout: 20 * time.Millisecond, ControllerOutputRunes: 1200},
	}
	result := runtime.Run(context.Background(), state)
	if result.Err == nil || state.Status != RuntimeCancelled || state.StopReason != "context_cancelled" {
		t.Fatalf("controller timeout must be a cancelled terminal run: result=%+v state=%+v", result, state)
	}
}

func TestMemoryAgentCheckpointerCopiesAndRejectsRevisionRegression(t *testing.T) {
	state := newRuntimeTestState(t)
	checkpoint := NewMemoryAgentCheckpointer()
	state.Revision = 3
	if err := checkpoint.Save(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	state.Question = "mutated after save"
	restored, err := checkpoint.Load(context.Background(), state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Question == state.Question {
		t.Fatal("checkpoint aliases caller state")
	}
	state.Revision = 2
	if err := checkpoint.Save(context.Background(), state); err == nil || !strings.Contains(err.Error(), "revision regression") {
		t.Fatalf("regression error=%v", err)
	}
}

func TestParseAgentDecisionStrictAndNoThoughtField(t *testing.T) {
	decision, err := ParseAgentDecision(`{"type":"act","reason_code":"initial_search","actions":[{"id":"a1","tool":"search_shops","args":{"query":"咖啡"}}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if decision.ReasonCode != "INITIAL_SEARCH" || len(decision.Actions) != 1 {
		t.Fatalf("decision=%+v", decision)
	}
	if _, err := ParseAgentDecision(`{"type":"finish","reason_code":"done","thought":"hidden chain"}`); err == nil {
		t.Fatal("unknown thought field must be rejected")
	}
}
