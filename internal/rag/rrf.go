package rag

import (
	"sort"
)

// RankedDoc is a document ID with its 0-based rank in one retrieval list.
type RankedDoc struct {
	ShopID int64
	Rank   int // 0-based
}

// FuseRRF merges multiple ranked lists with Reciprocal Rank Fusion.
// score(d) = Σ 1/(rrfK + rank_i + 1); higher is better.
// Ties broken by ascending shop_id for stability. Result capped at topK.
func FuseRRF(lists [][]int64, rrfK, topK int) []int64 {
	if rrfK <= 0 {
		rrfK = 60
	}
	if topK <= 0 {
		topK = 5
	}
	type agg struct {
		id    int64
		score float64
	}
	scores := make(map[int64]float64)
	for _, list := range lists {
		for rank, id := range list {
			scores[id] += 1.0 / float64(rrfK+rank+1)
		}
	}
	items := make([]agg, 0, len(scores))
	for id, sc := range scores {
		items = append(items, agg{id: id, score: sc})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		return items[i].id < items[j].id
	})
	if len(items) > topK {
		items = items[:topK]
	}
	out := make([]int64, len(items))
	for i, it := range items {
		out[i] = it.id
	}
	return out
}
