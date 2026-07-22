package redis

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

const (
	vecShopKeyPrefix = "vec:shop:"
	vecShopIndex     = "idx:shop:vector"
)

// ErrInvalidEmbeddingDim 表示 LLM_EMBEDDING_DIM / 传入 dim 非法（禁止静默 fallback）。
var ErrInvalidEmbeddingDim = errors.New("invalid embedding dimension: must be > 0")

// InitShopVectorIndex 创建店铺向量索引（RediSearch）。
// 若索引已存在则跳过。需在 Redis Stack 环境下运行。
// dim 必须 > 0（通常来自 LLM_EMBEDDING_DIM）；禁止静默回退到固定维度。
func InitShopVectorIndex(ctx context.Context, client *redis.Client, dim int) error {
	if dim <= 0 {
		return fmt.Errorf("%w (got %d; set LLM_EMBEDDING_DIM to match the embedding model)", ErrInvalidEmbeddingDim, dim)
	}
	if client == nil {
		return errors.New("redis client is nil")
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
