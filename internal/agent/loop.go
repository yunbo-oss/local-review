package agent

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"local-review-go/internal/llm"
)

var shopCiteRe = regexp.MustCompile(`\[shop:(\d+)\]`)
var markdownShopLinkRe = regexp.MustCompile(`\[[^\]]+\]\(\s*shop:(\d+)\s*\)`)

// LoopResult 有界循环结果
type LoopResult struct {
	Answer            string
	Steps             int
	ModelCalls        int
	ToolCalls         int // 实际执行（非 duplicate 跳过）的次数
	ToolAttempts      int // 含失败/重复/未知的尝试次数
	DuplicateRejected int
	ObservedShopIDs   []int64
	Usage             llm.TokenUsage
	Messages          []llm.ChatMessage
	Err               error
	GroundingOK       bool
	GroundingCode     string
	AllowNoResult     bool
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

		res.ModelCalls++
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
			res.Answer = NormalizeCitationSyntax(turn.Message.Content)
			finalizeGrounding(&res, exec)
			tryRepairGrounding(runCtx, client, &res, exec)
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

		budgetExhausted := false
		for callIdx, tc := range calls {
			if res.ToolAttempts >= cfg.MaxToolAttempts {
				for _, skipped := range calls[callIdx:] {
					res.Messages = append(res.Messages, llm.ChatMessage{
						Role: "tool", Name: skipped.Name, ToolCallID: skipped.ID,
						Content: `{"error":"max_tool_attempts","message":"tool budget exhausted; answer from existing evidence"}`,
					})
				}
				budgetExhausted = true
				break
			}
			if res.ToolCalls >= cfg.MaxToolCalls {
				for _, skipped := range calls[callIdx:] {
					res.Messages = append(res.Messages, llm.ChatMessage{
						Role: "tool", Name: skipped.Name, ToolCallID: skipped.ID,
						Content: `{"error":"max_tool_calls","message":"tool budget exhausted; answer from existing evidence"}`,
					})
				}
				budgetExhausted = true
				break
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
		if budgetExhausted {
			break
		}
	}

	// 预算耗尽：尝试无工具收尾
	res.Messages = append(res.Messages, llm.ChatMessage{
		Role:    "user",
		Content: "工具调用阶段已经结束。不要再调用或输出工具协议；请只根据已有工具结果直接给出中文最终回答，推荐必须带 [shop:id] 引用；若没有候选则明确说没有结果。",
	})
	res.ModelCalls++
	turn, err := client.ChatCompleteTurn(runCtx, res.Messages)
	if err != nil {
		res.Err = fmt.Errorf("max steps reached: %w", err)
		res.ObservedShopIDs = observedList(exec)
		return res
	}
	res.Usage.PromptTokens += turn.Usage.PromptTokens
	res.Usage.CompletionTokens += turn.Usage.CompletionTokens
	res.Usage.TotalTokens += turn.Usage.TotalTokens
	res.Answer = NormalizeCitationSyntax(turn.Message.Content)
	res.Messages = append(res.Messages, turn.Message)
	finalizeGrounding(&res, exec)
	tryRepairGrounding(runCtx, client, &res, exec)
	return res
}

// NormalizeCitationSyntax accepts the common Markdown-link variant emitted by
// compatible models and canonicalizes it to the API's [shop:id] contract.
func NormalizeCitationSyntax(answer string) string {
	return markdownShopLinkRe.ReplaceAllString(answer, "[shop:$1]")
}

// tryRepairGrounding performs one bounded, no-tool revision for formatting
// omissions or an unknown shop introduced by the model. Fact conflicts and
// unsupported semantic claims are intentionally never rewritten here.
func tryRepairGrounding(ctx context.Context, client llm.ToolChatClient, res *LoopResult, exec *ToolExecutor) {
	if res == nil || strings.TrimSpace(res.Answer) == "" {
		return
	}
	if res.GroundingCode != ErrGroundingNoCitation && res.GroundingCode != ErrGroundingUnknownShop {
		return
	}
	ids := observedList(exec)
	if exec != nil && len(exec.RequiredSemantics) > 0 && exec.Ledger != nil {
		if semanticIDs := exec.Ledger.SemanticEvidenceIDs(exec.RequiredSemantics); len(semanticIDs) > 0 {
			ids = semanticIDs
		}
	}
	if len(ids) == 0 {
		return
	}
	instruction := fmt.Sprintf(
		"引用校验失败：请保留原回答的事实与结论，只把涉及店铺的陈述补成严格的 [shop:id] 引用格式。可引用 id 仅限 %v；不要调用工具，不要加入新事实。",
		ids,
	)
	if res.GroundingCode == ErrGroundingUnknownShop {
		instruction = fmt.Sprintf(
			"引用校验失败：原回答包含未在工具结果中出现的店铺。请删除所有未知店名、对应事实和未知引用，只根据可引用 id %v 重写简洁中文答案；每个推荐都使用严格 [shop:id]，不要调用工具，不要加入新事实。",
			ids,
		)
	}
	res.Messages = append(res.Messages, llm.ChatMessage{Role: "user", Content: instruction})
	res.ModelCalls++
	turn, err := client.ChatCompleteTurn(ctx, res.Messages)
	if err != nil {
		return
	}
	res.Usage.PromptTokens += turn.Usage.PromptTokens
	res.Usage.CompletionTokens += turn.Usage.CompletionTokens
	res.Usage.TotalTokens += turn.Usage.TotalTokens
	res.Answer = NormalizeCitationSyntax(turn.Message.Content)
	res.Messages = append(res.Messages, turn.Message)
	finalizeGrounding(res, exec)
}

func finalizeGrounding(res *LoopResult, exec *ToolExecutor) {
	res.ObservedShopIDs = observedList(exec)
	var ledger *EvidenceLedger
	if exec != nil {
		ledger = exec.Ledger
		exec.syncObservedFromLedger()
		res.ObservedShopIDs = observedList(exec)
	}
	var semanticIDs []int64
	if exec != nil && len(exec.RequiredSemantics) > 0 && ledger != nil {
		semanticIDs = ledger.SemanticEvidenceIDs(exec.RequiredSemantics)
		if len(semanticIDs) == 0 {
			res.Answer = fmt.Sprintf(
				"没有找到已读取评价能够支持这些语义要求（%s）的候选店铺；为避免无依据推荐，建议放宽条件后重试。",
				strings.Join(exec.RequiredSemantics, "、"),
			)
		}
	}
	allowNo := InferAllowNoResult(res.Answer, ledger)
	if exec != nil && len(exec.RequiredSemantics) > 0 && len(semanticIDs) == 0 {
		allowNo = true
	}
	res.AllowNoResult = allowNo
	err := VerifyAnswer(res.Answer, ledger, VerifyOptions{
		AllowNoResult: allowNo, SemanticEvidenceIDs: semanticIDs,
	})
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
	res.Err = nil
	res.GroundingCode = ""
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
