package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func newRuntimeTestState(t *testing.T) *AgentState {
	t.Helper()
	intent := FallbackIntentSpec("海淀区安静办公咖啡", "agent_multistep")
	state, err := NewAgentState(
		"run-1", "otel-trace-1", "海淀区安静办公咖啡", intent,
		MemorySnapshot{Policy: MemoryReadOnly, ProfileSummary: "常在海淀"},
		DefaultRuntimeBudget(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestRuntimeBudgetFromEnvUsesV2DefaultsAndOverrides(t *testing.T) {
	t.Setenv("AGENT_REACT_MAX_TURNS", "6")
	t.Setenv("AGENT_REACT_MAX_REVIEW_PAGES", "3")
	t.Setenv("AGENT_REACT_MAX_NO_NOVELTY_ROUNDS", "0")
	budget := RuntimeBudgetFromEnv()
	if budget.MaxTurns != 6 || budget.MaxReviewPagesPerShop != 3 || budget.MaxNoNoveltyRounds != 0 {
		t.Fatalf("budget=%+v", budget)
	}
}

func TestAgentStateRoundTripKeepsDurableControlState(t *testing.T) {
	state := newRuntimeTestState(t)
	state.Candidates[7] = CandidateState{ShopID: 7, Rank: 1, SourceActionID: "search-1"}
	state.Actions["search-1"] = ActionRecord{
		Action: AgentAction{ID: "search-1", Tool: ToolSearchShops, Args: json.RawMessage(`{"query":"咖啡"}`)},
		Status: ActionSucceeded,
	}
	state.ActionOrder = []string{"search-1"}
	state.Evidence = EvidenceSnapshot{Shops: map[int64]ShopEvidenceSnapshot{
		7: {ShopID: 7, Name: "七号咖啡", DiscoveredBy: ToolSearchShops},
	}}

	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var restored AgentState
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatal(err)
	}
	if restored.Question != state.Question || restored.IntentSource != "fallback" || restored.Candidates[7].ShopID != 7 {
		t.Fatalf("state did not round-trip: %+v", restored)
	}
	if restored.Evidence.Shops[7].Name != "七号咖啡" {
		t.Fatalf("evidence did not round-trip: %+v", restored.Evidence)
	}
}

func TestValidateDecisionRejectsUnknownToolDependencyAndCycle(t *testing.T) {
	state := newRuntimeTestState(t)
	tests := []struct {
		name     string
		decision AgentDecision
		contains string
	}{
		{
			name: "unknown tool",
			decision: AgentDecision{Type: DecisionAct, ReasonCode: "TEST", Actions: []AgentAction{
				{ID: "a1", Tool: "browse_web", Args: json.RawMessage(`{}`)},
			}},
			contains: "unsupported action tool",
		},
		{
			name: "unknown dependency",
			decision: AgentDecision{Type: DecisionAct, ReasonCode: "TEST", Actions: []AgentAction{
				{ID: "a1", Tool: ToolSearchShops, Args: json.RawMessage(`{"query":"咖啡"}`), DependsOn: []string{"missing"}},
			}},
			contains: "unknown action",
		},
		{
			name: "cycle",
			decision: AgentDecision{Type: DecisionAct, ReasonCode: "TEST", Actions: []AgentAction{
				{ID: "a1", Tool: ToolSearchShops, Args: json.RawMessage(`{"query":"咖啡"}`), DependsOn: []string{"a2"}},
				{ID: "a2", Tool: ToolSearchShops, Args: json.RawMessage(`{"query":"书店"}`), DependsOn: []string{"a1"}},
			}},
			contains: "dependency cycle",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := state.ValidateDecision(tc.decision)
			if err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("error=%v, want containing %q", err, tc.contains)
			}
		})
	}
}

func TestFinishRequiresSearchAndAvailableReviewEvidence(t *testing.T) {
	state := newRuntimeTestState(t)
	finish := AgentDecision{Type: DecisionFinish, ReasonCode: "EVIDENCE_SUFFICIENT"}
	if err := state.ValidateDecision(finish); err == nil || !strings.Contains(err.Error(), "successful search") {
		t.Fatalf("initial finish error=%v", err)
	}
	state.Actions["search"] = ActionRecord{
		Action: AgentAction{ID: "search", Tool: ToolSearchShops, Args: json.RawMessage(`{"query":"咖啡"}`)},
		Status: ActionSucceeded,
	}
	state.ActionOrder = []string{"search"}
	state.Candidates[7] = CandidateState{
		ShopID: 7, SourceActionID: "search", ReviewPages: 1, ReviewCursor: "next",
	}
	state.Gaps = []EvidenceGap{{
		ShopID: 7, Requirement: "安静办公", EvidenceType: ToolListShopBlogs,
		Status: EvidenceUnknown,
	}}
	if err := state.ValidateDecision(finish); err == nil || !strings.Contains(err.Error(), "review evidence") {
		t.Fatalf("premature finish error=%v", err)
	}
	candidate := state.Candidates[7]
	candidate.ReviewCursor = ""
	state.Candidates[7] = candidate
	if err := state.ValidateDecision(finish); err != nil {
		t.Fatalf("exhausted evidence should permit no-result finish: %v", err)
	}
}

func TestValidateDecisionAcceptsParallelEvidenceActions(t *testing.T) {
	state := newRuntimeTestState(t)
	state.Candidates[7] = CandidateState{ShopID: 7, SourceActionID: "search-1"}
	state.Actions["search-1"] = ActionRecord{Status: ActionSucceeded}
	decision := AgentDecision{Type: DecisionAct, ReasonCode: "MISSING_EVIDENCE", Actions: []AgentAction{
		{ID: "detail-7", Tool: ToolGetShop, Args: json.RawMessage(`{"shop_id":7}`), DependsOn: []string{"search-1"}},
		{ID: "reviews-7", Tool: ToolListShopBlogs, Args: json.RawMessage(`{"shop_id":7,"limit":5}`), DependsOn: []string{"search-1"}},
	}}
	if err := state.ValidateDecision(decision); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceSnapshotRestoresLedgerWithoutAliasing(t *testing.T) {
	ledger := NewEvidenceLedger()
	ledger.DiscoverFromSearch(7, "七号咖啡", map[string]any{"area": "海淀区"})
	if err := ledger.RecordBlogEvidence(7, []int64{91}, []string{"安静，有插座"}); err != nil {
		t.Fatal(err)
	}
	snapshot := ledger.Snapshot()
	restored := NewEvidenceLedgerFromSnapshot(snapshot)

	if !restored.IsDiscovered(7) || !restored.HasBlogEvidence(7) {
		t.Fatalf("restored ledger missing evidence: %+v", restored.Get(7))
	}
	snapshot.Shops[7] = ShopEvidenceSnapshot{ShopID: 7, Name: "被修改"}
	if restored.Get(7).Name != "七号咖啡" {
		t.Fatal("restored ledger aliases checkpoint memory")
	}
}
