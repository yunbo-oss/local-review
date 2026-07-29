package llm

import (
	"context"
	"math"
	"testing"
)

func TestLocalEmbeddingDeterministicAndNormalized(t *testing.T) {
	c := NewLocalEmbeddingClient(384)
	a, err := c.Embed(context.Background(), "环境安静，适合办公学习")
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Embed(context.Background(), "环境安静，适合办公学习")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 384 || len(b) != 384 {
		t.Fatalf("dimensions %d/%d", len(a), len(b))
	}
	var norm float64
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("not deterministic at %d", i)
		}
		norm += float64(a[i] * a[i])
	}
	if math.Abs(norm-1) > 1e-5 {
		t.Fatalf("norm=%f want 1", norm)
	}
}

func TestLocalEmbeddingSemanticAliasImprovesSimilarity(t *testing.T) {
	c := NewLocalEmbeddingClient(384)
	query, _ := c.Embed(context.Background(), "想找适合写方案的地方")
	work, _ := c.Embed(context.Background(), "环境安静，有插座，适合办公")
	pet, _ := c.Embed(context.Background(), "露台可以带狗，宠物友好")
	if cosine(query, work) <= cosine(query, pet) {
		t.Fatalf("work similarity=%f pet=%f", cosine(query, work), cosine(query, pet))
	}
}

func cosine(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i] * b[i])
	}
	return sum
}
