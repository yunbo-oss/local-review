package logic

import (
	"math"
	"strings"

	repoInterfaces "local-review-go/internal/repository/interface"
)

type RetrievalDecision string

const (
	RetrievalAccept  RetrievalDecision = "accept"
	RetrievalVerify  RetrievalDecision = "verify"
	RetrievalAbstain RetrievalDecision = "abstain"
)

// RetrievalAssessment makes weak retrieval an explicit, auditable state. The
// score is deliberately conservative: LLM rerank relevance and direct review
// evidence both contribute, and only uniformly near-zero scored candidates
// are rejected before answer generation.
type RetrievalAssessment struct {
	CandidateCount   int               `json:"candidate_count"`
	ScoredCount      int               `json:"scored_count"`
	TopRerankScore   float64           `json:"top_rerank_score"`
	EvidenceCoverage float64           `json:"evidence_coverage"`
	Confidence       float64           `json:"confidence"`
	Decision         RetrievalDecision `json:"decision"`
	Reason           string            `json:"reason"`
}

func AssessRetrieval(candidates []repoInterfaces.ShopSearchResult, scores map[int64]float64, hasSoftPreferences bool) RetrievalAssessment {
	assessment := RetrievalAssessment{CandidateCount: len(candidates)}
	if len(candidates) == 0 {
		assessment.Decision = RetrievalAbstain
		assessment.Reason = "no_candidates"
		return assessment
	}
	withEvidence := 0
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.TextContent) != "" {
			withEvidence++
		}
		if score, ok := scores[candidate.ShopID]; ok {
			assessment.ScoredCount++
			assessment.TopRerankScore = math.Max(assessment.TopRerankScore, clamp01(score))
		}
	}
	assessment.EvidenceCoverage = float64(withEvidence) / float64(len(candidates))
	if assessment.ScoredCount == 0 {
		assessment.Confidence = 0.45 + 0.25*assessment.EvidenceCoverage
		assessment.Decision = RetrievalAccept
		assessment.Reason = "unscored_fallback"
		if hasSoftPreferences && assessment.EvidenceCoverage < 0.5 {
			assessment.Decision = RetrievalVerify
			assessment.Reason = "soft_preference_needs_evidence"
		}
		return assessment
	}
	assessment.Confidence = clamp01(0.7*assessment.TopRerankScore + 0.3*assessment.EvidenceCoverage)
	if assessment.TopRerankScore < 0.18 {
		assessment.Decision = RetrievalAbstain
		assessment.Reason = "reranker_uniformly_irrelevant"
		return assessment
	}
	if assessment.Confidence < 0.55 || (hasSoftPreferences && assessment.EvidenceCoverage < 0.5) {
		assessment.Decision = RetrievalVerify
		assessment.Reason = "low_confidence_requires_evidence"
		return assessment
	}
	assessment.Decision = RetrievalAccept
	assessment.Reason = "sufficient_relevance_and_evidence"
	return assessment
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
