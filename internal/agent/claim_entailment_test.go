package agent

import (
	"context"
	"testing"

	"local-review-go/internal/llm"
)

func TestLLMClaimEntailmentVerifierSupportsDirectEvidence(t *testing.T) {
	ledger := NewEvidenceLedger()
	ledger.DiscoverFromSearch(7, "七号咖啡", nil)
	if err := ledger.RecordBlogEvidence(7, []int64{701}, []string{"工作日下午很安静，桌边有插座，适合用电脑。"}); err != nil {
		t.Fatal(err)
	}
	answer := ClaimAnswer{Recommendations: []ClaimedRecommendation{{
		ShopID: 7, Claims: []EvidenceClaim{{
			Text: "适合安静办公", EvidenceRefs: []string{"blog:701"},
		}},
	}}}
	client := &scriptedClient{turns: []llm.AssistantTurn{{
		Message: llm.ChatMessage{Role: "assistant", Content: `{"verdicts":[{"shop_id":7,"claim_index":0,"verdict":"supported","confidence":0.93,"reason_code":"DIRECT_SUPPORT"}]}`},
		Usage:   llm.TokenUsage{TotalTokens: 19},
	}}}
	usage, calls, err := NewLLMClaimEntailmentVerifier(client).Verify(context.Background(), answer, ledger)
	if err != nil || calls != 1 || usage.TotalTokens != 19 {
		t.Fatalf("usage=%+v calls=%d err=%v", usage, calls, err)
	}
}

func TestLLMClaimEntailmentVerifierFailsClosedBelowThreshold(t *testing.T) {
	ledger := NewEvidenceLedger()
	ledger.DiscoverFromSearch(7, "七号咖啡", nil)
	_ = ledger.RecordBlogEvidence(7, []int64{701}, []string{"店里周末人很多。"})
	answer := ClaimAnswer{Recommendations: []ClaimedRecommendation{{
		ShopID: 7, Claims: []EvidenceClaim{{Text: "适合安静办公", EvidenceRefs: []string{"blog:701"}}},
	}}}
	client := &scriptedClient{turns: []llm.AssistantTurn{{
		Message: llm.ChatMessage{Role: "assistant", Content: `{"verdicts":[{"shop_id":7,"claim_index":0,"verdict":"unknown","confidence":0.92,"reason_code":"NO_SUPPORT"}]}`},
	}}}
	if _, _, err := NewLLMClaimEntailmentVerifier(client).Verify(context.Background(), answer, ledger); err == nil {
		t.Fatal("unsupported subjective claim must fail closed")
	}
}
