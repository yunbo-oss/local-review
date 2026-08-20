package agent

import (
	"context"
	"testing"

	"local-review-go/internal/llm"

	"github.com/sashabaranov/go-openai"
)

type understandingChatStub struct {
	response string
	usage    llm.TokenUsage
}

func (s *understandingChatStub) ChatStream(context.Context, []openai.ChatCompletionMessage, func(string)) error {
	return nil
}
func (s *understandingChatStub) ChatComplete(context.Context, []openai.ChatCompletionMessage) (string, error) {
	return s.response, nil
}
func (s *understandingChatStub) ChatCompleteWithUsage(context.Context, []openai.ChatCompletionMessage) (string, llm.TokenUsage, error) {
	return s.response, s.usage, nil
}

func TestParseIntentSpecAndRetainOriginalQuery(t *testing.T) {
	raw := `{"intent":"compare","route":"agent","hard_filters":{"area":"海淀区","type_name":"咖啡","max_price":100,"min_price":0,"min_score":0,"min_comments":0},"soft_preferences":["安静办公","有插座","安静办公"],"entities":["上一轮两家"],"evidence_requirements":["reviews","shop_detail","reviews","invalid"],"rewritten_queries":["海淀安静办公咖啡","海淀有插座咖啡"],"need_clarification":false,"clarification_question":"","confidence":0.93}`
	spec, err := ParseIntentSpec(raw, "比较上一轮两家，预算100以内")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Intent != "compare" || spec.Route != "agent" || spec.HardFilters.MaxPrice != 100 {
		t.Fatalf("spec=%+v", spec)
	}
	if len(spec.SoftPreferences) != 2 || len(spec.EvidenceRequirements) != 2 {
		t.Fatalf("dedupe/sanitize failed: %+v", spec)
	}
	queries := spec.RetrievalQueries()
	if len(queries) != 3 || queries[0] != "比较上一轮两家，预算100以内" {
		t.Fatalf("queries=%v", queries)
	}
}

func TestLLMQueryUnderstanderStructuredOutput(t *testing.T) {
	stub := &understandingChatStub{
		response: `{"intent":"search","route":"rag_oneshot","hard_filters":{"area":"丰台区","type_name":"日料","max_price":180,"min_price":0,"min_score":0,"min_comments":0},"soft_preferences":["适合约会"],"entities":[],"evidence_requirements":["reviews"],"rewritten_queries":["丰台区适合约会的日料"],"need_clarification":false,"clarification_question":"","confidence":0.91}`,
		usage:    llm.TokenUsage{PromptTokens: 100, CompletionTokens: 30, TotalTokens: 130},
	}
	u := NewLLMQueryUnderstander(stub)
	spec, usage, err := u.Understand(context.Background(), QueryUnderstandingInput{Question: "丰台约会日料，人均180以内"})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Source != "llm" || spec.HardFilters.Area != "丰台区" || usage.TotalTokens != 130 {
		t.Fatalf("spec=%+v usage=%+v", spec, usage)
	}
}

func TestIntentSpecContextRoundTrip(t *testing.T) {
	want := FallbackIntentSpec("咖啡", "rag_oneshot")
	got, ok := IntentSpecFromContext(WithIntentSpec(context.Background(), want))
	if !ok || got.OriginalQuestion != want.OriginalQuestion {
		t.Fatalf("got=%+v ok=%v", got, ok)
	}
}

func TestIntentContextCarriesUnderstandingUsage(t *testing.T) {
	spec := FallbackIntentSpec("找咖啡", "rag_oneshot")
	usage := llm.TokenUsage{PromptTokens: 12, CompletionTokens: 4, TotalTokens: 16}
	ctx := WithIntentResult(context.Background(), spec, usage)
	got, ok := IntentSpecFromContext(ctx)
	if !ok || got.OriginalQuestion != spec.OriginalQuestion {
		t.Fatalf("intent context lost spec: %+v ok=%v", got, ok)
	}
	if gotUsage := IntentUsageFromContext(ctx); gotUsage.TotalTokens != 16 {
		t.Fatalf("intent context lost usage: %+v", gotUsage)
	}
	if calls := IntentCallsFromContext(ctx); calls != 1 {
		t.Fatalf("intent context calls=%d, want 1", calls)
	}
}
