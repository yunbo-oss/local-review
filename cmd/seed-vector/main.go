// 店铺向量化导入：从 MySQL 读取店铺 → Embedding API → 写入 Redis Stack
// 用法：LLM_API_KEY=xxx go run ./cmd/seed-vector
package main

import (
	"context"
	"log"
	"os"
	"time"

	"local-review-go/internal/config"
	"local-review-go/internal/config/mysql"
	"local-review-go/internal/config/redis"
	"local-review-go/internal/llm"
	"local-review-go/internal/rag"
	"local-review-go/internal/repository"
	repoInterfaces "local-review-go/internal/repository/interface"
)

func main() {
	config.Init()
	ctx := context.Background()

	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		log.Fatal("请设置 LLM_API_KEY 环境变量")
	}

	// 初始化索引
	client := redis.GetRedisClient()
	cfg := llm.LoadConfig()
	if err := redis.InitShopVectorIndex(ctx, client, cfg.EmbeddingDim); err != nil {
		log.Fatalf("创建向量索引失败: %v", err)
	}

	// 创建依赖
	embClient, _ := llm.NewOpenAIClient(cfg)
	if embClient == nil {
		log.Fatal("Embedding 客户端初始化失败")
	}
	vecRepo := repository.NewVectorRepo(client)
	shopRepo := repository.NewShopRepo(mysql.GetMysqlDB())
	shopTypeRepo := repository.NewShopTypeRepo(mysql.GetMysqlDB())
	blogRepo := repository.NewBlogRepo(mysql.GetMysqlDB())

	// 构建 typeId -> typeName
	types, err := shopTypeRepo.ListAll(ctx)
	if err != nil {
		log.Fatalf("查询店铺类型失败: %v", err)
	}
	typeMap := make(map[int64]string)
	for _, t := range types {
		typeMap[t.Id] = t.Name
	}

	// 获取所有店铺 ID
	ids, err := shopRepo.ListAllIDs(ctx)
	if err != nil {
		log.Fatalf("查询店铺 ID 失败: %v", err)
	}
	if len(ids) == 0 {
		log.Println("无店铺数据，请先执行 make seed")
		return
	}

	// 分批获取店铺并向量化
	batchSize := 10
	success := 0
	for i := 0; i < len(ids); i += batchSize {
		end := i + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batchIDs := ids[i:end]
		shops, err := shopRepo.GetByIDs(ctx, batchIDs)
		if err != nil {
			log.Printf("获取店铺 %v 失败: %v", batchIDs, err)
			continue
		}

		for i, shop := range shops {
			// 间隔 1.5 秒避免 API 限流（RPM），首条不延迟
			if i > 0 {
				time.Sleep(1500 * time.Millisecond)
			}
			typeName := typeMap[shop.TypeId]
			if typeName == "" {
				typeName = "其他"
			}
			// 获取该店铺用户点评摘要，用于 embedding（承载 filter 无法表达的语义）
			blogs, _ := blogRepo.ListByShopID(ctx, shop.Id, rag.MaxBlogsForEmbedding)
			textContent := rag.BuildShopTextForEmbedding(&shop, blogs)
			vecs, err := embClient.EmbedBatch(ctx, []string{textContent})
			if err != nil {
				log.Printf("店铺 %d Embedding 失败: %v", shop.Id, err)
				continue
			}
			if len(vecs) == 0 {
				continue
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
				log.Printf("存储店铺 %d 向量失败: %v", shop.Id, err)
				continue
			}
			success++
			log.Printf("已导入店铺 %d: %s", shop.Id, shop.Name)
		}
	}

	log.Printf("向量导入完成: %d/%d", success, len(ids))
}
