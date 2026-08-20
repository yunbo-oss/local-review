package model

import "time"

const AGENT_RUN_TABLE = "agent_runs"

// AgentRun 一次推荐运行摘要（隐私安全：不存完整问句/评价正文）
type AgentRun struct {
	ID                  int64      `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	RunKey              string     `gorm:"column:run_key;uniqueIndex;size:64;not null" json:"run_key"`
	TraceID             string     `gorm:"column:trace_id;index;size:64;not null" json:"trace_id"`
	SpanID              string     `gorm:"column:span_id;index;size:32" json:"span_id"`
	UserID              int64      `gorm:"column:user_id;index;not null" json:"user_id"`
	SessionID           string     `gorm:"column:session_id;size:128;not null" json:"session_id"`
	Status              string     `gorm:"column:status;size:32;not null;index" json:"status"`
	Model               string     `gorm:"column:model;size:128" json:"model"`
	PolicyVersion       string     `gorm:"column:policy_version;size:64" json:"policy_version"`
	Route               string     `gorm:"column:route;size:32" json:"route"`
	RouteReason         string     `gorm:"column:route_reason;size:64" json:"route_reason"`
	Steps               int        `gorm:"column:steps" json:"steps"`
	ToolAttempts        int        `gorm:"column:tool_attempts" json:"tool_attempts"`
	ToolExecuted        int        `gorm:"column:tool_executed" json:"tool_executed"`
	DuplicateRejected   int        `gorm:"column:duplicate_rejected" json:"duplicate_rejected"`
	PromptTokens        int        `gorm:"column:prompt_tokens" json:"prompt_tokens"`
	CompletionTokens    int        `gorm:"column:completion_tokens" json:"completion_tokens"`
	LatencyMs           int64      `gorm:"column:latency_ms" json:"latency_ms"`
	GroundingStatus     string     `gorm:"column:grounding_status;size:32" json:"grounding_status"`
	StopReason          string     `gorm:"column:stop_reason;size:64" json:"stop_reason"`
	DegradedMode        bool       `gorm:"column:degraded_mode" json:"degraded_mode"`
	EvidenceSummaryJSON string     `gorm:"column:evidence_summary_json;type:jsonb" json:"evidence_summary_json"`
	CreatedAt           time.Time  `gorm:"column:created_at" json:"created_at"`
	CompletedAt         *time.Time `gorm:"column:completed_at" json:"completed_at"`
}

func (*AgentRun) TableName() string { return AGENT_RUN_TABLE }

// AgentRun 状态
const (
	AgentRunRunning   = "RUNNING"
	AgentRunCompleted = "COMPLETED"
	AgentRunFailed    = "FAILED"
	AgentRunCancelled = "CANCELLED"
)
