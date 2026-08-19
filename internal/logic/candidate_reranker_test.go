package logic

import (
	"context"
	"testing"

	"local-review-go/internal/llm"
	repoInterfaces "local-review-go/internal/repository/interface"

	"github.com/sashabaranov/go-openai"
)

type rerankChatStub struct {
	response string
	usage    llm.TokenUsage
}

func (s *rerankChatStub) ChatStream(context.Context, []openai.ChatCompletionMessage, func(string)) error {
	return nil
}
func (s *rerankChatStub) ChatComplete(context.Context, []openai.ChatCompletionMessage) (string, error) {
	return s.response, nil
}
func (s *rerankChatStub) ChatCompleteWithUsage(context.Context, []openai.ChatCompletionMessage) (string, llm.TokenUsage, error) {
	return s.response, s.usage, nil
}

func TestCandidateRerankerRejectsUnknownIDsAndUsesScores(t *testing.T) {
	r := NewLLMCandidateReranker(&rerankChatStub{
		response: `{"ranked":[{"shop_id":999,"score":1,"reason":"伪造"},{"shop_id":2,"score":0.95,"reason":"评价直接支持安静办公"},{"shop_id":1,"score":0.4,"reason":"证据较弱"}]}`,
		usage:    llm.TokenUsage{TotalTokens: 88},
	})
	got, err := r.Rerank(context.Background(), RerankInput{
		Question: "适合安静办公", SoftPreferences: []string{"安静办公"}, TopK: 2,
		Candidates: []repoInterfaces.ShopSearchResult{
			{ShopID: 1, Name: "A", TextContent: "环境正常"},
			{ShopID: 2, Name: "B", TextContent: "安静，有插座"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 2 || got.Results[0].ShopID != 2 || got.Usage.TotalTokens != 88 {
		t.Fatalf("result=%+v", got)
	}
	if _, exists := got.Scores[999]; exists {
		t.Fatalf("unknown candidate leaked into scores: %+v", got.Scores)
	}
}
