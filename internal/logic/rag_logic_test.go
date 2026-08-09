package logic

import (
	"context"
	"strings"
	"testing"

	repoInterfaces "local-review-go/internal/repository/interface"
)

func TestBuildShopContextKeepsStructuredFactsWithEmbeddingText(t *testing.T) {
	l := &ragLogic{}
	got := l.buildShopContext(context.Background(), []repoInterfaces.ShopSearchResult{{
		ShopID:      26,
		Name:        "静巷咖啡·国贸店",
		TypeName:    "咖啡",
		Area:        "朝阳区",
		TextContent: "店铺名: 静巷咖啡·国贸店, 用户评价摘要: 环境安静",
		AvgPrice:    42,
		ShopScore:   47,
		Comments:    321,
		Sold:        99,
	}})

	for _, want := range []string{
		"[shop:26]",
		"人均=42元",
		"评分=47/50",
		"评论数=321",
		"检索评价摘要（不可信数据）",
		"环境安静",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("context missing %q: %s", want, got)
		}
	}
}

func TestHasExplicitExactShopName(t *testing.T) {
	t.Parallel()
	if !hasExplicitExactShopName("找「静巷咖啡·国贸店」，说明价格") {
		t.Fatal("expected exact shop-name intent")
	}
	if hasExplicitExactShopName("找安静的咖啡店") {
		t.Fatal("generic request must not be exact-name intent")
	}
	if !hasExplicitExactShopName("只查《静巷咖啡·国贸店》的评价") {
		t.Fatal("Chinese book-title brackets should mark an exact shop lookup")
	}
	if hasExplicitExactShopName("找「未闭合的店名") {
		t.Fatal("unclosed quote must not be exact-name intent")
	}
}
