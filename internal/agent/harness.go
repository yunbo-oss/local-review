package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local-review-go/internal/llm"
)

// RecommendAgentHarness 编排：校验 → Context → Loop → Verify（persist 由 logic 层负责）
type RecommendAgentHarness struct {
	Tools        llm.ToolChatClient
	Exec         *ToolExecutor
	Config       RunConfig
	Builder      *ContextBuilder
	Planner      Planner
	Controller   DecisionController
	Checkpointer AgentCheckpointer
	ReactConfig  ReactRuntimeConfig
}

// HarnessInput 一次运行输入（不含完整 preference JSON 落库）
type HarnessInput struct {
	RunID           string
	TraceID         string
	Policy          string
	ProfileSummary  string
	EpisodicSummary string
	History         []llm.ChatMessage
	Question        string
	OnStatus        StatusCallback
	Intent          IntentSpec
	MemoryPolicy    MemoryPolicy
}

// RecommendRunOutcome Harness 返回
type RecommendRunOutcome struct {
	Answer            string
	Steps             int
	ToolCalls         int
	ToolAttempts      int
	DuplicateRejected int
	ObservedShopIDs   []int64
	Usage             llm.TokenUsage
	GroundingOK       bool
	GroundingCode     string
	TraceID           string
	Route             string
	StopReason        string
	LatencyMs         int64
	DegradedMode      bool
	Err               error
	Loop              LoopResult
}

// Run 执行有界推荐循环（不写 PostgreSQL/Redis；由 RecommendAgentLogic 负责持久化）
func (h *RecommendAgentHarness) Run(ctx context.Context, in HarnessInput) RecommendRunOutcome {
	start := time.Now()
	out := RecommendRunOutcome{TraceID: strings.TrimSpace(in.TraceID)}
	q := strings.TrimSpace(in.Question)
	if q == "" {
		out.Err = fmt.Errorf("question required")
		out.StopReason = "invalid"
		return out
	}
	if h.Tools == nil || h.Exec == nil {
		out.Err = fmt.Errorf("harness not configured")
		out.StopReason = "invalid"
		return out
	}
	cfg := h.Config
	if cfg.MaxSteps == 0 {
		cfg = DefaultRunConfig()
	}
	runCtx, runSpan, spanCreated := EnsureRunSpan(ctx, cfg.MaxSteps, cfg.MaxToolCalls)
	if spanCreated {
		defer runSpan.End()
	}
	if traceID := TraceIDFromContext(runCtx); traceID != "" {
		out.TraceID = traceID
	}
	builder := h.Builder
	if builder == nil {
		builder = &ContextBuilder{}
	}
	msgs := builder.BuildStructured(BuildInput{
		Policy: in.Policy, ProfileSummary: in.ProfileSummary,
		EpisodicSummary: in.EpisodicSummary, History: in.History, Question: q,
	})
	h.Exec.OnStatus = in.OnStatus
	var loopRes LoopResult
	if h.Controller != nil {
		SetRunSpanAttributes(runSpan, in.RunID, RuntimeVersionV2React)
		memoryPolicy := in.MemoryPolicy
		if memoryPolicy == "" {
			memoryPolicy = MemoryWriteAfterSuccess
		}
		runID := strings.TrimSpace(in.RunID)
		if runID == "" {
			runID = out.TraceID
		}
		loopRes = RunReact(runCtx, h.Tools, h.Controller, h.Exec, h.Checkpointer, cfg, h.ReactConfig, msgs, ReactHarnessInput{
			RunID: runID, TraceID: out.TraceID, Question: q, Intent: in.Intent,
			Memory: MemorySnapshot{
				Policy: memoryPolicy, ProfileSummary: in.ProfileSummary,
				SessionSummary: in.EpisodicSummary,
			},
		})
	} else if h.Planner != nil {
		SetRunSpanAttributes(runSpan, in.RunID, RuntimeVersionV1Plan)
		loopRes = RunPlanned(runCtx, h.Tools, h.Planner, h.Exec, cfg, msgs, PlanInput{
			Intent: in.Intent, ProfileSummary: in.ProfileSummary,
			HistorySummary: in.EpisodicSummary,
		})
	} else {
		SetRunSpanAttributes(runSpan, in.RunID, "v1_tool_loop")
		loopRes = RunLoop(runCtx, h.Tools, h.Exec, cfg, msgs, in.OnStatus)
	}
	out.Loop = loopRes
	out.Answer = loopRes.Answer
	out.Steps = loopRes.Steps
	out.ToolCalls = loopRes.ToolCalls
	out.ToolAttempts = loopRes.ToolAttempts
	out.DuplicateRejected = loopRes.DuplicateRejected
	out.ObservedShopIDs = loopRes.ObservedShopIDs
	out.Usage = loopRes.Usage
	out.GroundingOK = loopRes.GroundingOK
	out.GroundingCode = loopRes.GroundingCode
	out.LatencyMs = time.Since(start).Milliseconds()
	out.Err = loopRes.Err
	if runCtx.Err() != nil || ctx.Err() != nil {
		out.StopReason = "client_disconnect"
		out.Err = runCtx.Err()
		return out
	}
	if loopRes.Err != nil && (loopRes.Answer == "" || !loopRes.GroundingOK) {
		out.StopReason = "error"
		return out
	}
	if !loopRes.GroundingOK {
		out.StopReason = "grounding"
		if out.Err == nil {
			out.Err = NewPublicError(ErrGroundingUnknownShop, "回答未通过有据可查校验，请重试")
		}
		return out
	}
	out.StopReason = "final"
	if loopRes.RuntimeState != nil {
		switch loopRes.RuntimeState.Status {
		case RuntimeNeedsClarify:
			out.StopReason = "clarify"
		case RuntimeBudgetExhausted:
			out.StopReason = loopRes.RuntimeState.StopReason
		case RuntimeCompleted:
			if loopRes.RuntimeState.StopReason != "" {
				out.StopReason = loopRes.RuntimeState.StopReason
			}
		}
	}
	return out
}
