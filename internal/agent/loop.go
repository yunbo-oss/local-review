package agent

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"local-review-go/internal/llm"
)

var shopCiteRe = regexp.MustCompile(`\[shop:(\d+)\]`)

// LoopResult 有界循环结果
type LoopResult struct {
	Answer             string
	Steps              int
	ToolCalls          int // 实际执行（非 duplicate 跳过）的次数
	ToolAttempts       int // 含失败/重复/未知的尝试次数
	DuplicateRejected  int
	ObservedShopIDs    []int64
	Usage              llm.TokenUsage
	Messages           []llm.ChatMessage
	Err                error
	GroundingOK        bool
	GroundingCode      string
	AllowNoResult      bool
}

// StatusCallback SSE status
type StatusCallback func(ToolStatus)

// RunLoop 有界 tool-loop
func RunLoop(ctx context.Context, client llm.ToolChatClient, exec *ToolExecutor, cfg RunConfig, messages []llm.ChatMessage, onStatus StatusCallback) LoopResult {
	if cfg.MaxSteps <= 0 {
		cfg = DefaultRunConfig()
	}
	if cfg.MaxToolAttempts <= 0 {
		cfg.MaxToolAttempts = DefaultMaxToolAttempts
	}
	if cfg.MaxToolsPerTurn <= 0 {
		cfg.MaxToolsPerTurn = DefaultMaxToolsPerTurn
	}
	if exec != nil {
		exec.OnStatus = onStatus
		if exec.MaxChars <= 0 {
			exec.MaxChars = cfg.MaxToolResultChars
		}
		if exec.Ledger == nil {
			exec.Ledger = NewEvidenceLedger()
		}
		if exec.Observed == nil {
			exec.Observed = map[int64]struct{}{}
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, cfg.RunTimeout)
	defer cancel()
	runCtx, span := StartRunSpan(runCtx, cfg.MaxSteps, cfg.MaxToolCalls)
	defer span.End()

	res := LoopResult{Messages: append([]llm.ChatMessage{}, messages...)}
	seen := map[string]struct{}{}
	tools := ToolDefinitions()

	for step := 0; step < cfg.MaxSteps; step++ {
		if err := runCtx.Err(); err != nil {
			res.Err = err
			return res
		}
		res.Steps = step + 1

		turn, err := client.ChatWithTools(runCtx, res.Messages, tools)
		if err != nil {
			res.Err = err
			return res
		}
		res.Usage.PromptTokens += turn.Usage.PromptTokens
		res.Usage.CompletionTokens += turn.Usage.CompletionTokens
		res.Usage.TotalTokens += turn.Usage.TotalTokens

		res.Messages = append(res.Messages, turn.Message)

		if len(turn.ToolCalls) == 0 {
			res.Answer = turn.Message.Content
			finalizeGrounding(&res, exec)
			return res
		}

		// 单 turn 工具调用上限
		calls := turn.ToolCalls
		if len(calls) > cfg.MaxToolsPerTurn {
			for _, tc := range calls[cfg.MaxToolsPerTurn:] {
				res.ToolAttempts++
				res.Messages = append(res.Messages, llm.ChatMessage{
					Role: "tool", Name: tc.Name, ToolCallID: tc.ID,
					Content: `{"error":"max_tools_per_turn","message":"tool calls beyond per-turn cap skipped"}`,
				})
			}
			calls = calls[:cfg.MaxToolsPerTurn]
		}

		for _, tc := range calls {
			if res.ToolAttempts >= cfg.MaxToolAttempts {
				res.Err = NewPublicError(ErrMaxToolCalls, fmt.Sprintf("max tool attempts %d reached", cfg.MaxToolAttempts))
				res.ObservedShopIDs = observedList(exec)
				return res
			}
			if res.ToolCalls >= cfg.MaxToolCalls {
				res.Err = NewPublicError(ErrMaxToolCalls, fmt.Sprintf("max tool calls %d reached", cfg.MaxToolCalls))
				res.ObservedShopIDs = observedList(exec)
				return res
			}
			res.ToolAttempts++
			key := tc.Name + "|" + CanonicalArgs(tc.Args)
			if _, ok := seen[key]; ok {
				res.DuplicateRejected++
				res.Messages = append(res.Messages, llm.ChatMessage{
					Role: "tool", Name: tc.Name, ToolCallID: tc.ID,
					Content: `{"error":"duplicate_tool_call","message":"identical tool+args already executed"}`,
				})
				continue
			}
			seen[key] = struct{}{}
			res.ToolCalls++

			toolCtx, toolCancel := context.WithTimeout(runCtx, cfg.ToolTimeout)
			out, execErr := exec.Execute(toolCtx, tc.Name, tc.Args)
			toolCancel()
			if execErr != nil {
				out = fmt.Sprintf(`{"error":%q}`, execErr.Error())
			}
			res.Messages = append(res.Messages, llm.ChatMessage{
				Role: "tool", Name: tc.Name, ToolCallID: tc.ID, Content: out,
			})
		}
	}

	// 预算耗尽：尝试无工具收尾
	turn, err := client.ChatCompleteTurn(runCtx, res.Messages)
	if err != nil {
		res.Err = fmt.Errorf("max steps reached: %w", err)
		res.ObservedShopIDs = observedList(exec)
		return res
	}
	res.Answer = turn.Message.Content
	res.Messages = append(res.Messages, turn.Message)
	finalizeGrounding(&res, exec)
	return res
}

func finalizeGrounding(res *LoopResult, exec *ToolExecutor) {
	res.ObservedShopIDs = observedList(exec)
	var ledger *EvidenceLedger
	if exec != nil {
		ledger = exec.Ledger
		exec.syncObservedFromLedger()
		res.ObservedShopIDs = observedList(exec)
	}
	allowNo := InferAllowNoResult(res.Answer, ledger)
	res.AllowNoResult = allowNo
	err := VerifyAnswer(res.Answer, ledger, VerifyOptions{AllowNoResult: allowNo})
	if err != nil {
		res.GroundingOK = false
		res.Err = err
		if pe, ok := err.(*PublicError); ok {
			res.GroundingCode = pe.Code
		}
		return
	}
	// 兼容旧校验：引用 ⊆ observed
	if !ValidateGroundedness(res.Answer, res.ObservedShopIDs) {
		res.GroundingOK = false
		res.Err = NewPublicError(ErrGroundingUnknownShop, "answer cites shops outside observed set")
		res.GroundingCode = ErrGroundingUnknownShop
		return
	}
	res.GroundingOK = true
}

func observedList(exec *ToolExecutor) []int64 {
	if exec == nil {
		return nil
	}
	if exec.Ledger != nil {
		return exec.Ledger.CiteableIDs()
	}
	if exec.Observed == nil {
		return nil
	}
	out := make([]int64, 0, len(exec.Observed))
	for id := range exec.Observed {
		out = append(out, id)
	}
	return out
}

// ValidateGroundedness 所有 [shop:id] 必须在 observed 中；无引用则 true
func ValidateGroundedness(answer string, observed []int64) bool {
	set := map[int64]struct{}{}
	for _, id := range observed {
		set[id] = struct{}{}
	}
	matches := shopCiteRe.FindAllStringSubmatch(answer, -1)
	for _, m := range matches {
		id, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return false
		}
		if _, ok := set[id]; !ok {
			return false
		}
	}
	return true
}

// ParseCitedShopIDs 提取引用
func ParseCitedShopIDs(answer string) []int64 {
	matches := shopCiteRe.FindAllStringSubmatch(answer, -1)
	var out []int64
	seen := map[int64]struct{}{}
	for _, m := range matches {
		id, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// Sleep 便于测试覆盖（保留）
var _ = time.Second
