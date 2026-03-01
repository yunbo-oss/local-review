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
		"avg_price", doc.AvgPrice,
		"score", doc.Score,
		"comments", doc.Comments,
		"sold", doc.Sold,
		"embedding", embedBytes,
	).Err()
}

// DeleteShop 删除店铺向量
func (r *vectorRepo) DeleteShop(ctx context.Context, shopID int64) error {
	key := redisx.VEC_SHOP_KEY_PREFIX + strconv.FormatInt(shopID, 10)
	return r.client.Del(ctx, key).Err()
}

// SearchShops 带预过滤的 KNN 向量检索（Filtered Vector Search）
// 预过滤器：TAG（area, type_name）+ NUMERIC 范围（avg_price, score, comments）
// 语义阈值：MaxDistance 在结果解析后过滤（COSINE 距离越小越相似）
func (r *vectorRepo) SearchShops(ctx context.Context, queryEmbedding []float32, filter *repoInterfaces.VectorSearchFilter, k int) ([]repoInterfaces.ShopSearchResult, error) {
	if k <= 0 {
		k = 5
	}
	vecBytes := llm.Float32ToBytes(queryEmbedding)

	// 构建预过滤表达式 + KNN
	// 格式：(预过滤)=>[KNN k @embedding $vec AS score]
	preFilter := buildPreFilter(filter)
	query := fmt.Sprintf("(%s)=>[KNN %d @embedding $vec AS score]", preFilter, k)

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
	results, err := parseSearchResult(res, k)
	if err != nil {
		return nil, err
	}
	// 语义相似度阈值：过滤掉距离过大的结果
	if filter != nil && filter.MaxDistance > 0 {
		filtered := results[:0]
		for _, r := range results {
			if r.Score <= filter.MaxDistance {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}
	return results, nil
}

// buildPreFilter 构建 RediSearch 预过滤表达式
// 空 filter 或全空条件返回 "*"
func buildPreFilter(filter *repoInterfaces.VectorSearchFilter) string {
	if filter == nil {
		return "*"
	}
	var parts []string
	if filter.Area != "" {
		parts = append(parts, fmt.Sprintf("@area:{%s}", escapeTagValue(filter.Area)))
	}
	if filter.TypeName != "" {
		parts = append(parts, fmt.Sprintf("@type_name:{%s}", escapeTagValue(filter.TypeName)))
	}
	if filter.MaxPrice > 0 || filter.MinPrice > 0 {
		minVal, maxVal := "-inf", "+inf"
		if filter.MinPrice > 0 {
			minVal = strconv.FormatInt(filter.MinPrice, 10)
		}
		if filter.MaxPrice > 0 {
			maxVal = strconv.FormatInt(filter.MaxPrice, 10)
		}
		parts = append(parts, fmt.Sprintf("@avg_price:[%s %s]", minVal, maxVal))
	}
	if filter.MinScore > 0 {
		parts = append(parts, fmt.Sprintf("@score:[%d +inf]", filter.MinScore))
	}
	if filter.MinComments > 0 {
		parts = append(parts, fmt.Sprintf("@comments:[%d +inf]", filter.MinComments))
	}
	if len(parts) == 0 {
		return "*"
	}
	// 多个条件 AND 连接
	result := ""
	for _, p := range parts {
		result += "(" + p + ")"
	}
	return result
}

// escapeTagValue 转义 RediSearch TAG 特殊字符：, " ' { } ( ) \
func escapeTagValue(s string) string {
	if s == "" {
		return s
	}
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case ',', '"', '\'', '{', '}', '(', ')', '\\':
			b = append(b, '\\', c)
		default:
			b = append(b, c)
		}
	}
	return string(b)
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
