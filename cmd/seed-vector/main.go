// 店铺检索文档导入：从 PostgreSQL 读取店铺和评价，生成向量后持久化回 PostgreSQL + pgvector。
package main

import (
	"context"
	"flag"
	"log"

	"local-review-go/internal/bootstrap"
	"local-review-go/internal/config"
	"local-review-go/internal/config/postgres"
	"local-review-go/internal/llm"
	"local-review-go/internal/rag"
	"local-review-go/internal/repository"
	repoInterfaces "local-review-go/internal/repository/interface"
)

func main() {
	reset := flag.Bool("reset", false, "clear and rebuild shop search documents")
	expectedCount := flag.Int("expected-count", 0, "fail unless exactly this many shop vectors exist")
	flag.Parse()

	config.Init()
	ctx := context.Background()
	db := postgres.GetPostgresDB()
	if err := bootstrap.Migrate(db); err != nil {
		log.Fatalf("初始化 PostgreSQL 检索结构失败: %v", err)
	}
	if *reset {
		if err := db.WithContext(ctx).Exec("DELETE FROM shop_search_documents").Error; err != nil {
			log.Fatalf("清空检索文档失败: %v", err)
		}
	}

	cfg := llm.LoadConfig()
	embClient, _, _ := llm.NewOpenAIClient(cfg)
	if embClient == nil {
		log.Fatal("Embedding 客户端初始化失败")
	}
	vecRepo := repository.NewVectorRepo(db)
	shopRepo := repository.NewShopRepo(db)
	shopTypeRepo := repository.NewShopTypeRepo(db)
	blogRepo := repository.NewBlogRepo(db)

	types, err := shopTypeRepo.ListAll(ctx)
	if err != nil {
		log.Fatalf("查询店铺类型失败: %v", err)
	}
	typeMap := make(map[int64]string, len(types))
	for _, item := range types {
		typeMap[item.Id] = item.Name
	}
	ids, err := shopRepo.ListAllIDs(ctx)
	if err != nil {
		log.Fatalf("查询店铺 ID 失败: %v", err)
	}
	if len(ids) == 0 {
		log.Fatal("无店铺数据，请先执行 seed")
	}

	const batchSize = 25
	success := 0
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		shops, getErr := shopRepo.GetByIDs(ctx, ids[start:end])
		if getErr != nil {
			log.Printf("读取店铺批次失败: %v", getErr)
			continue
		}
		texts := make([]string, len(shops))
		for index, shop := range shops {
			blogs, _ := blogRepo.ListByShopID(ctx, shop.Id, rag.MaxBlogsForEmbedding)
			texts[index] = rag.BuildShopTextForEmbedding(&shop, blogs)
		}
		vectors, embedErr := embClient.EmbedBatch(ctx, texts)
		if embedErr != nil || len(vectors) != len(shops) {
			log.Printf("生成批次向量失败: vectors=%d shops=%d err=%v", len(vectors), len(shops), embedErr)
			continue
		}
		for index, shop := range shops {
			typeName := typeMap[shop.TypeId]
			if typeName == "" {
				typeName = "其他"
			}
			sourceVersion := shop.UpdateTime.UnixMilli()
			if sourceVersion <= 0 {
				sourceVersion = 1
			}
			doc := &repoInterfaces.ShopVectorDoc{
				ShopID: shop.Id, Name: shop.Name, TypeName: typeName, Area: shop.Area,
				TextContent: texts[index], AvgPrice: shop.AvgPrice, Score: shop.Score,
				Comments: shop.Comments, Sold: shop.Sold, Embedding: vectors[index],
				EmbeddingModel: cfg.EmbeddingModel, SourceVersion: sourceVersion,
			}
			if err := vecRepo.StoreShop(ctx, doc); err != nil {
				log.Printf("持久化店铺 %d 检索文档失败: %v", shop.Id, err)
				continue
			}
			success++
		}
	}

	var count int64
	if err := db.WithContext(ctx).Table("shop_search_documents").Count(&count).Error; err != nil {
		log.Fatalf("统计检索文档失败: %v", err)
	}
	log.Printf("检索文档导入完成: success=%d stored=%d source=%d", success, count, len(ids))
	if *expectedCount > 0 && count != int64(*expectedCount) {
		log.Fatalf("检索文档数量不符合预期: got=%d want=%d", count, *expectedCount)
	}
}
