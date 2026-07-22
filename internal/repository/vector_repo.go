package repository

import (
	"context"
	"fmt"
	"local-review-go/internal/llm"
	repoInterfaces "local-review-go/internal/repository/interface"
	"local-review-go/pkg/utils/redisx"
	"strconv"
	"strings"

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
func (r *vectorRepo) SearchShops(ctx context.Context, queryEmbedding []float32, filter *repoInterfaces.VectorSearchFilter, k int) ([]repoInterfaces.ShopSearchResult, error) {
	if k <= 0 {
		k = 5
	}
	vecBytes := llm.Float32ToBytes(queryEmbedding)

	preFilter := buildPreFilter(filter)
	query := fmt.Sprintf("(%s)=>[KNN %d @embedding $vec AS vector_score]", preFilter, k)

	args := []interface{}{
		"FT.SEARCH", redisx.VEC_SHOP_INDEX,
		query,
		"PARAMS", "2", "vec", vecBytes,
		"DIALECT", "2",
		"SORTBY", "vector_score", "ASC",
		"RETURN", "9", "name", "type_name", "area", "text_content", "avg_price", "score", "comments", "sold", "vector_score",
	}
	cmd := r.client.Do(ctx, args...)
	res, err := cmd.Slice()
	if err != nil {
		return nil, fmt.Errorf("FT.SEARCH KNN: %w", err)
	}
	results, err := parseSearchResult(res, k)
	if err != nil {
		return nil, err
	}
	if filter != nil && filter.MaxDistance > 0 {
		filtered := results[:0]
		for _, item := range results {
			if item.Score <= filter.MaxDistance {
				filtered = append(filtered, item)
			}
		}
		results = filtered
	}
	return results, nil
}

// SearchText 带预过滤的全文检索（name WEIGHT 高 + text_content，SCORER BM25）
func (r *vectorRepo) SearchText(ctx context.Context, query string, filter *repoInterfaces.VectorSearchFilter, k int) ([]repoInterfaces.ShopSearchResult, error) {
	if k <= 0 {
		k = 5
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("text search query is empty")
	}
	preFilter := buildPreFilter(filter)
	// 文本查询：转义后在 name|text_content 上检索，并与预过滤 AND；显式 BM25（避免依赖模块默认 TFIDF）
	escaped := escapeTextQuery(q)
	textClause := fmt.Sprintf("(@name|@text_content:(%s))", escaped)
	var fullQuery string
	if preFilter == "*" {
		fullQuery = textClause
	} else {
		fullQuery = fmt.Sprintf("(%s %s)", preFilter, textClause)
	}

	args := []interface{}{
		"FT.SEARCH", redisx.VEC_SHOP_INDEX,
		fullQuery,
		"SCORER", "BM25",
		"LIMIT", "0", strconv.Itoa(k),
		"RETURN", "8", "name", "type_name", "area", "text_content", "avg_price", "score", "comments", "sold",
	}
	cmd := r.client.Do(ctx, args...)
	res, err := cmd.Slice()
	if err != nil {
		return nil, fmt.Errorf("FT.SEARCH TEXT: %w", err)
	}
	return parseSearchResult(res, k)
}

// escapeTextQuery 转义 RediSearch 文本查询特殊字符，并按空白拆成 OR 词
func escapeTextQuery(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return s
	}
	escaped := make([]string, 0, len(fields))
	for _, w := range fields {
		escaped = append(escaped, escapeRediSearchToken(w))
	}
	return strings.Join(escaped, " | ")
}

func escapeRediSearchToken(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ',', '.', '<', '>', '{', '}', '[', ']', '"', '\'', ':', ';', '!', '@', '#', '$', '%', '^', '&', '*', '(', ')', '-', '+', '=', '~', '|':
			b.WriteRune('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// buildPreFilter 构建 RediSearch 预过滤表达式
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
	result := ""
	for _, p := range parts {
		result += "(" + p + ")"
	}
	return result
}

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

func parseSearchResult(res []interface{}, k int) ([]repoInterfaces.ShopSearchResult, error) {
	if len(res) < 1 {
		return nil, nil
	}
	total, _ := res[0].(int64)
	if total == 0 {
		return nil, nil
	}

	var results []repoInterfaces.ShopSearchResult
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

		shopID := parseShopIDFromKey(docID)
		score := 0.0
		name, typeName, area, textContent := "", "", "", ""
		var avgPrice int64
		var shopScore, comments, sold int
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
			case "avg_price":
				if n, err := strconv.ParseInt(v, 10, 64); err == nil {
					avgPrice = n
				}
			case "score":
				if n, err := strconv.Atoi(v); err == nil {
					shopScore = n
				}
			case "comments":
				if n, err := strconv.Atoi(v); err == nil {
					comments = n
				}
			case "sold":
				if n, err := strconv.Atoi(v); err == nil {
					sold = n
				}
			case "vector_score":
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
			AvgPrice:    avgPrice,
			ShopScore:   shopScore,
			Comments:    comments,
			Sold:        sold,
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
