package interfaces

import (
	"context"
)

// ShopVectorDoc 店铺检索文档（持久化到 PostgreSQL + pgvector）
type ShopVectorDoc struct {
	ShopID         int64
	Name           string
	TypeName       string
	Area           string
	TextContent    string
	AvgPrice       int64
	Score          int
	Comments       int
	Sold           int
	Embedding      []float32
	EmbeddingModel string
	ContentHash    string
	SourceVersion  int64
}

// VectorSearchFilter 向量检索预过滤条件（硬性条件，必须满足）
// 示例：WHERE area="朝阳区" AND type_name="美食" AND avg_price<200 AND score>=45
type VectorSearchFilter struct {
	Area        string  // 区域，如 "朝阳区"
	TypeName    string  // 类型，如 "美食"
	MaxPrice    int64   // 人均上限，<= 此值，0 表示不限制
	MinPrice    int64   // 人均下限，>= 此值，0 表示不限制
	MinScore    int     // 评分下限，>= 此值，0 表示不限制
	MinComments int     // 评论数下限，>= 此值，0 表示不限制
	MaxDistance float64 // 语义相似度阈值：COSINE 距离上限，超过则过滤；0 表示不限制（距离越小越相似）
}

// ShopSearchResult 向量 / Hybrid 检索结果（有序列表下标 0 为最佳）
type ShopSearchResult struct {
	ShopID         int64
	Name           string
	TypeName       string
	Area           string
	TextContent    string  // 用于组装 Prompt
	AvgPrice       int64   // 人均（硬过滤合规）
	ShopScore      int     // 店铺评分
	Comments       int     // 评论数
	Sold           int     // 销量
	Score          float64 // KNN 距离（COSINE 越小越相似）；Hybrid 时可为 0，排序以列表顺序为准
	ExactNameMatch bool    // 文本检索确认与引号内规范化店名完全一致
}

// VectorRepo 向量存储与检索接口
type VectorRepo interface {
	// StoreShop 存储店铺向量
	StoreShop(ctx context.Context, doc *ShopVectorDoc) error
	// DeleteShop 删除店铺向量
	DeleteShop(ctx context.Context, shopID int64) error
	// SearchShops 带预过滤的 KNN 向量检索，filter 为 nil 则不过滤
	SearchShops(ctx context.Context, queryEmbedding []float32, filter *VectorSearchFilter, k int) ([]ShopSearchResult, error)
	// SearchText 使用 PostgreSQL 全文索引检索名称与文本内容，filter 为 nil 则不过滤。
	SearchText(ctx context.Context, query string, filter *VectorSearchFilter, k int) ([]ShopSearchResult, error)
}
