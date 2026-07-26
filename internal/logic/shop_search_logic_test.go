package logic

import (
	"context"
	"errors"
	"testing"

	repoInterfaces "local-review-go/internal/repository/interface"
)

type stubEmbed struct {
	vec []float32
	err error
}

func (s *stubEmbed) Embed(ctx context.Context, text string) ([]float32, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.vec, nil
}
func (s *stubEmbed) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		v, err := s.Embed(ctx, texts[i])
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}
func (s *stubEmbed) Dimension() int { return len(s.vec) }

type stubVector struct {
	dense []repoInterfaces.ShopSearchResult
	text  []repoInterfaces.ShopSearchResult
	dErr  error
	tErr  error
}

func (s *stubVector) StoreShop(ctx context.Context, doc *repoInterfaces.ShopVectorDoc) error {
	return nil
}
func (s *stubVector) DeleteShop(ctx context.Context, shopID int64) error { return nil }
func (s *stubVector) SearchShops(ctx context.Context, queryEmbedding []float32, filter *repoInterfaces.VectorSearchFilter, k int) ([]repoInterfaces.ShopSearchResult, error) {
	if s.dErr != nil {
		return nil, s.dErr
	}
	return s.dense, nil
}
func (s *stubVector) SearchText(ctx context.Context, query string, filter *repoInterfaces.VectorSearchFilter, k int) ([]repoInterfaces.ShopSearchResult, error) {
	if s.tErr != nil {
		return nil, s.tErr
	}
	return s.text, nil
}

func TestResolveFilter_ExplicitOverridesExtracted(t *testing.T) {
	t.Parallel()
	extracted := &repoInterfaces.VectorSearchFilter{Area: "海淀区", TypeName: "咖啡", MaxPrice: 50}
	explicit := &repoInterfaces.VectorSearchFilter{Area: "朝阳区", MaxPrice: 100}
	got := ResolveFilter(explicit, extracted)
	if got == nil || got.Area != "朝阳区" || got.TypeName != "咖啡" || got.MaxPrice != 100 {
		t.Fatalf("unexpected resolve: %+v", got)
	}
}

func TestResolveFilter_NilBoth(t *testing.T) {
	t.Parallel()
	if ResolveFilter(nil, nil) != nil {
		t.Fatal("want nil")
	}
}

func TestShopSearchLogic_Dense(t *testing.T) {
	t.Parallel()
	l := NewShopSearchLogic(ShopSearchLogicDeps{
		EmbeddingClient: &stubEmbed{vec: []float32{0.1, 0.2}},
		VectorRepo: &stubVector{
			dense: []repoInterfaces.ShopSearchResult{{ShopID: 5, Name: "A"}, {ShopID: 7, Name: "B"}},
		},
	})
	got, err := l.Search(context.Background(), "火锅", nil, RetrieverDense, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ShopID != 5 {
		t.Fatalf("got %+v", got)
	}
}

func TestShopSearchLogic_HybridFuses(t *testing.T) {
	t.Parallel()
	l := NewShopSearchLogic(ShopSearchLogicDeps{
		EmbeddingClient: &stubEmbed{vec: []float32{0.1}},
		VectorRepo: &stubVector{
			dense: []repoInterfaces.ShopSearchResult{{ShopID: 1}, {ShopID: 2}, {ShopID: 3}},
			text:  []repoInterfaces.ShopSearchResult{{ShopID: 2}, {ShopID: 4}, {ShopID: 1}},
		},
	})
	got, err := l.Search(context.Background(), "火锅", nil, RetrieverHybrid, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].ShopID != 2 {
		t.Fatalf("want shop 2 first after RRF, got %+v", got)
	}
}

func TestShopSearchLogic_HybridTextFailureNoSilentDense(t *testing.T) {
	t.Parallel()
	l := NewShopSearchLogic(ShopSearchLogicDeps{
		EmbeddingClient: &stubEmbed{vec: []float32{0.1}},
		VectorRepo: &stubVector{
			dense: []repoInterfaces.ShopSearchResult{{ShopID: 1}},
			tErr:  errors.New("redis text down"),
		},
	})
	_, err := l.Search(context.Background(), "火锅", nil, RetrieverHybrid, 5)
	if err == nil {
		t.Fatal("hybrid must error when text search fails; silent dense success forbidden")
	}
}

func TestShopSearchLogic_HybridDegradedExplicit(t *testing.T) {
	t.Parallel()
	l := NewShopSearchLogic(ShopSearchLogicDeps{
		EmbeddingClient: &stubEmbed{vec: []float32{0.1}},
		VectorRepo: &stubVector{
			dense: []repoInterfaces.ShopSearchResult{{ShopID: 1, Name: "A"}},
			tErr:  errors.New("redis text down"),
		},
	})
	out, err := l.SearchWithMeta(context.Background(), "火锅", nil, RetrieverHybrid, 5, SearchModeDegraded)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Degraded || out.DegradedReason == "" || len(out.Results) != 1 {
		t.Fatalf("%+v", out)
	}
}

func TestShopSearchLogic_OrderedIDsConsistency(t *testing.T) {
	t.Parallel()
	// SC-003 prep: same inputs → identical ordered IDs
	deps := ShopSearchLogicDeps{
		EmbeddingClient: &stubEmbed{vec: []float32{0.1}},
		VectorRepo: &stubVector{
			dense: []repoInterfaces.ShopSearchResult{{ShopID: 9}, {ShopID: 5}, {ShopID: 1}},
			text:  []repoInterfaces.ShopSearchResult{{ShopID: 5}, {ShopID: 9}},
		},
	}
	l := NewShopSearchLogic(deps)
	a, err := l.Search(context.Background(), "烤鸭", &repoInterfaces.VectorSearchFilter{Area: "东城区"}, RetrieverHybrid, 5)
	if err != nil {
		t.Fatal(err)
	}
	b, err := l.Search(context.Background(), "烤鸭", &repoInterfaces.VectorSearchFilter{Area: "东城区"}, RetrieverHybrid, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) {
		t.Fatalf("length mismatch %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].ShopID != b[i].ShopID {
			t.Fatalf("order mismatch at %d: %d vs %d", i, a[i].ShopID, b[i].ShopID)
		}
	}
}
