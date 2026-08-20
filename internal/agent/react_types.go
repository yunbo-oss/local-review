package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const AgentStateSchemaVersion = 2

type MemoryPolicy string

const (
	MemoryNone                  MemoryPolicy = "none"
	MemoryReadOnly              MemoryPolicy = "read_only"
	MemoryWriteAfterSuccess     MemoryPolicy = "write_after_success"
	DefaultReactMaxTurns                     = 4
	DefaultReactMaxToolCalls                 = 10
	DefaultReactMaxToolAttempts              = 12
	DefaultReactMaxSearchRounds              = 2
	DefaultReactMaxReviewPages               = 2
	DefaultReactMaxCandidates                = 5
	DefaultReactNoNoveltyRounds              = 1
)

type RuntimeStatus string

const (
	RuntimeReady           RuntimeStatus = "READY"
	RuntimeRunning         RuntimeStatus = "RUNNING"
	RuntimeCompleted       RuntimeStatus = "COMPLETED"
	RuntimeNeedsClarify    RuntimeStatus = "NEEDS_CLARIFICATION"
	RuntimeFailed          RuntimeStatus = "FAILED"
	RuntimeCancelled       RuntimeStatus = "CANCELLED"
	RuntimeBudgetExhausted RuntimeStatus = "BUDGET_EXHAUSTED"
)

type DecisionType string

const (
	DecisionAct     DecisionType = "act"
	DecisionFinish  DecisionType = "finish"
	DecisionClarify DecisionType = "clarify"
)

// AgentDecision is deliberately structured and does not expose or persist a
// model chain-of-thought. ReasonCode is a short auditable control-plane reason.
type AgentDecision struct {
	Type          DecisionType  `json:"type"`
	ReasonCode    string        `json:"reason_code"`
	Actions       []AgentAction `json:"actions,omitempty"`
	Clarification string        `json:"clarification,omitempty"`
}

type AgentAction struct {
	ID        string          `json:"id"`
	Tool      string          `json:"tool"`
	Args      json.RawMessage `json:"args"`
	DependsOn []string        `json:"depends_on,omitempty"`
}

type ActionStatus string

const (
	ActionPending   ActionStatus = "PENDING"
	ActionRunning   ActionStatus = "RUNNING"
	ActionSucceeded ActionStatus = "SUCCEEDED"
	ActionFailed    ActionStatus = "FAILED"
	ActionSkipped   ActionStatus = "SKIPPED"
)

// ToolResult is the V2 tool contract. Failures are first-class values instead
// of successful calls whose output happens to contain {"error": ...}.
type ToolResult struct {
	ActionID     string       `json:"action_id"`
	Tool         string       `json:"tool"`
	ArgsHash     string       `json:"args_hash"`
	Status       ActionStatus `json:"status"`
	Output       string       `json:"output,omitempty"`
	ErrorCode    string       `json:"error_code,omitempty"`
	ErrorDetail  string       `json:"error_detail,omitempty"`
	LatencyMs    int64        `json:"latency_ms"`
	ResultCount  int          `json:"result_count"`
	CandidateIDs []int64      `json:"candidate_ids,omitempty"`
	NextCursor   string       `json:"next_cursor,omitempty"`
}

type ActionRecord struct {
	Action    AgentAction  `json:"action"`
	Status    ActionStatus `json:"status"`
	Turn      int          `json:"turn"`
	AttemptNo int          `json:"attempt_no"`
	Attempts  []ToolResult `json:"attempts,omitempty"`
	Result    ToolResult   `json:"result"`
}

type CandidateState struct {
	ShopID          int64   `json:"shop_id"`
	Name            string  `json:"name,omitempty"`
	Rank            int     `json:"rank,omitempty"`
	RetrievalRank   int     `json:"retrieval_rank,omitempty"`
	SourceActionID  string  `json:"source_action_id"`
	DetailsLoaded   bool    `json:"details_loaded"`
	ReviewPages     int     `json:"review_pages"`
	ReviewCursor    string  `json:"review_cursor,omitempty"`
	EvidenceScore   float64 `json:"evidence_score"`
	Rejected        bool    `json:"rejected"`
	RejectionReason string  `json:"rejection_reason,omitempty"`
}

type EvidenceGapStatus string

const (
	EvidenceMissing      EvidenceGapStatus = "missing"
	EvidenceUnknown      EvidenceGapStatus = "unknown"
	EvidenceSupported    EvidenceGapStatus = "supported"
	EvidenceContradicted EvidenceGapStatus = "contradicted"
)

type EvidenceGap struct {
	ShopID       int64             `json:"shop_id,omitempty"`
	Requirement  string            `json:"requirement"`
	EvidenceType string            `json:"evidence_type"`
	Status       EvidenceGapStatus `json:"status"`
	Confidence   float64           `json:"confidence"`
	ReasonCode   string            `json:"reason_code,omitempty"`
}

type MemoryFact struct {
	Key        string  `json:"key"`
	Value      string  `json:"value"`
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
	UpdatedAt  int64   `json:"updated_at"`
}

type MemorySnapshot struct {
	Policy         MemoryPolicy `json:"policy"`
	ProfileSummary string       `json:"profile_summary,omitempty"`
	SessionSummary string       `json:"session_summary,omitempty"`
	RelevantFacts  []MemoryFact `json:"relevant_facts,omitempty"`
}

type RuntimeBudget struct {
	MaxTurns              int `json:"max_turns"`
	MaxToolCalls          int `json:"max_tool_calls"`
	MaxToolAttempts       int `json:"max_tool_attempts"`
	MaxParallelTools      int `json:"max_parallel_tools"`
	MaxSearchRounds       int `json:"max_search_rounds"`
	MaxReviewPagesPerShop int `json:"max_review_pages_per_shop"`
	MaxCandidates         int `json:"max_candidates"`
	MaxNoNoveltyRounds    int `json:"max_no_novelty_rounds"`

	Turns           int `json:"turns"`
	ToolCalls       int `json:"tool_calls"`
	ToolAttempts    int `json:"tool_attempts"`
	SearchRounds    int `json:"search_rounds"`
	NoNoveltyRounds int `json:"no_novelty_rounds"`
}

func DefaultRuntimeBudget() RuntimeBudget {
	return RuntimeBudget{
		MaxTurns: DefaultReactMaxTurns, MaxToolCalls: DefaultReactMaxToolCalls,
		MaxToolAttempts:       DefaultReactMaxToolAttempts,
		MaxParallelTools:      DefaultMaxToolsPerTurn,
		MaxSearchRounds:       DefaultReactMaxSearchRounds,
		MaxReviewPagesPerShop: DefaultReactMaxReviewPages,
		MaxCandidates:         DefaultReactMaxCandidates,
		MaxNoNoveltyRounds:    DefaultReactNoNoveltyRounds,
	}
}

// RuntimeBudgetFromEnv uses V2-specific knobs so replaying the V1 baseline is
// not silently changed by the larger Parallel ReAct budget.
func RuntimeBudgetFromEnv() RuntimeBudget {
	budget := DefaultRuntimeBudget()
	positive := []struct {
		name   string
		target *int
	}{
		{"AGENT_REACT_MAX_TURNS", &budget.MaxTurns},
		{"AGENT_REACT_MAX_TOOL_CALLS", &budget.MaxToolCalls},
		{"AGENT_REACT_MAX_TOOL_ATTEMPTS", &budget.MaxToolAttempts},
		{"AGENT_REACT_MAX_PARALLEL_TOOLS", &budget.MaxParallelTools},
		{"AGENT_REACT_MAX_SEARCH_ROUNDS", &budget.MaxSearchRounds},
		{"AGENT_REACT_MAX_REVIEW_PAGES", &budget.MaxReviewPagesPerShop},
		{"AGENT_REACT_MAX_CANDIDATES", &budget.MaxCandidates},
	}
	for _, item := range positive {
		if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(item.name))); err == nil && value > 0 {
			*item.target = value
		}
	}
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("AGENT_REACT_MAX_NO_NOVELTY_ROUNDS"))); err == nil && value >= 0 {
		budget.MaxNoNoveltyRounds = value
	}
	return budget
}

type AgentState struct {
	SchemaVersion  int            `json:"schema_version"`
	RunID          string         `json:"run_id"`
	TraceID        string         `json:"trace_id,omitempty"`
	Revision       int64          `json:"revision"`
	Question       string         `json:"question"`
	Intent         IntentSpec     `json:"intent"`
	IntentSource   string         `json:"intent_source,omitempty"`
	Memory         MemorySnapshot `json:"memory"`
	Status         RuntimeStatus  `json:"status"`
	StopReason     string         `json:"stop_reason,omitempty"`
	AnswerVerified bool           `json:"answer_verified"`

	Candidates  map[int64]CandidateState `json:"candidates"`
	Evidence    EvidenceSnapshot         `json:"evidence"`
	Gaps        []EvidenceGap            `json:"gaps,omitempty"`
	Actions     map[string]ActionRecord  `json:"actions"`
	ActionOrder []string                 `json:"action_order"`
	SeenCalls   map[string]string        `json:"seen_calls"`
	Budget      RuntimeBudget            `json:"budget"`

	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

func NewAgentState(runID, traceID, question string, intent IntentSpec, memory MemorySnapshot, budget RuntimeBudget) (*AgentState, error) {
	if budget.MaxTurns <= 0 {
		budget = DefaultRuntimeBudget()
	}
	now := time.Now().UnixMilli()
	state := &AgentState{
		SchemaVersion: AgentStateSchemaVersion,
		RunID:         strings.TrimSpace(runID), TraceID: strings.TrimSpace(traceID),
		Question: strings.TrimSpace(question), Intent: intent, IntentSource: intent.Source,
		Memory: memory, Status: RuntimeReady,
		Candidates: map[int64]CandidateState{}, Evidence: EvidenceSnapshot{Shops: map[int64]ShopEvidenceSnapshot{}},
		Actions: map[string]ActionRecord{}, SeenCalls: map[string]string{}, Budget: budget,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := state.Validate(); err != nil {
		return nil, err
	}
	return state, nil
}

func (s *AgentState) Validate() error {
	if s == nil {
		return fmt.Errorf("agent state is nil")
	}
	if s.SchemaVersion != AgentStateSchemaVersion {
		return fmt.Errorf("unsupported agent state schema %d", s.SchemaVersion)
	}
	if strings.TrimSpace(s.RunID) == "" || strings.TrimSpace(s.Question) == "" {
		return fmt.Errorf("run_id and question are required")
	}
	if s.Budget.MaxTurns <= 0 || s.Budget.MaxToolCalls <= 0 || s.Budget.MaxToolAttempts <= 0 ||
		s.Budget.MaxParallelTools <= 0 || s.Budget.MaxSearchRounds <= 0 ||
		s.Budget.MaxReviewPagesPerShop <= 0 || s.Budget.MaxCandidates <= 0 ||
		s.Budget.MaxNoNoveltyRounds < 0 {
		return fmt.Errorf("runtime budget limits must be positive")
	}
	if s.Candidates == nil || s.Actions == nil || s.SeenCalls == nil {
		return fmt.Errorf("agent state maps must be initialized")
	}
	return nil
}

var actionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func (s *AgentState) ValidateDecision(decision AgentDecision) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.Status != RuntimeReady && s.Status != RuntimeRunning {
		return fmt.Errorf("state %s does not accept decisions", s.Status)
	}
	if strings.TrimSpace(decision.ReasonCode) == "" || len(decision.ReasonCode) > 80 {
		return fmt.Errorf("decision reason_code is required and must be <= 80 bytes")
	}
	switch decision.Type {
	case DecisionFinish:
		if len(decision.Actions) != 0 || strings.TrimSpace(decision.Clarification) != "" {
			return fmt.Errorf("finish decision cannot contain actions or clarification")
		}
		return s.validateFinishPreconditions()
	case DecisionClarify:
		if len(decision.Actions) != 0 || strings.TrimSpace(decision.Clarification) == "" {
			return fmt.Errorf("clarify decision requires clarification and no actions")
		}
		return nil
	case DecisionAct:
		if len(decision.Actions) == 0 {
			return fmt.Errorf("act decision requires at least one action")
		}
		if len(decision.Actions) > s.Budget.MaxToolAttempts-s.Budget.ToolAttempts {
			return fmt.Errorf("decision exceeds remaining tool-attempt budget")
		}
	default:
		return fmt.Errorf("unsupported decision type %q", decision.Type)
	}

	batch := make(map[string]AgentAction, len(decision.Actions))
	for _, action := range decision.Actions {
		if !actionIDPattern.MatchString(action.ID) {
			return fmt.Errorf("invalid action id %q", action.ID)
		}
		if _, exists := s.Actions[action.ID]; exists {
			return fmt.Errorf("action id %q already exists", action.ID)
		}
		if _, duplicate := batch[action.ID]; duplicate {
			return fmt.Errorf("duplicate action id %q", action.ID)
		}
		if !allowedRuntimeTool(action.Tool) {
			return fmt.Errorf("unsupported action tool %q", action.Tool)
		}
		if !validJSONObject(action.Args) {
			return fmt.Errorf("action %s args must be one JSON object", action.ID)
		}
		batch[action.ID] = action
	}
	for _, action := range decision.Actions {
		seenDependencies := map[string]struct{}{}
		for _, dependency := range action.DependsOn {
			if dependency == action.ID {
				return fmt.Errorf("action %s cannot depend on itself", action.ID)
			}
			if _, duplicate := seenDependencies[dependency]; duplicate {
				return fmt.Errorf("action %s has duplicate dependency %s", action.ID, dependency)
			}
			seenDependencies[dependency] = struct{}{}
			if _, inBatch := batch[dependency]; !inBatch {
				if _, historical := s.Actions[dependency]; !historical {
					return fmt.Errorf("action %s depends on unknown action %s", action.ID, dependency)
				}
			}
		}
	}
	if cycle := decisionCycle(batch); len(cycle) > 0 {
		return fmt.Errorf("decision contains dependency cycle: %s", strings.Join(cycle, " -> "))
	}
	return nil
}

func (s *AgentState) validateFinishPreconditions() error {
	searched := false
	for _, actionID := range s.ActionOrder {
		record := s.Actions[actionID]
		if record.Action.Tool == ToolSearchShops && record.Status == ActionSucceeded {
			searched = true
			break
		}
	}
	if !searched {
		return fmt.Errorf("finish requires at least one successful search")
	}
	if len(s.Candidates) == 0 || len(s.Gaps) == 0 {
		return nil
	}
	byCandidate := make(map[int64][]EvidenceGap)
	for _, gap := range s.Gaps {
		if gap.ShopID > 0 {
			byCandidate[gap.ShopID] = append(byCandidate[gap.ShopID], gap)
		}
	}
	for shopID, gaps := range byCandidate {
		candidate := s.Candidates[shopID]
		if candidate.Rejected {
			continue
		}
		allSupported := true
		for _, gap := range gaps {
			if gap.Status != EvidenceSupported {
				allSupported = false
				break
			}
		}
		if allSupported {
			return nil
		}
	}
	if s.Budget.ToolCalls >= s.Budget.MaxToolCalls || s.Budget.ToolAttempts >= s.Budget.MaxToolAttempts {
		return nil
	}
	for shopID, gaps := range byCandidate {
		candidate := s.Candidates[shopID]
		if candidate.Rejected {
			continue
		}
		for _, gap := range gaps {
			switch gap.EvidenceType {
			case ToolGetShop:
				if gap.Status == EvidenceMissing && !candidate.DetailsLoaded {
					return fmt.Errorf("finish rejected: candidate %d still needs shop details", shopID)
				}
			case ToolListShopBlogs:
				if candidate.ReviewPages == 0 ||
					(candidate.ReviewCursor != "" && candidate.ReviewPages < s.Budget.MaxReviewPagesPerShop) {
					return fmt.Errorf("finish rejected: candidate %d still has review evidence to inspect", shopID)
				}
			}
		}
	}
	return nil
}

func allowedRuntimeTool(name string) bool {
	switch strings.TrimSpace(name) {
	case ToolSearchShops, ToolGetShop, ToolListShopBlogs:
		return true
	default:
		return false
	}
}

func validJSONObject(raw json.RawMessage) bool {
	var value map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || value == nil {
		return false
	}
	compact, err := json.Marshal(value)
	return err == nil && len(compact) > 1
}

func decisionCycle(actions map[string]AgentAction) []string {
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var path []string
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			path = append(path, id)
			return true
		}
		if visited[id] {
			return false
		}
		visiting[id] = true
		path = append(path, id)
		for _, dependency := range actions[id].DependsOn {
			if _, ok := actions[dependency]; ok && visit(dependency) {
				return true
			}
		}
		visiting[id] = false
		visited[id] = true
		path = path[:len(path)-1]
		return false
	}
	ids := make([]string, 0, len(actions))
	for id := range actions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		path = path[:0]
		if visit(id) {
			return append([]string(nil), path...)
		}
	}
	return nil
}
