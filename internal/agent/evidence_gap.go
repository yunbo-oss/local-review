package agent

import (
	"sort"
	"strings"
)

type EvidenceGapEvaluator interface {
	Evaluate(state *AgentState) []EvidenceGap
}

// DeterministicEvidenceGapEvaluator handles evidence requirements that can be
// proven from typed tool state. Open-ended semantic entailment remains a later,
// separately calibrated judge instead of being guessed here.
type DeterministicEvidenceGapEvaluator struct{}

func (DeterministicEvidenceGapEvaluator) Evaluate(state *AgentState) []EvidenceGap {
	if state == nil {
		return nil
	}
	active := make([]CandidateState, 0, len(state.Candidates))
	for _, candidate := range state.Candidates {
		if !candidate.Rejected {
			active = append(active, candidate)
		}
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].Rank == active[j].Rank {
			return active[i].ShopID < active[j].ShopID
		}
		return active[i].Rank < active[j].Rank
	})
	if len(active) == 0 {
		return []EvidenceGap{{
			Requirement: "candidate", EvidenceType: ToolSearchShops,
			Status: EvidenceMissing, ReasonCode: "NO_CANDIDATES",
		}}
	}

	needDetails, needReviews := false, len(state.Intent.SoftPreferences) > 0
	explicitReviewsRequired := false
	for _, requirement := range state.Intent.EvidenceRequirements {
		needDetails = needDetails || requirement == "shop_detail"
		needReviews = needReviews || requirement == "reviews"
		explicitReviewsRequired = explicitReviewsRequired || requirement == "reviews"
	}
	var gaps []EvidenceGap
	for _, candidate := range active {
		evidence := state.Evidence.Shops[candidate.ShopID]
		if needDetails {
			status, confidence, reason := EvidenceMissing, 0.0, "MISSING_DETAILS"
			if evidence.Verified || candidate.DetailsLoaded {
				status, confidence, reason = EvidenceSupported, 1, "DETAILS_VERIFIED"
			}
			gaps = append(gaps, EvidenceGap{
				ShopID: candidate.ShopID, Requirement: "shop_detail", EvidenceType: ToolGetShop,
				Status: status, Confidence: confidence, ReasonCode: reason,
			})
		}
		if !needReviews {
			continue
		}
		// Retrieval summaries are useful for ranking but are not proof that the
		// raw-review tool ran. Security and review-inspection requests require
		// provenance-bearing blog IDs before any semantic gap can be supported.
		if explicitReviewsRequired && len(evidence.BlogIDs) == 0 {
			gaps = append(gaps, EvidenceGap{
				ShopID: candidate.ShopID, Requirement: "reviews", EvidenceType: ToolListShopBlogs,
				Status: EvidenceMissing, Confidence: 0, ReasonCode: "RAW_REVIEWS_NOT_FETCHED",
			})
			continue
		}
		texts := append([]string(nil), evidence.BlogTexts...)
		if field, ok := evidence.Fields["review_evidence"]; ok {
			if text, ok := field.Value.(string); ok && strings.TrimSpace(text) != "" {
				texts = append(texts, text)
			}
		}
		joined := strings.Join(texts, "\n")
		if len(state.Intent.SoftPreferences) == 0 {
			status, confidence, reason := EvidenceMissing, 0.0, "MISSING_REVIEWS"
			if len(evidence.BlogIDs) > 0 || strings.TrimSpace(joined) != "" {
				status, confidence, reason = EvidenceSupported, 1, "REVIEWS_FETCHED"
			}
			gaps = append(gaps, EvidenceGap{
				ShopID: candidate.ShopID, Requirement: "reviews", EvidenceType: ToolListShopBlogs,
				Status: status, Confidence: confidence, ReasonCode: reason,
			})
			continue
		}
		for _, preference := range state.Intent.SoftPreferences {
			concepts := RequiredSemanticConcepts(preference)
			gap := EvidenceGap{
				ShopID: candidate.ShopID, Requirement: preference, EvidenceType: ToolListShopBlogs,
				Status: EvidenceMissing, ReasonCode: "MISSING_REVIEWS",
			}
			switch {
			case strings.TrimSpace(joined) == "":
			case len(concepts) == 0:
				gap.Status = EvidenceUnknown
				gap.Confidence = 0.5
				gap.ReasonCode = "SEMANTIC_JUDGE_REQUIRED"
			case ReviewTextSupportsSemantics(joined, concepts):
				gap.Status = EvidenceSupported
				gap.Confidence = 1
				gap.ReasonCode = "REVIEW_SUPPORT_FOUND"
			default:
				gap.Status = EvidenceUnknown
				gap.Confidence = 0
				gap.ReasonCode = "REVIEW_SUPPORT_NOT_FOUND"
			}
			gaps = append(gaps, gap)
		}
	}
	return gaps
}

func updateCandidateEvidenceScores(state *AgentState) {
	if state == nil {
		return
	}
	total := map[int64]int{}
	supported := map[int64]int{}
	for _, gap := range state.Gaps {
		if gap.ShopID <= 0 {
			continue
		}
		total[gap.ShopID]++
		if gap.Status == EvidenceSupported {
			supported[gap.ShopID]++
		}
	}
	for id, candidate := range state.Candidates {
		if total[id] == 0 {
			candidate.EvidenceScore = 0
		} else {
			candidate.EvidenceScore = float64(supported[id]) / float64(total[id])
		}
		state.Candidates[id] = candidate
	}
	rerankCandidatesByEvidence(state)
}

// rerankCandidatesByEvidence is the second reranking stage. Retrieval/LLM
// reranking establishes RetrievalRank; verified requirement coverage then
// deterministically promotes candidates with stronger evidence.
func rerankCandidatesByEvidence(state *AgentState) {
	if state == nil || len(state.Candidates) == 0 {
		return
	}
	candidates := make([]CandidateState, 0, len(state.Candidates))
	for _, candidate := range state.Candidates {
		candidates = append(candidates, candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Rejected != candidates[j].Rejected {
			return !candidates[i].Rejected
		}
		if candidates[i].EvidenceScore != candidates[j].EvidenceScore {
			return candidates[i].EvidenceScore > candidates[j].EvidenceScore
		}
		leftRank, rightRank := candidates[i].RetrievalRank, candidates[j].RetrievalRank
		if leftRank == 0 {
			leftRank = candidates[i].Rank
		}
		if rightRank == 0 {
			rightRank = candidates[j].Rank
		}
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return candidates[i].ShopID < candidates[j].ShopID
	})
	for rank, candidate := range candidates {
		candidate.Rank = rank + 1
		state.Candidates[candidate.ShopID] = candidate
	}
}
