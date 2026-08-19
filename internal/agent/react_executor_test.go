package agent

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type structuredRunnerStub struct {
	mu             sync.Mutex
	calls          []string
	active         int
	maxActive      int
	failFirstQuery string
	failedOnce     bool
	delay          time.Duration
}

type timeoutStructuredRunner struct {
	mu    sync.Mutex
	calls int
}

func (s *timeoutStructuredRunner) ExecuteStructured(ctx context.Context, name, _ string) ToolResult {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	<-ctx.Done()
	return ToolResult{Tool: name, Status: ActionFailed, ErrorCode: ErrToolTimeout, ErrorDetail: ctx.Err().Error()}
}

func (s *timeoutStructuredRunner) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *structuredRunnerStub) ExecuteStructured(_ context.Context, name, args string) ToolResult {
	s.mu.Lock()
	s.calls = append(s.calls, name+"|"+CanonicalArgs(args))
	s.active++
	if s.active > s.maxActive {
		s.maxActive = s.active
	}
	fail := s.failFirstQuery != "" && !s.failedOnce && name == ToolSearchShops && containsAny(args, s.failFirstQuery)
	if fail {
		s.failedOnce = true
	}
	s.mu.Unlock()
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	s.mu.Lock()
	s.active--
	s.mu.Unlock()
	if fail {
		return ToolResult{Tool: name, Status: ActionFailed, ErrorCode: ErrToolExecution, ErrorDetail: "transient search failure"}
	}
	switch name {
	case ToolSearchShops:
		return ToolResult{
			Tool: name, Status: ActionSucceeded, ResultCount: 2,
			CandidateIDs: []int64{7, 9},
			Output:       `[{"shop_id":7,"name":"七号咖啡"},{"shop_id":9,"name":"九号咖啡"}]`,
		}
	case ToolGetShop:
		return ToolResult{Tool: name, Status: ActionSucceeded, ResultCount: 1, Output: `{"shop_id":7,"name":"七号咖啡"}`}
	case ToolListShopBlogs:
		return ToolResult{Tool: name, Status: ActionSucceeded, ResultCount: 2, Output: `{"shop_id":7,"blogs":[{"blog_id":1},{"blog_id":2}]}`}
	default:
		return ToolResult{Tool: name, Status: ActionFailed, ErrorCode: ErrToolUnknown}
	}
}

func (s *structuredRunnerStub) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *structuredRunnerStub) concurrency() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxActive
}

func TestParallelActionExecutorSearchBarrierThenEvidenceFanout(t *testing.T) {
	state := newRuntimeTestState(t)
	runner := &structuredRunnerStub{delay: 20 * time.Millisecond}
	executor := &ParallelActionExecutor{Tools: runner, Config: ParallelExecutorConfig{
		MaxParallel: 3, MaxAttemptsPerAction: 1, ToolTimeout: time.Second,
	}}
	search := AgentDecision{Type: DecisionAct, ReasonCode: "INITIAL_SEARCH", Actions: []AgentAction{
		{ID: "search-1", Tool: ToolSearchShops, Args: json.RawMessage(`{"query":"海淀咖啡"}`)},
	}}
	first, err := executor.Execute(context.Background(), state, search)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.NewCandidates) != 2 || state.Candidates[7].Name != "七号咖啡" {
		t.Fatalf("candidate registry not updated: batch=%+v state=%+v", first, state.Candidates)
	}

	evidence := AgentDecision{Type: DecisionAct, ReasonCode: "MISSING_EVIDENCE", Actions: []AgentAction{
		{ID: "detail-7", Tool: ToolGetShop, Args: json.RawMessage(`{"shop_id":7}`), DependsOn: []string{"search-1"}},
		{ID: "reviews-7", Tool: ToolListShopBlogs, Args: json.RawMessage(`{"shop_id":7,"limit":5}`), DependsOn: []string{"search-1"}},
	}}
	if _, err := executor.Execute(context.Background(), state, evidence); err != nil {
		t.Fatal(err)
	}
	candidate := state.Candidates[7]
	if !candidate.DetailsLoaded || candidate.ReviewPages != 1 {
		t.Fatalf("evidence state not updated: %+v", candidate)
	}
	if runner.concurrency() < 2 {
		t.Fatalf("independent evidence actions were not concurrent; max=%d", runner.concurrency())
	}
	if state.Budget.ToolCalls != 3 || state.Budget.ToolAttempts != 3 || state.Budget.SearchRounds != 1 {
		t.Fatalf("unexpected budgets: %+v", state.Budget)
	}
}

func TestParallelActionExecutorRetriesTransientFailure(t *testing.T) {
	state := newRuntimeTestState(t)
	runner := &structuredRunnerStub{failFirstQuery: "retry"}
	executor := &ParallelActionExecutor{Tools: runner, Config: ParallelExecutorConfig{
		MaxParallel: 1, MaxAttemptsPerAction: 2, ToolTimeout: time.Second,
	}}
	decision := AgentDecision{Type: DecisionAct, ReasonCode: "INITIAL_SEARCH", Actions: []AgentAction{
		{ID: "search-retry", Tool: ToolSearchShops, Args: json.RawMessage(`{"query":"retry coffee"}`)},
	}}
	if _, err := executor.Execute(context.Background(), state, decision); err != nil {
		t.Fatal(err)
	}
	record := state.Actions["search-retry"]
	if record.Status != ActionSucceeded || len(record.Attempts) != 2 || runner.callCount() != 2 {
		t.Fatalf("retry record=%+v calls=%d", record, runner.callCount())
	}
	if state.Budget.ToolAttempts != 2 || state.Budget.ToolCalls != 1 {
		t.Fatalf("retry budgets=%+v", state.Budget)
	}
}

func TestParallelActionExecutorBoundsToolTimeoutAndRetry(t *testing.T) {
	state := newRuntimeTestState(t)
	runner := &timeoutStructuredRunner{}
	executor := &ParallelActionExecutor{Tools: runner, Config: ParallelExecutorConfig{
		MaxParallel: 1, MaxAttemptsPerAction: 2, ToolTimeout: 15 * time.Millisecond,
	}}
	decision := AgentDecision{Type: DecisionAct, ReasonCode: "TIMEOUT_TEST", Actions: []AgentAction{{
		ID: "search-timeout", Tool: ToolSearchShops, Args: json.RawMessage(`{"query":"slow"}`),
	}}}
	started := time.Now()
	if _, err := executor.Execute(context.Background(), state, decision); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("tool timeout was not bounded: %v", elapsed)
	}
	record := state.Actions["search-timeout"]
	if record.Status != ActionFailed || record.Result.ErrorCode != ErrToolTimeout || len(record.Attempts) != 2 || runner.callCount() != 2 {
		t.Fatalf("timeout retry record=%+v calls=%d", record, runner.callCount())
	}
	if state.Budget.ToolAttempts != 2 || state.Budget.ToolCalls != 0 {
		t.Fatalf("timeout retry budgets=%+v", state.Budget)
	}
}

func TestParallelActionExecutorSkipsDuplicateAndFailedDependency(t *testing.T) {
	state := newRuntimeTestState(t)
	runner := &structuredRunnerStub{failFirstQuery: "always-fail"}
	executor := &ParallelActionExecutor{Tools: runner, Config: ParallelExecutorConfig{
		MaxParallel: 2, MaxAttemptsPerAction: 1, ToolTimeout: time.Second,
	}}
	duplicate := AgentDecision{Type: DecisionAct, ReasonCode: "DUPLICATE_TEST", Actions: []AgentAction{
		{ID: "search-a", Tool: ToolSearchShops, Args: json.RawMessage(`{"query":"same"}`)},
		{ID: "search-b", Tool: ToolSearchShops, Args: json.RawMessage(`{"query":"same"}`)},
	}}
	if _, err := executor.Execute(context.Background(), state, duplicate); err != nil {
		t.Fatal(err)
	}
	if state.Actions["search-b"].Result.ErrorCode != ErrToolDuplicate || runner.callCount() != 1 {
		t.Fatalf("duplicate was executed: record=%+v calls=%d", state.Actions["search-b"], runner.callCount())
	}

	failed := AgentDecision{Type: DecisionAct, ReasonCode: "DEPENDENCY_TEST", Actions: []AgentAction{
		{ID: "search-fail", Tool: ToolSearchShops, Args: json.RawMessage(`{"query":"always-fail"}`)},
		{ID: "search-after", Tool: ToolSearchShops, Args: json.RawMessage(`{"query":"after"}`), DependsOn: []string{"search-fail"}},
	}}
	if _, err := executor.Execute(context.Background(), state, failed); err != nil {
		t.Fatal(err)
	}
	if state.Actions["search-fail"].Status != ActionFailed || state.Actions["search-after"].Result.ErrorCode != ErrToolDependency {
		t.Fatalf("dependency states: fail=%+v after=%+v", state.Actions["search-fail"], state.Actions["search-after"])
	}
}

func TestParallelActionExecutorEnforcesCandidatePrecondition(t *testing.T) {
	state := newRuntimeTestState(t)
	runner := &structuredRunnerStub{}
	executor := &ParallelActionExecutor{Tools: runner, Config: DefaultParallelExecutorConfig()}
	decision := AgentDecision{Type: DecisionAct, ReasonCode: "INVALID_CANDIDATE", Actions: []AgentAction{
		{ID: "detail-404", Tool: ToolGetShop, Args: json.RawMessage(`{"shop_id":404}`)},
	}}
	if _, err := executor.Execute(context.Background(), state, decision); err != nil {
		t.Fatal(err)
	}
	result := state.Actions["detail-404"].Result
	if result.Status != ActionSkipped || result.ErrorCode != ErrToolNotAllowed || runner.callCount() != 0 {
		t.Fatalf("candidate guard result=%+v calls=%d", result, runner.callCount())
	}
}

func TestParallelActionExecutorRequiresServerIssuedReviewCursor(t *testing.T) {
	state := newRuntimeTestState(t)
	state.Candidates[7] = CandidateState{
		ShopID: 7, Rank: 1, RetrievalRank: 1, SourceActionID: "search-1",
		ReviewPages: 1, ReviewCursor: "next-page",
	}
	runner := &structuredRunnerStub{}
	executor := &ParallelActionExecutor{Tools: runner, Config: DefaultParallelExecutorConfig()}
	wrong := AgentDecision{Type: DecisionAct, ReasonCode: "CONTINUE_REVIEWS", Actions: []AgentAction{
		{ID: "reviews-wrong", Tool: ToolListShopBlogs, Args: json.RawMessage(`{"shop_id":7,"cursor":"invented"}`)},
	}}
	if _, err := executor.Execute(context.Background(), state, wrong); err != nil {
		t.Fatal(err)
	}
	if got := state.Actions["reviews-wrong"].Result; got.Status != ActionSkipped || got.ErrorCode != ErrToolInvalidArgs {
		t.Fatalf("wrong cursor result=%+v", got)
	}
	if runner.callCount() != 0 {
		t.Fatalf("wrong cursor reached tool runner: %d", runner.callCount())
	}

	correct := AgentDecision{Type: DecisionAct, ReasonCode: "CONTINUE_REVIEWS", Actions: []AgentAction{
		{ID: "reviews-next", Tool: ToolListShopBlogs, Args: json.RawMessage(`{"shop_id":7,"cursor":"next-page"}`)},
	}}
	if _, err := executor.Execute(context.Background(), state, correct); err != nil {
		t.Fatal(err)
	}
	if state.Actions["reviews-next"].Status != ActionSucceeded || state.Candidates[7].ReviewPages != 2 {
		t.Fatalf("next page state=%+v", state.Candidates[7])
	}
}
