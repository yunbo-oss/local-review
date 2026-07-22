package rag

import (
	"reflect"
	"testing"
)

func TestFuseRRF_OverlapRanksFirst(t *testing.T) {
	t.Parallel()
	dense := []int64{1, 2, 3}
	text := []int64{2, 4, 1}
	got := FuseRRF([][]int64{dense, text}, 60, 4)
	if len(got) == 0 || got[0] != 2 {
		t.Fatalf("shop 2 appears high in both lists; want first, got %v", got)
	}
}

func TestFuseRRF_UnilateralDocsKept(t *testing.T) {
	t.Parallel()
	dense := []int64{1, 2}
	text := []int64{3, 4}
	got := FuseRRF([][]int64{dense, text}, 60, 4)
	wantSet := map[int64]bool{1: true, 2: true, 3: true, 4: true}
	if len(got) != 4 {
		t.Fatalf("want 4 docs, got %v", got)
	}
	for _, id := range got {
		if !wantSet[id] {
			t.Fatalf("unexpected id %d in %v", id, got)
		}
	}
}

func TestFuseRRF_StableTieBreakByShopID(t *testing.T) {
	t.Parallel()
	// Same single-list ranks → equal RRF scores → ascending shop_id
	a := []int64{10}
	b := []int64{5}
	got1 := FuseRRF([][]int64{a, b}, 60, 2)
	got2 := FuseRRF([][]int64{a, b}, 60, 2)
	if !reflect.DeepEqual(got1, got2) {
		t.Fatalf("unstable: %v vs %v", got1, got2)
	}
	if got1[0] != 5 || got1[1] != 10 {
		t.Fatalf("tie-break by shop_id ascending: want [5,10], got %v", got1)
	}
}

func TestFuseRRF_TopKCap(t *testing.T) {
	t.Parallel()
	dense := []int64{1, 2, 3, 4, 5, 6}
	text := []int64{6, 5, 4, 3, 2, 1}
	got := FuseRRF([][]int64{dense, text}, 60, 3)
	if len(got) != 3 {
		t.Fatalf("want topK=3, got %v", got)
	}
}

func TestFuseRRF_IdenticalInputStable(t *testing.T) {
	t.Parallel()
	lists := [][]int64{{3, 1, 2}, {2, 3, 4}}
	a := FuseRRF(lists, 60, 5)
	b := FuseRRF(lists, 60, 5)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("unstable output: %v vs %v", a, b)
	}
}
