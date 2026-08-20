package logic

import (
	"strings"

	"local-review-go/internal/agent"
	repoInterfaces "local-review-go/internal/repository/interface"
)

// IntentSpecToVectorFilter converts the shared Query Understanding result to
// the storage filter used by both one-shot RAG and Agent tools.
func IntentSpecToVectorFilter(spec agent.IntentSpec) *repoInterfaces.VectorSearchFilter {
	h := spec.HardFilters
	area := normalizeArea(h.Area)
	typeName := normalizeShopType(h.TypeName)
	if typeName != "" && !isCanonicalShopType(typeName) {
		typeName = ""
	}
	f := &repoInterfaces.VectorSearchFilter{
		Area: area, TypeName: typeName,
		MaxPrice: h.MaxPrice, MinPrice: h.MinPrice,
		MinScore: h.MinScore, MinComments: h.MinComments,
	}
	if f.MaxPrice < 0 {
		f.MaxPrice = 0
	}
	if f.MinPrice < 0 {
		f.MinPrice = 0
	}
	if f.MinScore < 0 {
		f.MinScore = 0
	}
	if f.MinComments < 0 {
		f.MinComments = 0
	}
	if f.Area == "" && f.TypeName == "" && f.MaxPrice == 0 && f.MinPrice == 0 &&
		f.MinScore == 0 && f.MinComments == 0 {
		return nil
	}
	return f
}

func intentEvidenceTools(spec agent.IntentSpec, question string) []string {
	tools := []string{agent.ToolSearchShops}
	for _, requirement := range spec.EvidenceRequirements {
		switch strings.TrimSpace(requirement) {
		case "shop_detail":
			tools = appendIntentTool(tools, agent.ToolGetShop)
		case "reviews":
			tools = appendIntentTool(tools, agent.ToolListShopBlogs)
		}
	}
	if len(spec.SoftPreferences) > 0 || spec.Intent == "compare" {
		tools = appendIntentTool(tools, agent.ToolListShopBlogs)
	}
	if len(tools) == 1 && spec.Intent != "search" {
		return requiredEvidenceTools(question)
	}
	return tools
}

func appendIntentTool(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
