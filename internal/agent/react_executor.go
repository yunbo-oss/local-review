package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

type StructuredToolRunner interface {
	ExecuteStructured(ctx context.Context, name, argsJSON string) ToolResult
}

type evidenceSnapshotter interface {
	EvidenceSnapshot() EvidenceSnapshot
}

type ParallelExecutorConfig struct {
	MaxParallel          int
	MaxAttemptsPerAction int
	ToolTimeout          time.Duration
	RetryBackoff         time.Duration
}

func DefaultParallelExecutorConfig() ParallelExecutorConfig {
	return ParallelExecutorConfig{
		MaxParallel: DefaultMaxToolsPerTurn, MaxAttemptsPerAction: 2,
		ToolTimeout: DefaultToolTimeout, RetryBackoff: 25 * time.Millisecond,
	}
}

type BatchExecution struct {
	Decision      AgentDecision    `json:"decision"`
	Results       []ToolResult     `json:"results"`
	NewCandidates []int64          `json:"new_candidates,omitempty"`
	Evidence      EvidenceSnapshot `json:"evidence"`
	StartedAt     int64            `json:"started_at"`
	CompletedAt   int64            `json:"completed_at"`
}

// ParallelActionExecutor executes the ready frontier of a decision DAG in
// parallel. It mutates AgentState only between waves, never from tool goroutines.
type ParallelActionExecutor struct {
	Tools  StructuredToolRunner
	Config ParallelExecutorConfig
}

func (e *ParallelActionExecutor) Execute(ctx context.Context, state *AgentState, decision AgentDecision) (BatchExecution, error) {
	batch := BatchExecution{Decision: decision, StartedAt: time.Now().UnixMilli()}
	if e == nil || e.Tools == nil {
		return batch, fmt.Errorf("parallel executor is not configured")
	}
	if decision.Type != DecisionAct {
		return batch, fmt.Errorf("parallel executor only accepts act decisions")
	}
	if err := state.ValidateDecision(decision); err != nil {
		return batch, err
	}
	cfg := e.Config
	if cfg.MaxParallel <= 0 {
		cfg = DefaultParallelExecutorConfig()
	}
	if cfg.MaxParallel > state.Budget.MaxParallelTools {
		cfg.MaxParallel = state.Budget.MaxParallelTools
	}
	if cfg.MaxAttemptsPerAction <= 0 {
		cfg.MaxAttemptsPerAction = 1
	}
	if cfg.ToolTimeout <= 0 {
		cfg.ToolTimeout = DefaultToolTimeout
	}

	state.Status = RuntimeRunning
	for _, action := range decision.Actions {
		state.Actions[action.ID] = ActionRecord{
			Action: cloneAction(action), Status: ActionPending, Turn: state.Budget.Turns,
		}
		state.ActionOrder = append(state.ActionOrder, action.ID)
	}
	state.Revision++
	state.UpdatedAt = time.Now().UnixMilli()

	beforeCandidateIDs := make(map[int64]struct{}, len(state.Candidates))
	for id := range state.Candidates {
		beforeCandidateIDs[id] = struct{}{}
	}
	beforeEvidence := stateEvidenceNovelty(state)
	remaining := make(map[string]AgentAction, len(decision.Actions))
	for _, action := range decision.Actions {
		remaining[action.ID] = action
	}
	ledger := &executionBudgetLedger{state: state}

	for len(remaining) > 0 {
		ready, blocked := readyActionFrontier(state, decision.Actions, remaining)
		for _, action := range blocked {
			result := skippedToolResult(action, ErrToolDependency, "dependency did not succeed")
			applyActionResult(state, action, []ToolResult{result})
			batch.Results = append(batch.Results, result)
			delete(remaining, action.ID)
		}
		if len(ready) == 0 {
			if len(blocked) > 0 {
				continue
			}
			return batch, fmt.Errorf("decision DAG made no progress")
		}

		runnable := make([]AgentAction, 0, len(ready))
		for _, action := range ready {
			key := action.Tool + "|" + CanonicalArgs(string(action.Args))
			if previous, duplicate := state.SeenCalls[key]; duplicate {
				_ = ledger.reserveAttempt()
				result := skippedToolResult(action, ErrToolDuplicate, "duplicate of "+previous)
				applyActionResult(state, action, []ToolResult{result})
				batch.Results = append(batch.Results, result)
				delete(remaining, action.ID)
				continue
			}
			if result := preflightAction(state, action); result != nil {
				_ = ledger.reserveAttempt()
				applyActionResult(state, action, []ToolResult{*result})
				batch.Results = append(batch.Results, *result)
				delete(remaining, action.ID)
				continue
			}
			if !ledger.reserveCallSlot() {
				_ = ledger.reserveAttempt()
				result := skippedToolResult(action, ErrMaxToolCalls, "tool-call budget exhausted")
				applyActionResult(state, action, []ToolResult{result})
				batch.Results = append(batch.Results, result)
				delete(remaining, action.ID)
				continue
			}
			state.SeenCalls[key] = action.ID
			runnable = append(runnable, action)
		}

		attempts := make([][]ToolResult, len(runnable))
		g, gctx := errgroup.WithContext(ctx)
		sem := make(chan struct{}, cfg.MaxParallel)
		for i := range runnable {
			i := i
			g.Go(func() error {
				sem <- struct{}{}
				defer func() { <-sem }()
				attempts[i] = e.executeAction(gctx, runnable[i], cfg, ledger)
				return nil
			})
		}
		_ = g.Wait()

		for i, action := range runnable {
			results := attempts[i]
			if len(results) == 0 {
				results = []ToolResult{skippedToolResult(action, ErrMaxToolCalls, "tool-attempt budget exhausted")}
			}
			ledger.finishCallSlot(results[len(results)-1].Status == ActionSucceeded)
			applyActionResult(state, action, results)
			batch.Results = append(batch.Results, results[len(results)-1])
			delete(remaining, action.ID)
		}
		syncCandidatesFromResults(state, batch.Results)
		if snapshotter, ok := e.Tools.(evidenceSnapshotter); ok {
			state.Evidence = snapshotter.EvidenceSnapshot()
		}
	}

	syncCandidatesFromResults(state, batch.Results)
	if snapshotter, ok := e.Tools.(evidenceSnapshotter); ok {
		state.Evidence = snapshotter.EvidenceSnapshot()
	}
	for id := range state.Candidates {
		if _, existed := beforeCandidateIDs[id]; !existed {
			batch.NewCandidates = append(batch.NewCandidates, id)
		}
	}
	sort.Slice(batch.NewCandidates, func(i, j int) bool { return batch.NewCandidates[i] < batch.NewCandidates[j] })
	if len(batch.NewCandidates) > 0 || stateEvidenceNovelty(state) > beforeEvidence {
		state.Budget.NoNoveltyRounds = 0
	} else {
		state.Budget.NoNoveltyRounds++
	}
	state.Revision++
	state.UpdatedAt = time.Now().UnixMilli()
	batch.Evidence = state.Evidence
	batch.CompletedAt = state.UpdatedAt
	return batch, nil
}

func (e *ParallelActionExecutor) executeAction(ctx context.Context, action AgentAction, cfg ParallelExecutorConfig, ledger *executionBudgetLedger) (attempts []ToolResult) {
	actionCtx, actionSpan := StartActionSpan(ctx, action, ledger.state.Budget.Turns)
	defer func() { FinishActionSpan(actionSpan, attempts) }()
	for attempt := 1; attempt <= cfg.MaxAttemptsPerAction; attempt++ {
		if !ledger.reserveAttempt() {
			break
		}
		toolCtx, cancel := context.WithTimeout(actionCtx, cfg.ToolTimeout)
		result := e.Tools.ExecuteStructured(toolCtx, action.Tool, string(action.Args))
		cancel()
		result.ActionID = action.ID
		result.Tool = action.Tool
		if result.ArgsHash == "" {
			result.ArgsHash = toolArgsHash(string(action.Args))
		}
		attempts = append(attempts, result)
		if result.Status == ActionSucceeded || !retryableToolResult(result) || attempt == cfg.MaxAttemptsPerAction {
			break
		}
		if cfg.RetryBackoff > 0 {
			timer := time.NewTimer(cfg.RetryBackoff)
			select {
			case <-actionCtx.Done():
				timer.Stop()
				return attempts
			case <-timer.C:
			}
		}
	}
	return attempts
}

type executionBudgetLedger struct {
	mu            sync.Mutex
	state         *AgentState
	reservedCalls int
}

func (l *executionBudgetLedger) reserveCallSlot() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state.Budget.ToolCalls+l.reservedCalls >= l.state.Budget.MaxToolCalls {
		return false
	}
	l.reservedCalls++
	return true
}

func (l *executionBudgetLedger) finishCallSlot(success bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.reservedCalls > 0 {
		l.reservedCalls--
	}
	if success {
		l.state.Budget.ToolCalls++
	}
}

func (l *executionBudgetLedger) reserveAttempt() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state.Budget.ToolAttempts >= l.state.Budget.MaxToolAttempts {
		return false
	}
	l.state.Budget.ToolAttempts++
	return true
}

func readyActionFrontier(state *AgentState, ordered []AgentAction, remaining map[string]AgentAction) (ready, blocked []AgentAction) {
	for _, action := range ordered {
		if _, pending := remaining[action.ID]; !pending {
			continue
		}
		allSucceeded := true
		dependencyFailed := false
		for _, dependency := range action.DependsOn {
			record := state.Actions[dependency]
			switch record.Status {
			case ActionSucceeded:
			case ActionFailed, ActionSkipped:
				dependencyFailed = true
			default:
				allSucceeded = false
			}
		}
		if dependencyFailed {
			blocked = append(blocked, action)
		} else if allSucceeded {
			ready = append(ready, action)
		}
	}
	return ready, blocked
}

func preflightAction(state *AgentState, action AgentAction) *ToolResult {
	switch action.Tool {
	case ToolSearchShops:
		if state.Budget.SearchRounds >= state.Budget.MaxSearchRounds {
			result := skippedToolResult(action, ErrMaxSearchRounds, "search-round budget exhausted")
			return &result
		}
		state.Budget.SearchRounds++
	case ToolGetShop, ToolListShopBlogs:
		shopID, err := runtimeActionShopID(action.Args)
		if err != nil {
			result := skippedToolResult(action, ErrToolInvalidArgs, err.Error())
			return &result
		}
		candidate, ok := state.Candidates[shopID]
		if !ok || candidate.Rejected {
			result := skippedToolResult(action, ErrToolNotAllowed, "shop_id not in active candidate registry")
			return &result
		}
		if action.Tool == ToolListShopBlogs {
			if candidate.ReviewPages >= state.Budget.MaxReviewPagesPerShop {
				result := skippedToolResult(action, ErrMaxReviewPages, "review-page budget exhausted")
				return &result
			}
			var args reviewArgs
			if err := json.Unmarshal(action.Args, &args); err != nil {
				result := skippedToolResult(action, ErrToolInvalidArgs, "invalid review args")
				return &result
			}
			cursor := strings.TrimSpace(args.Cursor)
			switch {
			case candidate.ReviewPages == 0 && cursor != "":
				result := skippedToolResult(action, ErrToolInvalidArgs, "first review page must not set cursor")
				return &result
			case candidate.ReviewPages > 0 && candidate.ReviewCursor == "":
				result := skippedToolResult(action, ErrMaxReviewPages, "review pagination is exhausted")
				return &result
			case candidate.ReviewPages > 0 && cursor != candidate.ReviewCursor:
				result := skippedToolResult(action, ErrToolInvalidArgs, "cursor must match candidate review_cursor")
				return &result
			}
		}
	}
	return nil
}

func retryableToolResult(result ToolResult) bool {
	return result.Status == ActionFailed && (result.ErrorCode == ErrToolTimeout || result.ErrorCode == ErrToolExecution)
}

func skippedToolResult(action AgentAction, code, detail string) ToolResult {
	return ToolResult{
		ActionID: action.ID, Tool: action.Tool, ArgsHash: toolArgsHash(string(action.Args)),
		Status: ActionSkipped, ErrorCode: code, ErrorDetail: detail,
	}
}

func applyActionResult(state *AgentState, action AgentAction, attempts []ToolResult) {
	final := attempts[len(attempts)-1]
	record := state.Actions[action.ID]
	record.Action = cloneAction(action)
	record.AttemptNo = len(attempts)
	record.Attempts = append([]ToolResult(nil), attempts...)
	record.Result = final
	record.Status = final.Status
	state.Actions[action.ID] = record
	if final.Status != ActionSucceeded {
		return
	}
	if action.Tool == ToolGetShop || action.Tool == ToolListShopBlogs {
		if shopID, err := runtimeActionShopID(action.Args); err == nil {
			candidate := state.Candidates[shopID]
			if action.Tool == ToolGetShop {
				candidate.DetailsLoaded = true
			} else {
				candidate.ReviewPages++
				candidate.ReviewCursor = final.NextCursor
			}
			state.Candidates[shopID] = candidate
		}
	}
}

func runtimeActionShopID(raw json.RawMessage) (int64, error) {
	var args struct {
		ShopID int64 `json:"shop_id"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || args.ShopID <= 0 {
		return 0, fmt.Errorf("valid shop_id is required")
	}
	return args.ShopID, nil
}

func syncCandidatesFromResults(state *AgentState, results []ToolResult) {
	for _, result := range results {
		if result.Tool != ToolSearchShops || result.Status != ActionSucceeded {
			continue
		}
		names := searchCandidateNames(result.Output)
		for rank, id := range result.CandidateIDs {
			if id <= 0 {
				continue
			}
			candidate, exists := state.Candidates[id]
			if !exists && len(state.Candidates) >= state.Budget.MaxCandidates {
				continue
			}
			candidate.ShopID = id
			candidate.Rank = rank + 1
			candidate.RetrievalRank = rank + 1
			candidate.SourceActionID = result.ActionID
			if name := names[id]; name != "" {
				candidate.Name = name
			}
			state.Candidates[id] = candidate
		}
	}
}

func searchCandidateNames(raw string) map[int64]string {
	var items []struct {
		ShopID int64  `json:"shop_id"`
		Name   string `json:"name"`
	}
	_ = json.Unmarshal([]byte(raw), &items)
	out := make(map[int64]string, len(items))
	for _, item := range items {
		out[item.ShopID] = item.Name
	}
	return out
}

func evidenceNovelty(snapshot EvidenceSnapshot) int {
	total := 0
	for _, item := range snapshot.Shops {
		total += len(item.Fields) + len(item.BlogIDs)
		if item.Verified {
			total++
		}
	}
	return total
}

func stateEvidenceNovelty(state *AgentState) int {
	if state == nil {
		return 0
	}
	total := evidenceNovelty(state.Evidence)
	for _, candidate := range state.Candidates {
		if candidate.DetailsLoaded {
			total++
		}
		total += candidate.ReviewPages
	}
	return total
}

func cloneAction(action AgentAction) AgentAction {
	action.Args = append(json.RawMessage(nil), action.Args...)
	action.DependsOn = append([]string(nil), action.DependsOn...)
	return action
}

func actionFailureSummary(results []ToolResult) string {
	var parts []string
	for _, result := range results {
		if result.Status == ActionSucceeded {
			continue
		}
		parts = append(parts, result.ActionID+":"+result.ErrorCode)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
