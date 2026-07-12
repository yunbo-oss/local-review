// RAG 检索评估：Recall@5、MRR
// 用法：LLM_API_KEY=xxx go run ./cmd/eval-rag [--test-set=script/rag-eval.json]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"

	"local-review-go/internal/config"
	"local-review-go/internal/config/redis"
	"local-review-go/internal/llm"
	"local-review-go/internal/repository"
)

// EvalCase 单条评估样本
type EvalCase struct {
	Question        string  `json:"question"`
	ExpectedShopIDs []int64 `json:"expected_shop_ids"`
}

func main() {
	testSetPath := flag.String("test-set", "script/rag-eval.json", "测试集 JSON 路径")
	flag.Parse()

	config.Init()
	ctx := context.Background()

	if os.Getenv("LLM_API_KEY") == "" {
		log.Fatal("请设置 LLM_API_KEY 环境变量")
	}

	// 加载测试集
	data, err := os.ReadFile(*testSetPath)
	if err != nil {
		log.Fatalf("读取测试集失败: %v", err)
	}
	var cases []EvalCase
	if err := json.Unmarshal(data, &cases); err != nil {
		log.Fatalf("解析测试集 JSON 失败: %v", err)
	}
	if len(cases) == 0 {
		log.Fatal("测试集为空")
	}

	// 初始化
	client := redis.GetRedisClient()
	cfg := llm.LoadConfig()
	embClient, _ := llm.NewOpenAIClient(cfg)
	if embClient == nil {
		log.Fatal("Embedding 客户端初始化失败")
	}
	vecRepo := repository.NewVectorRepo(client)

	// 评估
	recallHit := 0
	mrrSum := 0.0
	k := 5

	for i, c := range cases {
		if i > 0 {
			time.Sleep(500 * time.Millisecond) // 避免 Embedding API 限流
		}

		// 1. 问题转向量
		queryVec, err := embClient.Embed(ctx, c.Question)
		if err != nil {
			log.Printf("[%d] Embedding 失败: %v", i+1, err)
			continue
		}

		// 2. KNN 检索 TopK（无预过滤，纯语义检索）
		results, err := vecRepo.SearchShops(ctx, queryVec, nil, k)
		if err != nil {
			log.Printf("[%d] 检索失败: %v", i+1, err)
			continue
		}

		// 3. 提取 Top5 店铺 ID
		topIDs := make([]int64, len(results))
		for j, r := range results {
			topIDs[j] = r.ShopID
		}

		// 4. Recall@5：Top5 中是否包含任意期望店铺
		expectedSet := make(map[int64]bool)
		for _, id := range c.ExpectedShopIDs {
			expectedSet[id] = true
		}
		hit := false
		for _, id := range topIDs {
			if expectedSet[id] {
				hit = true
				break
			}
		}
		if hit {
			recallHit++
		}

		// 5. MRR：第一个期望店铺的排名（1-based）
		rank := 0
		for j, id := range topIDs {
			if expectedSet[id] {
				rank = j + 1
				break
			}
		}
		if rank > 0 {
			mrrSum += 1.0 / float64(rank)
		}

		// 日志
		if hit {
			log.Printf("[%d] ✓ %s -> Top5 含期望店铺", i+1, c.Question)
		} else {
			log.Printf("[%d] ✗ %s -> Top5 未命中期望 %v", i+1, c.Question, c.ExpectedShopIDs)
		}
	}

	// 输出指标
	n := float64(len(cases))
	recallPct := 0.0
	mrr := 0.0
	if n > 0 {
		recallPct = 100.0 * float64(recallHit) / n
		mrr = mrrSum / n
	}

	log.Println("----------")
	log.Printf("Recall@5: %d/%d = %.1f%%", recallHit, len(cases), recallPct)
	log.Printf("MRR:      %.4f", mrr)
}
