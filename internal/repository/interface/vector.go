package interfaces

import (
	"context"
)

// ShopVectorDoc 店铺向量文档（存入 Redis Hash）
type ShopVectorDoc struct {
	ShopID      int64
	Name        string
	TypeName    string
	Area        string
	TextContent string
	Embedding   []float32
}

// ShopSearchResult 向量检索结果
type ShopSearchResult struct {
	ShopID      int64
	Name        string
	TypeName    string
	Area        string
	TextContent string // 用于组装 Prompt
	Score       float64
}

// VectorRepo 向量存储与检索接口
type VectorRepo interface {
	// StoreShop 存储店铺向量
	StoreShop(ctx context.Context, doc *ShopVectorDoc) error
	// DeleteShop 删除店铺向量
	DeleteShop(ctx context.Context, shopID int64) error
	// SearchShops KNN 检索，typeFilter 为空则不过滤
	SearchShops(ctx context.Context, queryEmbedding []float32, typeFilter string, k int) ([]ShopSearchResult, error)
}
