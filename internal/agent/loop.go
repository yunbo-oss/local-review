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
	ToolCalls          int
	DuplicateRejected  int
	ObservedShopIDs    []int64
	Usage              llm.TokenUsage
	Messages           []llm.ChatMessage
	Err                error
	GroundingOK        bool
}

// StatusCallback SSE status
type StatusCallback func(ToolStatus)

// RunLoop 有界 tool-loop
func RunLoop(ctx context.Context, client llm.ToolChatClient, exec *ToolExecutor, cfg RunConfig, messages []llm.ChatMessage, onStatus StatusCallback) LoopResult {
	if cfg.MaxSteps <= 0 {
		cfg = DefaultRunConfig()
	}
	if exec != nil {
		exec.OnStatus = onStatus
		if exec.MaxChars <= 0 {
			exec.MaxChars = cfg.MaxToolResultChars
		}
		if exec.Observed == nil {
			exec.Observed = map[int64]struct{}{}
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, cfg.RunTimeout)
	defer cancel()

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
			res.ObservedShopIDs = observedList(exec)
			res.GroundingOK = ValidateGroundedness(res.Answer, res.ObservedShopIDs)
			if !res.GroundingOK {
				res.Err = fmt.Errorf("groundedness: answer cites shops outside observed set")
			}
			return res
		}

		for _, tc := range turn.ToolCalls {
			if res.ToolCalls >= cfg.MaxToolCalls {
				res.Err = fmt.Errorf("max tool calls %d reached", cfg.MaxToolCalls)
				res.ObservedShopIDs = observedList(exec)
				return res
			}
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
	res.ObservedShopIDs = observedList(exec)
	res.GroundingOK = ValidateGroundedness(res.Answer, res.ObservedShopIDs)
	if !res.GroundingOK {
		res.Err = fmt.Errorf("groundedness: answer cites shops outside observed set")
	}
	return res
}

func observedList(exec *ToolExecutor) []int64 {
	if exec == nil || exec.Observed == nil {
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
