package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/sashabaranov/go-openai"
)

type stubEmbedClient struct {
	dim  int
	vecs [][]float32
	err  error
}

func (s *stubEmbedClient) Embed(ctx context.Context, text string) ([]float32, error) {
	vecs, err := s.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, errors.New("empty")
	}
	return vecs[0], nil
}

func (s *stubEmbedClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		if i < len(s.vecs) {
			out[i] = s.vecs[i]
		} else if len(s.vecs) > 0 {
			out[i] = s.vecs[0]
		}
	}
	return out, nil
}

func (s *stubEmbedClient) Dimension() int { return s.dim }

func TestValidateEmbeddingDimension(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		dim     int
		vec     []float32
		wantErr bool
	}{
		{name: "match", dim: 4, vec: []float32{1, 2, 3, 4}, wantErr: false},
		{name: "mismatch_short", dim: 4, vec: []float32{1, 2}, wantErr: true},
		{name: "mismatch_long", dim: 2, vec: []float32{1, 2, 3}, wantErr: true},
		{name: "empty_vec", dim: 4, vec: nil, wantErr: true},
		{name: "illegal_dim", dim: 0, vec: []float32{1}, wantErr: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateEmbeddingDimension(tc.dim, tc.vec)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestOpenAIClient_EmbedBatch_RejectsDimMismatch(t *testing.T) {
	t.Parallel()
	// Unit-test the validation path without hitting the network:
	// simulate post-API check via validateEmbeddingDimension used by EmbedBatch.
	c := &openAIClient{config: Config{EmbeddingDim: 4}}
	err := c.checkEmbeddingBatch([][]float32{
		{1, 2, 3, 4},
		{1, 2}, // mismatch
	})
	if err == nil {
		t.Fatal("expected dimension mismatch error")
	}
}

func TestToOpenAIToolsAndMessages(t *testing.T) {
	t.Parallel()
	tools := []ToolDefinition{{
		Name:        "search_shops",
		Description: "search",
		Parameters:  []byte(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
	}}
	ot := toOpenAITools(tools)
	if len(ot) != 1 || ot[0].Function == nil || ot[0].Function.Name != "search_shops" {
		t.Fatalf("unexpected tools: %+v", ot)
	}

	msgs := []ChatMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "", ToolCalls: []ToolCall{{ID: "1", Name: "search_shops", Args: `{"query":"咖啡"}`}}},
		{Role: "tool", Content: "[]", ToolCallID: "1", Name: "search_shops"},
	}
	om := toOpenAIMessages(msgs)
	if len(om) != 3 {
		t.Fatalf("want 3 messages, got %d", len(om))
	}
	if len(om[1].ToolCalls) != 1 || om[1].ToolCalls[0].Function.Name != "search_shops" {
		t.Fatalf("tool calls not mapped: %+v", om[1])
	}
	if om[2].ToolCallID != "1" {
		t.Fatalf("tool call id: %q", om[2].ToolCallID)
	}
}

func TestAssistantTurnFromEmptyResponse(t *testing.T) {
	t.Parallel()
	_, err := assistantTurnFromResponse(openai.ChatCompletionResponse{})
	if err == nil {
		t.Fatal("expected empty response error")
	}
}
