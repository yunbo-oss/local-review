package model

import "time"

const AGENT_TOOL_CALL_TABLE = "agent_tool_calls"

// AgentToolCall 工具尝试摘要（不存无界 raw）
type AgentToolCall struct {
	ID              int64     `gorm:"primaryKey;AUTO_INCREMENT;column:id" json:"id"`
	RunID           int64     `gorm:"column:run_id;index;not null" json:"run_id"`
	StepNo          int       `gorm:"column:step_no;not null" json:"step_no"`
	AttemptNo       int       `gorm:"column:attempt_no;not null" json:"attempt_no"`
	ToolName        string    `gorm:"column:tool_name;size:64;not null" json:"tool_name"`
	ArgsHash        string    `gorm:"column:args_hash;size:64" json:"args_hash"`
	ArgsSummaryJSON string    `gorm:"column:args_summary_json;type:json" json:"args_summary_json"`
	Status          string    `gorm:"column:status;size:32" json:"status"`
	ErrorCode       string    `gorm:"column:error_code;size:64" json:"error_code"`
	LatencyMs       int64     `gorm:"column:latency_ms" json:"latency_ms"`
	ResultCount     int       `gorm:"column:result_count" json:"result_count"`
	CreatedAt       time.Time `gorm:"column:created_at" json:"created_at"`
}

func (*AgentToolCall) TableName() string { return AGENT_TOOL_CALL_TABLE }
