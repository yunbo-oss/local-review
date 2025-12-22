package utils

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"

	"github.com/redis/go-redis/v9"
)

// BloomFilter Redis分布式布隆过滤器
type BloomFilter struct {
	client *redis.Client // Redis 客户端
	key    string        // Redis Key
	m      uint64        // 位数组大小
	k      uint64        // 哈希函数数量
}

// NewBloomFilter 创建新的布隆过滤器
// client: Redis客户端
// key: Redis键名
// n: 预期元素数量
// p: 期望的误判率 (0 < p < 1)
func NewBloomFilter(client *redis.Client, key string, n uint64, p float64) *BloomFilter {
	if p <= 0 || p >= 1 {
		panic("false positive rate must be between 0 and 1")
	}
	if n == 0 {
		panic("number of elements must be positive")
	}

	// 计算最优参数
	m := optimalM(n, p)
	k := optimalK(n, m)

	return &BloomFilter{
		client: client,
		key:    key,
		m:      m,
		k:      k,
	}
}

// Add 添加元素到布隆过滤器
func (bf *BloomFilter) Add(id int64) error {
	ctx := context.Background()
	pipe := bf.client.Pipeline()

	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, uint64(id))

	for i := uint64(0); i < bf.k; i++ {
		// 计算哈希值
		hash := hashWithSeed(data, uint32(i))
		offset := hash % bf.m
		pipe.SetBit(ctx, bf.key, int64(offset), 1)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// AddBatch 批量添加元素到布隆过滤器（真正的批量操作）
// 将多个ID的操作合并到一个Pipeline中，减少Redis往返次数
func (bf *BloomFilter) AddBatch(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	ctx := context.Background()
	pipe := bf.client.Pipeline()

	// 为每个ID计算所有哈希位并添加到Pipeline
	for _, id := range ids {
		data := make([]byte, 8)
		binary.LittleEndian.PutUint64(data, uint64(id))

		for i := uint64(0); i < bf.k; i++ {
			hash := hashWithSeed(data, uint32(i))
			offset := hash % bf.m
			pipe.SetBit(ctx, bf.key, int64(offset), 1)
		}
	}

	// 一次性执行所有操作
	_, err := pipe.Exec(ctx)
	return err
}

// Contains 检查元素是否存在
func (bf *BloomFilter) Contains(id int64) (bool, error) {
	ctx := context.Background()
	pipe := bf.client.Pipeline()

	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, uint64(id))

	// 收集所有 GETBIT 命令的结果
	cmds := make([]*redis.IntCmd, bf.k)

	for i := uint64(0); i < bf.k; i++ {
		hash := hashWithSeed(data, uint32(i))
		offset := hash % bf.m
		cmds[i] = pipe.GetBit(ctx, bf.key, int64(offset))
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	// 检查所有位是否都为 1
	for _, cmd := range cmds {
		val, err := cmd.Result()
		if err != nil {
			return false, err
		}
		if val == 0 {
			return false, nil // 只要有一位是 0，则一定不存在
		}
	}

	return true, nil // 所有位都是 1，可能存在
}

// optimalM 计算最优的位数组大小
func optimalM(n uint64, p float64) uint64 {
	return uint64(math.Ceil(-float64(n) * math.Log(p) / (math.Ln2 * math.Ln2)))
}

// optimalK 计算最优的哈希函数数量
func optimalK(n, m uint64) uint64 {
	return uint64(math.Ceil(float64(m) / float64(n) * math.Ln2))
}

// hashWithSeed 使用种子创建哈希函数
func hashWithSeed(data []byte, seed uint32) uint64 {
	hasher := fnv.New64a()
	seedBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(seedBytes, seed)
	hasher.Write(seedBytes)
	hasher.Write(data)
	return hasher.Sum64()
}

// DebugInfo 获取调试信息
func (bf *BloomFilter) DebugInfo() string {
	return fmt.Sprintf("BloomFilter[Key=%s, m=%d, k=%d]", bf.key, bf.m, bf.k)
}
