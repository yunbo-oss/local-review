package interfaces

import (
	"context"
)

// AgentRunBegin 开始一次运行
type AgentRunBegin struct {
	RunKey        string
	TraceID       string
	SpanID        string
	UserID        int64
	SessionID     string
	Model         string
	PolicyVersion string
	Route         string
	RouteReason   string
}

// AgentToolCallInput 工具尝试摘要（批量 Finalize）
type AgentToolCallInput struct {
	StepNo          int
	AttemptNo       int
	ToolName        string
	ArgsHash        string
	ArgsSummaryJSON string
	Status          string
	ErrorCode       string
	LatencyMs       int64
	ResultCount     int
}

// AgentRunFinalize 终态
type AgentRunFinalize struct {
	RunKey              string
	TraceID             string
	Status              string // COMPLETED / FAILED / CANCELLED
	Steps               int
	ToolAttempts        int
	ToolExecuted        int
	DuplicateRejected   int
	PromptTokens        int
	CompletionTokens    int
	LatencyMs           int64
	GroundingStatus     string
	StopReason          string
	DegradedMode        bool
	EvidenceSummaryJSON string
	Tools               []AgentToolCallInput
}

// AgentRunRepo 运行与工具审计
type AgentRunRepo interface {
	Begin(ctx context.Context, in AgentRunBegin) (runID int64, err error)
	Finalize(ctx context.Context, in AgentRunFinalize) error
	GetByTraceID(ctx context.Context, traceID string) (status string, runID int64, err error)
}
