package llm

import (
	"context"
	"errors"
	"testing"
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
