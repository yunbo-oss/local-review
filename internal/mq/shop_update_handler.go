package mq

import (
	"context"
	"fmt"
	"strconv"

	"local-review-go/internal/llm"
	"local-review-go/internal/model"
	"local-review-go/internal/rag"
	repoInterfaces "local-review-go/internal/repository/interface"
	"local-review-go/pkg/utils/redisx"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

// NewShopUpdateCacheHandler 创建缓存消费者：异步删除 Redis 店铺缓存
func NewShopUpdateCacheHandler(rdb *redis.Client) ShopUpdateCacheHandler {
	return func(ctx context.Context, msg *ShopUpdateMsg) error {
		key := redisx.CACHE_SHOP_KEY + strconv.FormatInt(msg.ShopID, 10)
		if err := rdb.Del(ctx, key).Err(); err != nil {
			return fmt.Errorf("del cache shop %d: %w", msg.ShopID, err)
		}
		logrus.Debugf("店铺缓存已失效 shopId=%d", msg.ShopID)
		return nil
	}
}

// NewShopUpdateRAGHandler 创建 RAG 向量消费者：异步更新 Redis 向量
func NewShopUpdateRAGHandler(
	embClient llm.EmbeddingClient,
	vecRepo repoInterfaces.VectorRepo,
	shopRepo repoInterfaces.ShopRepo,
	shopTypeRepo repoInterfaces.ShopTypeRepo,
	blogRepo repoInterfaces.BlogRepo,
) ShopUpdateRAGHandler {
	return func(ctx context.Context, msg *ShopUpdateMsg) error {
		if embClient == nil || vecRepo == nil {
			logrus.Debug("RAG 未配置，跳过向量更新")
			return nil
		}
		shop, err := shopRepo.GetByID(ctx, msg.ShopID)
		if err != nil {
			return fmt.Errorf("get shop %d: %w", msg.ShopID, err)
		}
		typeName := "其他"
		if shop.TypeId > 0 {
			types, _ := shopTypeRepo.ListAll(ctx)
			for _, t := range types {
				if t.Id == shop.TypeId {
					typeName = t.Name
					break
				}
			}
		}
		// 获取该店铺用户点评摘要，用于 embedding（承载 filter 无法表达的语义）
		blogs := []model.Blog{}
		if blogRepo != nil {
			blogs, _ = blogRepo.ListByShopID(ctx, msg.ShopID, rag.MaxBlogsForEmbedding)
		}
		textContent := rag.BuildShopTextForEmbedding(shop, blogs)
		vecs, err := embClient.EmbedBatch(ctx, []string{textContent})
		if err != nil {
			return fmt.Errorf("embed shop %d: %w", msg.ShopID, err)
		}
		if len(vecs) == 0 {
			return fmt.Errorf("embedding empty for shop %d", msg.ShopID)
		}
		doc := &repoInterfaces.ShopVectorDoc{
			ShopID:      shop.Id,
			Name:        shop.Name,
			TypeName:    typeName,
			Area:        shop.Area,
			TextContent: textContent,
			AvgPrice:    shop.AvgPrice,
			Score:       shop.Score,
			Comments:    shop.Comments,
			Sold:        shop.Sold,
			Embedding:   vecs[0],
		}
		if err := vecRepo.StoreShop(ctx, doc); err != nil {
			return fmt.Errorf("store vector shop %d: %w", msg.ShopID, err)
		}
		logrus.Infof("店铺 RAG 向量已更新 shopId=%d", msg.ShopID)
		return nil
	}
}
