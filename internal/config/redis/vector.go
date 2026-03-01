package redis

import (
	"context"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

const (
	vecShopKeyPrefix = "vec:shop:"
	vecShopIndex     = "idx:shop:vector"
)

const (
	// 默认 Embedding 维度（OpenAI text-embedding-3-small / DeepSeek 等）
	defaultEmbeddingDim = 1536
)

// InitShopVectorIndex 创建店铺向量索引（RediSearch）。
// 若索引已存在则跳过。需在 Redis Stack 环境下运行。
func InitShopVectorIndex(ctx context.Context, client *redis.Client, dim int) error {
	if dim <= 0 {
		dim = defaultEmbeddingDim
	}
	dimStr := strconv.Itoa(dim)

	// FT.CREATE idx:shop:vector ON HASH PREFIX 1 "vec:shop:" SCHEMA ...
	// 注意：embedding 需为 BLOB 类型，DIM 与 Embedding API 返回维度一致
	args := []interface{}{
		"FT.CREATE", vecShopIndex,
		"ON", "HASH",
		"PREFIX", "1", vecShopKeyPrefix,
		"SCHEMA",
		"name", "TEXT", "WEIGHT", "5.0",
		"type_name", "TAG",
		"area", "TAG",
		"text_content", "TEXT",
		"avg_price", "NUMERIC", "SORTABLE",
		"score", "NUMERIC", "SORTABLE",
		"comments", "NUMERIC", "SORTABLE",
		"sold", "NUMERIC", "SORTABLE",
		"embedding", "VECTOR", "HNSW", "6", "TYPE", "FLOAT32", "DIM", dimStr, "DISTANCE_METRIC", "COSINE",
	}
	err := client.Do(ctx, args...).Err()
	if err != nil {
		// 索引已存在时返回 "Index already exists"
		if strings.Contains(err.Error(), "Index already exists") {
			logrus.Infof("Shop vector index already exists, skip creation")
			return nil
		}
		return err
	}
	logrus.Infof("Shop vector index created successfully (dim=%d)", dim)
	return nil
}
