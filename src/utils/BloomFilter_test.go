package utils

import (
	"fmt"
	"testing"
)

// 示例使用：防止缓存穿透
func TestBloomFilter(t *testing.T) {
	// 初始化布隆过滤器 (预期10万元素，误判率0.01%)
	bf := NewBloomFilter(100000, 0.0001)

	// 模拟缓存键
	validKeys := [][]byte{
		[]byte("user:1001"),
		[]byte("product:2002"),
		[]byte("order:3003"),
	}

	// 将有效键添加到布隆过滤器
	for _, key := range validKeys {
		bf.Add(key)
	}

	// 测试键
	testKeys := [][]byte{
		[]byte("user:1001"),    // 存在的键
		[]byte("user:9999"),    // 不存在的键
		[]byte("product:2002"), // 存在的键
		[]byte("invalid_key"),  // 不存在的键
	}

	for _, key := range testKeys {
		if bf.Contains(key) {
			fmt.Printf("键 '%s' 可能存在，需要查询缓存/数据库\n", key)
		} else {
			fmt.Printf("键 '%s' 肯定不存在，直接返回避免缓存穿透\n", key)
		}
	}
}
