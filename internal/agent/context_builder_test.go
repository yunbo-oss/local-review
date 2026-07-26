package agent

import (
	"strings"
	"testing"
	"unicode/utf8"

	"local-review-go/internal/llm"
)

func TestContextBuilder_KeepsPolicyAndQuestion(t *testing.T) {
	b := &ContextBuilder{Budgets: ContextBudgets{
		MaxRecentChars: 50, RecentMsgCount: 2, MaxOldSummary: 40, MaxQuestionChars: 20,
	}}
	policy := "POLICY_MUST_KEEP"
	q := "当前问题很长很长很长很长很长很长"
	hist := make([]llm.ChatMessage, 0, 20)
	for i := 0; i < 20; i++ {
		hist = append(hist, llm.ChatMessage{Role: "user", Content: strings.Repeat("旧历史内容", 10)})
	}
	msgs := b.BuildStructured(BuildInput{
		Policy: policy, ProfileSummary: "海淀", History: hist, Question: q,
	})
	if len(msgs) < 2 {
		t.Fatalf("msgs=%d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, policy) {
		t.Fatalf("policy dropped: %s", msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, "更早会话摘要") {
		t.Fatalf("want old summary: %s", msgs[0].Content)
	}
	last := msgs[len(msgs)-1]
	if last.Role != "user" {
		t.Fatalf("last=%+v", last)
	}
	if utf8.RuneCountInString(last.Content) > 21 { // 20 + ellipsis rune
		t.Fatalf("question not truncated: %q", last.Content)
	}
	if !strings.HasPrefix(last.Content, "当前问题") {
		t.Fatalf("question corrupted: %q", last.Content)
	}
}

func TestContextBuilder_RecentPriority(t *testing.T) {
	b := &ContextBuilder{Budgets: ContextBudgets{MaxRecentChars: 30, RecentMsgCount: 4}}
	hist := []llm.ChatMessage{
		{Role: "user", Content: "AAAA"},
		{Role: "assistant", Content: "BBBB"},
		{Role: "user", Content: "CCCC"},
		{Role: "assistant", Content: "DDDD"},
	}
	msgs := b.Build("sys", hist, "Q")
	joined := ""
	for _, m := range msgs {
		joined += m.Content
	}
	if !strings.Contains(joined, "DDDD") || !strings.Contains(joined, "Q") {
		t.Fatalf("%s", joined)
	}
}
