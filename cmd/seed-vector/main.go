// 店铺向量化导入：从 MySQL 读取店铺 → Embedding API → 写入 Redis Stack
// 用法：LLM_API_KEY=xxx go run ./cmd/seed-vector
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	"local-review-go/internal/config"
	"local-review-go/internal/config/mysql"
	"local-review-go/internal/config/redis"
	"local-review-go/internal/llm"
	"local-review-go/internal/rag"
	"local-review-go/internal/repository"
	repoInterfaces "local-review-go/internal/repository/interface"

	goredis "github.com/redis/go-redis/v9"
)

func main() {
	reset := flag.Bool("reset", false, "drop and rebuild only the shop vector index")
	expectedCount := flag.Int("expected-count", 0, "fail unless exactly this many shop vectors exist")
	flag.Parse()

	config.Init()
	ctx := context.Background()

	// 初始化索引
	client := redis.GetRedisClient()
	cfg := llm.LoadConfig()
	if *reset {
		if err := resetVectorIndex(ctx, client); err != nil {
			log.Fatalf("重置向量索引失败: %v", err)
		}
	}
	if err := redis.InitShopVectorIndex(ctx, client, cfg.EmbeddingDim); err != nil {
		log.Fatalf("创建向量索引失败: %v", err)
	}

	// 创建依赖
	embClient, _, _ := llm.NewOpenAIClient(cfg)
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

	// 分批获取店铺并向量化。本地 embedding 与远程 embedding 都走批量
	// 接口；避免逐店固定 sleep，也减少可选远程 provider 的请求数。
	batchSize := 25
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

		texts := make([]string, len(shops))
		for j, shop := range shops {
			typeName := typeMap[shop.TypeId]
			if typeName == "" {
				typeName = "其他"
			}
			// 获取该店铺用户点评摘要，用于 embedding（承载 filter 无法表达的语义）
			blogs, _ := blogRepo.ListByShopID(ctx, shop.Id, rag.MaxBlogsForEmbedding)
			texts[j] = rag.BuildShopTextForEmbedding(&shop, blogs)
		}
		vecs, err := embClient.EmbedBatch(ctx, texts)
		if err != nil {
			log.Printf("店铺批次 %v Embedding 失败: %v", batchIDs, err)
			continue
		}
		if len(vecs) != len(shops) {
			log.Printf("店铺批次 %v Embedding 数量不匹配: got=%d want=%d", batchIDs, len(vecs), len(shops))
			continue
		}
		for j, shop := range shops {
			typeName := typeMap[shop.TypeId]
			if typeName == "" {
				typeName = "其他"
			}
			doc := &repoInterfaces.ShopVectorDoc{
				ShopID:      shop.Id,
				Name:        shop.Name,
				TypeName:    typeName,
				Area:        shop.Area,
				TextContent: texts[j],
				AvgPrice:    shop.AvgPrice,
				Score:       shop.Score,
				Comments:    shop.Comments,
				Sold:        shop.Sold,
				Embedding:   vecs[j],
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
	count, err := countVectorKeys(ctx, client)
	if err != nil {
		log.Fatalf("统计向量数量失败: %v", err)
	}
	log.Printf("向量索引文档数量: %d", count)
	if *expectedCount > 0 && count != *expectedCount {
		log.Fatalf("向量数量不符合预期: got=%d want=%d", count, *expectedCount)
	}
}

func resetVectorIndex(ctx context.Context, client *goredis.Client) error {
	err := client.Do(ctx, "FT.DROPINDEX", "idx:shop:vector", "DD").Err()
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "unknown index") {
		return err
	}
	return nil
}

func countVectorKeys(ctx context.Context, client *goredis.Client) (int, error) {
	var cursor uint64
	total := 0
	for {
		keys, next, err := client.Scan(ctx, cursor, "vec:shop:*", 500).Result()
		if err != nil {
			return 0, fmt.Errorf("scan vec keys: %w", err)
		}
		total += len(keys)
		cursor = next
		if cursor == 0 {
			return total, nil
		}
	}
}
