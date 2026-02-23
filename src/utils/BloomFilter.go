package utils

import (
	"encoding/binary"
	"hash/fnv"
	"math"
)

type BloomFilter struct {
	m        uint64                // 位数组大小
	k        uint64                // 哈希函数数量
	bits     []byte                // 位数组
	hashFunc []func([]byte) uint64 // 哈希函数集
}

// NewBloomFilter 创建新的布隆过滤器
// n: 预期元素数量
// p: 期望的误判率 (0 < p < 1)
func NewBloomFilter(n uint64, p float64) *BloomFilter {
	if p <= 0 || p >= 1 {
		panic("false positive rate must be between 0 and 1")
	}
	if n == 0 {
		panic("number of elements must be positive")
	}

	// 计算最优参数
	m := optimalM(n, p)
	k := optimalK(n, m)

	// 初始化位数组(按8位对齐)
	bits := make([]byte, (m+7)/8)

	// 初始化哈希函数
	hashFunc := make([]func([]byte) uint64, k)
	for i := range hashFunc {
		seed := uint32(i)
		hashFunc[i] = func(data []byte) uint64 {
			return hashWithSeed(data, seed)
		}
	}

	return &BloomFilter{
		m:        m,
		k:        k,
		bits:     bits,
		hashFunc: hashFunc,
	}

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
	hasher := fnv.New64a() // 创建FNV-1a算法的64位版本

	// 将种子写入哈希
	seedBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(seedBytes, seed)
	hasher.Write(seedBytes)

	// 写入实际数据
	hasher.Write(data)
	return hasher.Sum64() // 计算并返回64位无符号整数哈希值

}

// Add 添加元素到布隆过滤器
func (bf *BloomFilter) Add(data []byte) {
	for _, hashFn := range bf.hashFunc {
		// 计算哈希值并取模
		hash := hashFn(data) % bf.m

		// 计算位位置
		byteIndex := hash / 8
		bitIndex := hash % 8

		// 设置位
		bf.bits[byteIndex] |= 1 << bitIndex
	}
}

// Contains 检查元素是否可能在布隆过滤器中
func (bf *BloomFilter) Contains(data []byte) bool {
	for _, hashFn := range bf.hashFunc {
		hash := hashFn(data) % bf.m
		byteIndex := hash / 8
		bitIndex := hash % 8

		// 如果任何一位未设置，元素肯定不存在
		if bf.bits[byteIndex]&(1<<bitIndex) == 0 {
			return false
		}
	}
	return true
}
