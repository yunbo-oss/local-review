package main

// Expected 场景期望（来自 agent.v1.json）
type Expected struct {
	FilterContains     map[string]any `json:"filter_contains"`
	AllowedShopIDs     []int64        `json:"allowed_shop_ids"`
	ForbiddenShopIDs   []int64        `json:"forbidden_shop_ids"`
	ProfileAfter       map[string]any `json:"profile_after"`
	MaxSteps           int            `json:"max_steps"`
	MaxToolCalls       int            `json:"max_tool_calls"`
	ExpectNoResults    bool           `json:"expect_no_results"`
	ExpectGroundedness *bool          `json:"expect_groundedness"`
}

// OutcomeActual 一次运行的可观测结果
type OutcomeActual struct {
	Filter             map[string]any
	CitedShopIDs       []int64
	ObservedShopIDs    []int64
	ProfileAfter       map[string]any
	Steps              int
	ModelCalls         int
	ToolCalls          int
	DuplicateToolCalls int
	Answer             string
	LatencyMs          int64
	PromptTokens       int
	CompletionTokens   int
	Tokens             int
}

// GradeResult 单项评分
type GradeResult struct {
	Name    string   `json:"name"`
	Pass    bool     `json:"pass"`
	Reasons []string `json:"reasons,omitempty"`
}

// TrialAggregate 多 trial 汇总
type TrialAggregate struct {
	Trials      int     `json:"trials"`
	Successes   int     `json:"successes"`
	SuccessRate float64 `json:"success_rate"`
}

// AgentCaseFile golden 文件
type AgentCaseFile struct {
	Version string      `json:"version"`
	Cases   []AgentCase `json:"cases"`
}

// AgentCase 单条场景
type AgentCase struct {
	ID           string         `json:"id"`
	Split        string         `json:"split"`
	SetupProfile map[string]any `json:"setup_profile"`
	Turns        []struct {
		User string `json:"user"`
	} `json:"turns"`
	Expected Expected `json:"expected"`
	Tags     []string `json:"tags"`
	Trials   int      `json:"trials"`
	Evidence string   `json:"evidence"`
}
