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
	Tools  llm.ToolChatClient
	Exec   *ToolExecutor
	Config RunConfig
	Builder *ContextBuilder
}

// HarnessInput 一次运行输入（不含完整 preference JSON 落库）
type HarnessInput struct {
	Policy         string
	ProfileSummary string
	History        []llm.ChatMessage
	Question       string
	OnStatus       StatusCallback
}

// RecommendRunOutcome Harness 返回
type RecommendRunOutcome struct {
	Answer             string
	Steps              int
	ToolCalls          int
	ToolAttempts       int
	DuplicateRejected  int
	ObservedShopIDs    []int64
	Usage              llm.TokenUsage
	GroundingOK        bool
	GroundingCode      string
	TraceID            string
	Route              string
	StopReason         string
	LatencyMs          int64
	DegradedMode       bool
	Err                error
	Loop               LoopResult
}

// Run 执行有界推荐循环（不写 MySQL/Redis；由 RecommendAgentLogic 负责持久化）
func (h *RecommendAgentHarness) Run(ctx context.Context, in HarnessInput) RecommendRunOutcome {
	start := time.Now()
	out := RecommendRunOutcome{}
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
	builder := h.Builder
	if builder == nil {
		builder = &ContextBuilder{}
	}
	msgs := builder.BuildStructured(BuildInput{
		Policy: in.Policy, ProfileSummary: in.ProfileSummary,
		History: in.History, Question: q,
	})
	loopRes := RunLoop(ctx, h.Tools, h.Exec, cfg, msgs, in.OnStatus)
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
	if ctx.Err() != nil {
		out.StopReason = "client_disconnect"
		out.Err = ctx.Err()
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
	return out
}
