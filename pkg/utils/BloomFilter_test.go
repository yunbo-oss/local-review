package utils

import (
	"context"
	"fmt"
	"testing"

	"github.com/redis/go-redis/v9"
)

// 示例使用：防止缓存穿透
func TestBloomFilter(t *testing.T) {
	// 连接本地Redis (假设在 Docker 中运行)
	client := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "8888.216", // match docker-compose
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	// 初始化布隆过滤器 (预期10万元素，误判率0.01%)
	// Use a test key
	key := "test:bf:shop"
	client.Del(context.Background(), key)

	bf := NewBloomFilter(client, key, 100000, 0.01)

	// 模拟店铺ID
	validIds := []int64{
		1001,
		2002,
		3003,
	}

	// 将有效键添加到布隆过滤器
	for _, id := range validIds {
		err := bf.Add(id)
		if err != nil {
			t.Errorf("Failed to add %d: %v", id, err)
		}
	}

	// 测试键
	testIds := []int64{
		1001, // 存在的键
		9999, // 不存在的键
		2002, // 存在的键
		8888, // 不存在的键
	}

	for _, id := range testIds {
		exists, err := bf.Contains(id)
		if err != nil {
			t.Errorf("Check failed for %d: %v", id, err)
		}

		if exists {
			fmt.Printf("ID '%d' 可能存在\n", id)
		} else {
			fmt.Printf("ID '%d' 肯定不存在\n", id)
		}
	}
}
