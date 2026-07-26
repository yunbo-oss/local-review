package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"local-review-go/internal/model"
	repoInterfaces "local-review-go/internal/repository/interface"

	"gorm.io/gorm"
)

type agentRunRepo struct {
	db *gorm.DB
}

// NewAgentRunRepo 创建运行仓储
func NewAgentRunRepo(db *gorm.DB) repoInterfaces.AgentRunRepo {
	return &agentRunRepo{db: db}
}

func (r *agentRunRepo) Begin(ctx context.Context, in repoInterfaces.AgentRunBegin) (int64, error) {
	if in.TraceID == "" || in.UserID <= 0 || in.SessionID == "" {
		return 0, fmt.Errorf("trace_id/user_id/session_id required")
	}
	row := model.AgentRun{
		TraceID:       in.TraceID,
		UserID:        in.UserID,
		SessionID:     in.SessionID,
		Status:        model.AgentRunRunning,
		Model:         in.Model,
		PolicyVersion: in.PolicyVersion,
		Route:         in.Route,
		RouteReason:   in.RouteReason,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *agentRunRepo) Finalize(ctx context.Context, in repoInterfaces.AgentRunFinalize) error {
	switch in.Status {
	case model.AgentRunCompleted, model.AgentRunFailed, model.AgentRunCancelled:
	default:
		return fmt.Errorf("invalid status %q", in.Status)
	}
	now := time.Now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row model.AgentRun
		if err := tx.Where("trace_id = ?", in.TraceID).First(&row).Error; err != nil {
			return err
		}
		if row.Status != model.AgentRunRunning {
			return fmt.Errorf("run already finalized: %s", row.Status)
		}
		updates := map[string]any{
			"status":                in.Status,
			"steps":                 in.Steps,
			"tool_attempts":         in.ToolAttempts,
			"tool_executed":         in.ToolExecuted,
			"duplicate_rejected":    in.DuplicateRejected,
			"prompt_tokens":         in.PromptTokens,
			"completion_tokens":     in.CompletionTokens,
			"latency_ms":            in.LatencyMs,
			"grounding_status":      in.GroundingStatus,
			"stop_reason":           in.StopReason,
			"degraded_mode":         in.DegradedMode,
			"evidence_summary_json": in.EvidenceSummaryJSON,
			"completed_at":          &now,
		}
		res := tx.Model(&model.AgentRun{}).
			Where("id = ? AND status = ?", row.ID, model.AgentRunRunning).
			Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return fmt.Errorf("finalize race")
		}
		if len(in.Tools) == 0 {
			return nil
		}
		calls := make([]model.AgentToolCall, 0, len(in.Tools))
		for _, t := range in.Tools {
			calls = append(calls, model.AgentToolCall{
				RunID:           row.ID,
				StepNo:          t.StepNo,
				AttemptNo:       t.AttemptNo,
				ToolName:        t.ToolName,
				ArgsHash:        t.ArgsHash,
				ArgsSummaryJSON: t.ArgsSummaryJSON,
				Status:          t.Status,
				ErrorCode:       t.ErrorCode,
				LatencyMs:       t.LatencyMs,
				ResultCount:     t.ResultCount,
			})
		}
		return tx.Create(&calls).Error
	})
}

func (r *agentRunRepo) GetByTraceID(ctx context.Context, traceID string) (string, int64, error) {
	var row model.AgentRun
	err := r.db.WithContext(ctx).Where("trace_id = ?", traceID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", 0, fmt.Errorf("run not found")
	}
	if err != nil {
		return "", 0, err
	}
	return row.Status, row.ID, nil
}
