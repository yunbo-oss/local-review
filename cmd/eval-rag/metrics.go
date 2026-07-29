package main

import (
	"fmt"
	"math"
	"sort"
)

// HitRateAtK：TopK 是否至少命中 1 个 relevant（Success@K）。禁止命名为 Recall。
func HitRateAtK(retrieved, relevant []int64, k int) float64 {
	if k <= 0 {
		k = len(retrieved)
	}
	if len(retrieved) > k {
		retrieved = retrieved[:k]
	}
	rel := toSet(relevant)
	for _, id := range retrieved {
		if rel[id] {
			return 1
		}
	}
	return 0
}

// RecallAtK：|命中 relevant| / |relevant|
func RecallAtK(retrieved, relevant []int64, k int) float64 {
	if len(relevant) == 0 {
		return 0
	}
	if k <= 0 {
		k = len(retrieved)
	}
	if len(retrieved) > k {
		retrieved = retrieved[:k]
	}
	rel := toSet(relevant)
	hit := 0
	seen := map[int64]bool{}
	for _, id := range retrieved {
		if rel[id] && !seen[id] {
			hit++
			seen[id] = true
		}
	}
	return float64(hit) / float64(len(relevant))
}

// PrecisionAtK：|命中 relevant| / K（分母为 K，非 retrieved 长度）
func PrecisionAtK(retrieved, relevant []int64, k int) float64 {
	if k <= 0 {
		return 0
	}
	if len(retrieved) > k {
		retrieved = retrieved[:k]
	}
	rel := toSet(relevant)
	hit := 0
	seen := map[int64]bool{}
	for _, id := range retrieved {
		if rel[id] && !seen[id] {
			hit++
			seen[id] = true
		}
	}
	return float64(hit) / float64(k)
}

// MRR：首个 relevant 的 1/rank（1-based）；未命中为 0
func MRR(retrieved, relevant []int64) float64 {
	rel := toSet(relevant)
	for i, id := range retrieved {
		if rel[id] {
			return 1.0 / float64(i+1)
		}
	}
	return 0
}

// NDCGAtK：二元 relevant 的 nDCG
func NDCGAtK(retrieved, relevant []int64, k int) float64 {
	if k <= 0 {
		k = len(retrieved)
	}
	if len(retrieved) > k {
		retrieved = retrieved[:k]
	}
	rel := toSet(relevant)
	var dcg float64
	for i, id := range retrieved {
		if rel[id] {
			dcg += 1.0 / math.Log2(float64(i)+2)
		}
	}
	idealHits := len(relevant)
	if idealHits > k {
		idealHits = k
	}
	var idcg float64
	for i := 0; i < idealHits; i++ {
		idcg += 1.0 / math.Log2(float64(i)+2)
	}
	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

// FilterFieldAccuracy：逐字段比较；双方皆空算一致
func FilterFieldAccuracy(pred, oracle *FilterJSON) float64 {
	if pred == nil {
		pred = &FilterJSON{}
	}
	if oracle == nil {
		oracle = &FilterJSON{}
	}
	fields := []bool{
		pred.Area == oracle.Area,
		pred.TypeName == oracle.TypeName,
		pred.MaxPrice == oracle.MaxPrice,
		pred.MinPrice == oracle.MinPrice,
		pred.MinScore == oracle.MinScore,
		pred.MinComments == oracle.MinComments,
	}
	ok := 0
	for _, f := range fields {
		if f {
			ok++
		}
	}
	return float64(ok) / float64(len(fields))
}

// FilterComplianceAtK：TopK 满足硬约束占比；无硬约束记 1.0
func FilterComplianceAtK(retrieved []ShopHit, oracle *FilterJSON, k int) float64 {
	if oracle == nil || !oracle.HasHardConstraints() {
		return 1.0
	}
	if k <= 0 {
		k = len(retrieved)
	}
	if len(retrieved) > k {
		retrieved = retrieved[:k]
	}
	if len(retrieved) == 0 {
		return 0
	}
	ok := 0
	for _, s := range retrieved {
		if shopMatchesFilter(s, oracle) {
			ok++
		}
	}
	return float64(ok) / float64(len(retrieved))
}

func shopMatchesFilter(s ShopHit, f *FilterJSON) bool {
	if f.Area != "" && s.Area != f.Area {
		return false
	}
	if f.TypeName != "" && s.TypeName != f.TypeName {
		return false
	}
	if f.MaxPrice > 0 && s.AvgPrice > f.MaxPrice {
		return false
	}
	if f.MinPrice > 0 && s.AvgPrice < f.MinPrice {
		return false
	}
	if f.MinScore > 0 && s.ShopScore < f.MinScore {
		return false
	}
	if f.MinComments > 0 && s.Comments < f.MinComments {
		return false
	}
	return true
}

func toSet(ids []int64) map[int64]bool {
	m := make(map[int64]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// AggregateQuality 对已成功评测的 case 求均值；infra 不进分母
func AggregateQuality(cases []CaseResult, topK int) (hit, recall, prec, mrr, ndcg float64) {
	n := 0
	for _, c := range cases {
		if c.InfraError != "" || c.ExpectNoResults {
			continue
		}
		n++
		hit += c.HitRate
		recall += c.Recall
		prec += c.Precision
		mrr += c.MRR
		ndcg += c.NDCG
	}
	if n == 0 {
		return 0, 0, 0, 0, 0
	}
	fn := float64(n)
	return hit / fn, recall / fn, prec / fn, mrr / fn, ndcg / fn
}

func AggregateTaskSuccess(cases []CaseResult) (taskSuccess, noResultAccuracy float64) {
	var taskN, noResultN, taskOK, noResultOK int
	for _, c := range cases {
		if c.InfraError != "" {
			continue
		}
		taskN++
		if c.TaskSuccess {
			taskOK++
		}
		if c.ExpectNoResults {
			noResultN++
			if c.NoResultPass {
				noResultOK++
			}
		}
	}
	if taskN > 0 {
		taskSuccess = float64(taskOK) / float64(taskN)
	}
	if noResultN > 0 {
		noResultAccuracy = float64(noResultOK) / float64(noResultN)
	}
	return taskSuccess, noResultAccuracy
}

func AggregateLatency(cases []CaseResult) (p50, p95 int64) {
	var values []int64
	for _, c := range cases {
		if c.InfraError == "" && c.LatencyMs >= 0 {
			values = append(values, c.LatencyMs)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return percentileInt64(values, 0.50), percentileInt64(values, 0.95)
}

func percentileInt64(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func ValidateRetrievalCase(c RetrievalCase) error {
	if c.ExpectNoResults {
		if len(c.RelevantShopIDs) != 0 {
			return fmt.Errorf("case %q: expect_no_results requires empty relevant_shop_ids", c.ID)
		}
		return nil
	}
	return ValidateRelevantNonEmpty(c.ID, c.RelevantShopIDs)
}

// ValidateRelevantNonEmpty 空 relevant 拒绝加载，除非 case 明确标记 expect_no_results。
func ValidateRelevantNonEmpty(id string, relevant []int64) error {
	if len(relevant) == 0 {
		return fmt.Errorf("case %q: relevant_shop_ids must be non-empty", id)
	}
	return nil
}
