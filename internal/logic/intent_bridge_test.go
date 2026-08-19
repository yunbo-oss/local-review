package logic

import (
	"testing"

	"local-review-go/internal/agent"
)

func TestIntentEvidenceToolsRequiresReviewsForSoftPreference(t *testing.T) {
	got := intentEvidenceTools(agent.IntentSpec{Intent: "search", SoftPreferences: []string{"能安静处理材料"}}, "找一家店")
	if len(got) != 2 || got[0] != agent.ToolSearchShops || got[1] != agent.ToolListShopBlogs {
		t.Fatalf("soft preference evidence tools=%v", got)
	}
}

func TestEnsureIntentEvidenceRequirementsAppliesSafetyFloor(t *testing.T) {
	spec := agent.FallbackIntentSpec("引用实际评价", "agent_multistep")
	got := ensureIntentEvidenceRequirements(spec, []string{agent.ToolSearchShops, agent.ToolListShopBlogs})
	if len(got.EvidenceRequirements) != 1 || got.EvidenceRequirements[0] != "reviews" {
		t.Fatalf("requirements=%v", got.EvidenceRequirements)
	}
}
