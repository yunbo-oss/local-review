package redis

import (
	"context"
	"errors"
	"testing"
)

func TestInitShopVectorIndex_RejectsNonPositiveDim(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		dim  int
	}{
		{name: "zero", dim: 0},
		{name: "negative", dim: -1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := InitShopVectorIndex(context.Background(), nil, tc.dim)
			if err == nil {
				t.Fatalf("expected error for dim=%d, got nil", tc.dim)
			}
			if !errors.Is(err, ErrInvalidEmbeddingDim) && err.Error() == "" {
				t.Fatalf("expected ErrInvalidEmbeddingDim or descriptive error, got %v", err)
			}
		})
	}
}

func TestInitShopVectorIndex_NoSilent1536Fallback(t *testing.T) {
	t.Parallel()
	// dim<=0 must fail — never substitute defaultEmbeddingDim=1536
	err := InitShopVectorIndex(context.Background(), nil, 0)
	if err == nil {
		t.Fatal("dim=0 must fail; silent 1536 fallback is forbidden")
	}
}
