package agent

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"local-review-go/internal/llm"
)

// 字符预算（第一阶段用 rune 计数，便于单测；后续可接 tokenizer）
const (
	DefaultMaxSystemChars   = 2500
	DefaultMaxProfileChars  = 400
	DefaultMaxOldSummary    = 500
	DefaultMaxRecentChars   = 4000
	DefaultMaxQuestionChars = 800
	DefaultRecentMsgCount   = 8
)

// ContextBudgets 可注入覆盖
type ContextBudgets struct {
	MaxSystemChars   int
	MaxProfileChars  int
	MaxOldSummary    int
	MaxRecentChars   int
	MaxQuestionChars int
	RecentMsgCount   int
}

func (b ContextBudgets) withDefaults() ContextBudgets {
	if b.MaxSystemChars <= 0 {
		b.MaxSystemChars = DefaultMaxSystemChars
	}
	if b.MaxProfileChars <= 0 {
		b.MaxProfileChars = DefaultMaxProfileChars
	}
	if b.MaxOldSummary <= 0 {
		b.MaxOldSummary = DefaultMaxOldSummary
	}
	if b.MaxRecentChars <= 0 {
		b.MaxRecentChars = DefaultMaxRecentChars
	}
	if b.MaxQuestionChars <= 0 {
		b.MaxQuestionChars = DefaultMaxQuestionChars
	}
	if b.RecentMsgCount <= 0 {
		b.RecentMsgCount = DefaultRecentMsgCount
	}
	return b
}

// ContextBuilder 构建带预算的消息列表：policy/profile 与当前问题永不被挤掉
type ContextBuilder struct {
	Budgets ContextBudgets
}

// BuildInput 分层输入
type BuildInput struct {
	Policy          string // system policy（必留）
	ProfileSummary  string // 短偏好摘要
	EpisodicSummary string // 持久化会话事件摘要
	History         []llm.ChatMessage
	Question        string
}

// Build 裁剪历史：旧消息压成摘要，最近消息优先；超长单条截断但不挤掉 system/question
func (b *ContextBuilder) Build(system string, history []llm.ChatMessage, question string) []llm.ChatMessage {
	return b.BuildStructured(BuildInput{Policy: system, History: history, Question: question})
}

// BuildStructured 完整分层构建
func (b *ContextBuilder) BuildStructured(in BuildInput) []llm.ChatMessage {
	bud := b.Budgets.withDefaults()
	policy := truncateRunes(strings.TrimSpace(in.Policy), bud.MaxSystemChars)
	question := truncateRunes(strings.TrimSpace(in.Question), bud.MaxQuestionChars)
	if question == "" {
		question = in.Question
	}

	sysParts := []string{policy}
	if s := strings.TrimSpace(in.ProfileSummary); s != "" {
		sysParts = append(sysParts, "当前用户偏好："+truncateRunes(s, bud.MaxProfileChars))
	}
	if s := strings.TrimSpace(in.EpisodicSummary); s != "" {
		sysParts = append(sysParts, "会话事件摘要："+truncateRunes(s, bud.MaxOldSummary))
	}

	recent, older := splitRecent(in.History, bud.RecentMsgCount)
	if sum := summarizeOlder(older, bud.MaxOldSummary); sum != "" {
		sysParts = append(sysParts, "更早会话摘要："+sum)
	}
	recent = fitRecentBudget(recent, bud.MaxRecentChars)

	msgs := []llm.ChatMessage{{Role: "system", Content: strings.Join(sysParts, "\n")}}
	msgs = append(msgs, recent...)
	msgs = append(msgs, llm.ChatMessage{Role: "user", Content: question})
	return msgs
}

func splitRecent(history []llm.ChatMessage, n int) (recent, older []llm.ChatMessage) {
	if n <= 0 || len(history) <= n {
		return append([]llm.ChatMessage{}, history...), nil
	}
	cut := len(history) - n
	return append([]llm.ChatMessage{}, history[cut:]...), append([]llm.ChatMessage{}, history[:cut]...)
}

func summarizeOlder(older []llm.ChatMessage, maxRunes int) string {
	if len(older) == 0 || maxRunes <= 0 {
		return ""
	}
	var b strings.Builder
	for _, m := range older {
		role := m.Role
		if role == "" {
			role = "?"
		}
		line := fmt.Sprintf("[%s] %s", role, strings.TrimSpace(m.Content))
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(line)
	}
	return truncateRunes(b.String(), maxRunes)
}

func fitRecentBudget(recent []llm.ChatMessage, maxRunes int) []llm.ChatMessage {
	if maxRunes <= 0 || len(recent) == 0 {
		return recent
	}
	// 从最新往旧累加，超预算则丢掉更旧的
	total := 0
	keepFrom := len(recent)
	for i := len(recent) - 1; i >= 0; i-- {
		n := utf8.RuneCountInString(recent[i].Content)
		if total+n > maxRunes && keepFrom < len(recent) {
			break
		}
		if total+n > maxRunes {
			// 单条过大：截断该条仍保留
			recent[i].Content = truncateRunes(recent[i].Content, maxRunes)
			keepFrom = i
			break
		}
		total += n
		keepFrom = i
	}
	out := recent[keepFrom:]
	return out
}

func truncateRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max]) + "…"
}
