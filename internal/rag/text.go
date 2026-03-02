package rag

import (
	"fmt"
	"strings"

	"local-review-go/internal/model"
)

const (
	// MaxBlogsForEmbedding 参与 embedding 的点评条数（按点赞排序取前 N 条），供调用方 ListByShopID 使用
	MaxBlogsForEmbedding = 5
	maxBlogExcerptLen    = 60
)

// BuildShopTextForEmbedding 构建用于 embedding 的店铺文本
//
// 设计：仅包含「店铺名 + 用户点评摘要」，与 filter 覆盖的字段（类型、区域、人均、评分、评论数）分离，
// 让 embedding 承载 filter 无法表达的语义（如「浪漫」「适合约会」「环境好」）。
func BuildShopTextForEmbedding(shop *model.Shop, blogs []model.Blog) string {
	summary := extractBlogSummary(blogs)
	return fmt.Sprintf("店铺名: %s, 用户评价摘要: %s", shop.Name, summary)
}

// extractBlogSummary 从点评列表中提取摘要，用于 embedding
// 按点赞数排序的 Top N 条，每条截断至 maxBlogExcerptLen 字符，用「、」连接
func extractBlogSummary(blogs []model.Blog) string {
	if len(blogs) == 0 {
		return "暂无用户点评"
	}
	var parts []string
	for i := 0; i < len(blogs) && i < MaxBlogsForEmbedding; i++ {
		b := &blogs[i]
		text := strings.TrimSpace(b.Title)
		if text != "" {
			text += " "
		}
		text += strings.TrimSpace(b.Content)
		if len(text) > maxBlogExcerptLen {
			text = text[:maxBlogExcerptLen] + "..."
		}
		if text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return "暂无用户点评"
	}
	return strings.Join(parts, "、")
}
