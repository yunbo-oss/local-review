package repository

import (
	"context"
	"fmt"
	"local-review-go/internal/llm"
	repoInterfaces "local-review-go/internal/repository/interface"
	"local-review-go/pkg/utils/redisx"
	"strconv"

	"github.com/redis/go-redis/v9"
)

type vectorRepo struct {
	client *redis.Client
}

// NewVectorRepo 创建向量 Repository
func NewVectorRepo(client *redis.Client) repoInterfaces.VectorRepo {
	return &vectorRepo{client: client}
}

// StoreShop 存储店铺向量到 Redis Hash
func (r *vectorRepo) StoreShop(ctx context.Context, doc *repoInterfaces.ShopVectorDoc) error {
	key := redisx.VEC_SHOP_KEY_PREFIX + strconv.FormatInt(doc.ShopID, 10)
	embedBytes := llm.Float32ToBytes(doc.Embedding)
	return r.client.HSet(ctx, key,
		"name", doc.Name,
		"type_name", doc.TypeName,
		"area", doc.Area,
		"text_content", doc.TextContent,
		"embedding", embedBytes,
	).Err()
}

// DeleteShop 删除店铺向量
func (r *vectorRepo) DeleteShop(ctx context.Context, shopID int64) error {
	key := redisx.VEC_SHOP_KEY_PREFIX + strconv.FormatInt(shopID, 10)
	return r.client.Del(ctx, key).Err()
}

// SearchShops KNN 检索。typeFilter 为空则不过滤类型
func (r *vectorRepo) SearchShops(ctx context.Context, queryEmbedding []float32, typeFilter string, k int) ([]repoInterfaces.ShopSearchResult, error) {
	if k <= 0 {
		k = 5
	}
	vecBytes := llm.Float32ToBytes(queryEmbedding)

	var query string
	if typeFilter != "" {
		// TAG 过滤：@type_name:{美食}，需转义特殊字符
		query = fmt.Sprintf("(@type_name:{%s})=>[KNN %d @embedding $vec AS score]", typeFilter, k)
	} else {
		query = fmt.Sprintf("(*)=>[KNN %d @embedding $vec AS score]", k)
	}

	// FT.SEARCH idx:shop:vector "query" PARAMS 2 vec <binary> DIALECT 2 SORTBY score ASC
	args := []interface{}{
		"FT.SEARCH", redisx.VEC_SHOP_INDEX,
		query,
		"PARAMS", "2", "vec", vecBytes,
		"DIALECT", "2",
		"SORTBY", "score", "ASC",
		"RETURN", "5", "name", "type_name", "area", "text_content", "score",
	}
	cmd := r.client.Do(ctx, args...)
	res, err := cmd.Slice()
	if err != nil {
		return nil, fmt.Errorf("FT.SEARCH: %w", err)
	}
	return parseSearchResult(res, k)
}

// parseSearchResult 解析 FT.SEARCH 返回的 slice
// 格式: [totalCount, docId1, [field1, val1, field2, val2, ...], docId2, ...]
func parseSearchResult(res []interface{}, k int) ([]repoInterfaces.ShopSearchResult, error) {
	if len(res) < 1 {
		return nil, nil
	}
	total, _ := res[0].(int64)
	if total == 0 {
		return nil, nil
	}

	var results []repoInterfaces.ShopSearchResult
	// res[1:] 为 docId, fields 交替
	i := 1
	for i < len(res) && len(results) < k {
		docID, ok := res[i].(string)
		if !ok {
			i++
			continue
		}
		i++
		if i >= len(res) {
			break
		}
		fields, ok := res[i].([]interface{})
		if !ok {
			i++
			continue
		}
		i++

		// 解析 docID: "vec:shop:123" -> 123
		shopID := parseShopIDFromKey(docID)
		score := 0.0
		name, typeName, area, textContent := "", "", "", ""
		for j := 0; j+1 < len(fields); j += 2 {
			f, _ := fields[j].(string)
			v, _ := fields[j+1].(string)
			switch f {
			case "name":
				name = v
			case "type_name":
				typeName = v
			case "area":
				area = v
			case "text_content":
				textContent = v
			case "score":
				if s, err := strconv.ParseFloat(v, 64); err == nil {
					score = s
				}
			}
		}
		results = append(results, repoInterfaces.ShopSearchResult{
			ShopID:      shopID,
			Name:        name,
			TypeName:    typeName,
			Area:        area,
			TextContent: textContent,
			Score:       score,
		})
	}
	return results, nil
}

func parseShopIDFromKey(key string) int64 {
	prefix := redisx.VEC_SHOP_KEY_PREFIX
	if len(key) > len(prefix) {
		if id, err := strconv.ParseInt(key[len(prefix):], 10, 64); err == nil {
			return id
		}
	}
	return 0
}
