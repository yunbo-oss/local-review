package repository

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"unicode"

	pgvector "github.com/pgvector/pgvector-go"
	"gorm.io/gorm"

	repoInterfaces "local-review-go/internal/repository/interface"
)

const shopSearchDocumentTable = "shop_search_documents"

type vectorRepo struct {
	db *gorm.DB
}

// NewVectorRepo uses PostgreSQL as the durable source for both search metadata
// and embeddings. Redis is deliberately not part of this repository.
func NewVectorRepo(db *gorm.DB) repoInterfaces.VectorRepo {
	return &vectorRepo{db: db}
}

func (r *vectorRepo) StoreShop(ctx context.Context, doc *repoInterfaces.ShopVectorDoc) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("postgres vector repository is not configured")
	}
	if doc == nil || doc.ShopID <= 0 || len(doc.Embedding) == 0 {
		return fmt.Errorf("invalid shop vector document")
	}
	contentHash := strings.TrimSpace(doc.ContentHash)
	if contentHash == "" {
		sum := sha256.Sum256([]byte(doc.TextContent))
		contentHash = fmt.Sprintf("%x", sum[:])
	}
	model := strings.TrimSpace(doc.EmbeddingModel)
	if model == "" {
		model = "unknown"
	}
	version := doc.SourceVersion
	if version <= 0 {
		version = 1
	}

	// source_version prevents an older asynchronous embedding job from
	// overwriting a document generated from newer shop/review data.
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO shop_search_documents (
			shop_id, name, name_normalized, type_name, area, text_content,
			avg_price, score, comments, sold, search_vector, embedding,
			embedding_model, content_hash, source_version, updated_at
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			setweight(to_tsvector('simple', ?), 'A') || setweight(to_tsvector('simple', ?), 'B'),
			?::vector, ?, ?, ?, NOW()
		)
		ON CONFLICT (shop_id) DO UPDATE SET
			name = EXCLUDED.name,
			name_normalized = EXCLUDED.name_normalized,
			type_name = EXCLUDED.type_name,
			area = EXCLUDED.area,
			text_content = EXCLUDED.text_content,
			avg_price = EXCLUDED.avg_price,
			score = EXCLUDED.score,
			comments = EXCLUDED.comments,
			sold = EXCLUDED.sold,
			search_vector = EXCLUDED.search_vector,
			embedding = EXCLUDED.embedding,
			embedding_model = EXCLUDED.embedding_model,
			content_hash = EXCLUDED.content_hash,
			source_version = EXCLUDED.source_version,
			updated_at = NOW()
		WHERE shop_search_documents.source_version <= EXCLUDED.source_version`,
		doc.ShopID, doc.Name, normalizeNameForMatch(doc.Name), doc.TypeName, doc.Area, doc.TextContent,
		doc.AvgPrice, doc.Score, doc.Comments, doc.Sold,
		lexicalDocument(doc.Name), lexicalDocument(doc.TextContent),
		pgvector.NewVector(doc.Embedding), model, contentHash, version,
	).Error
}

func (r *vectorRepo) DeleteShop(ctx context.Context, shopID int64) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("postgres vector repository is not configured")
	}
	return r.db.WithContext(ctx).Exec(
		"DELETE FROM "+shopSearchDocumentTable+" WHERE shop_id = ?", shopID,
	).Error
}

type postgresSearchRow struct {
	ShopID         int64   `gorm:"column:shop_id"`
	Name           string  `gorm:"column:name"`
	TypeName       string  `gorm:"column:type_name"`
	Area           string  `gorm:"column:area"`
	TextContent    string  `gorm:"column:text_content"`
	AvgPrice       int64   `gorm:"column:avg_price"`
	ShopScore      int     `gorm:"column:shop_score"`
	Comments       int     `gorm:"column:comments"`
	Sold           int     `gorm:"column:sold"`
	RankScore      float64 `gorm:"column:rank_score"`
	ExactNameMatch bool    `gorm:"column:exact_name_match"`
}

func (r *vectorRepo) SearchShops(ctx context.Context, queryEmbedding []float32, filter *repoInterfaces.VectorSearchFilter, k int) ([]repoInterfaces.ShopSearchResult, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("postgres vector repository is not configured")
	}
	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("query embedding is empty")
	}
	if k <= 0 {
		k = 5
	}
	where, filterArgs := postgresFilter(filter)
	queryVector := pgvector.NewVector(queryEmbedding)
	args := []any{queryVector}
	args = append(args, filterArgs...)
	args = append(args, queryVector, k)
	query := `SELECT shop_id, name, type_name, area, text_content, avg_price,
		score AS shop_score, comments, sold, (embedding <=> ?::vector) AS rank_score
		FROM shop_search_documents` + where + `
		ORDER BY embedding <=> ?::vector
		LIMIT ?`
	var rows []postgresSearchRow
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("pgvector cosine search: %w", err)
	}
	results := mapPostgresRows(rows)
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

func (r *vectorRepo) SearchText(ctx context.Context, queryText string, filter *repoInterfaces.VectorSearchFilter, k int) ([]repoInterfaces.ShopSearchResult, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("postgres vector repository is not configured")
	}
	queryText = strings.TrimSpace(queryText)
	if queryText == "" {
		return nil, fmt.Errorf("text search query is empty")
	}
	if k <= 0 {
		k = 5
	}
	queryExpression := lexicalQuery(queryText)
	if queryExpression == "" {
		return nil, nil
	}
	nameHint := extractCornerQuoted(queryText)
	if nameHint == "" {
		nameHint = extractNameHint(queryText)
	}
	hintNormalized := normalizeNameForMatch(nameHint)
	where, filterArgs := postgresFilter(filter)
	if where == "" {
		where = " WHERE "
	} else {
		where += " AND "
	}
	where += "search_vector @@ to_tsquery('simple', ?)"
	query := `SELECT shop_id, name, type_name, area, text_content, avg_price,
		score AS shop_score, comments, sold,
		(ts_rank_cd(search_vector, to_tsquery('simple', ?)) +
		 CASE WHEN ? <> '' AND name_normalized = ? THEN 10 ELSE 0 END) AS rank_score,
		(? <> '' AND name_normalized = ?) AS exact_name_match
		FROM shop_search_documents` + where + `
		ORDER BY rank_score DESC, shop_id ASC
		LIMIT ?`
	// The rank expression appears before WHERE in SQL, so place its tsquery and
	// exact-name arguments before filter/WHERE arguments.
	orderedArgs := []any{queryExpression, hintNormalized, hintNormalized, hintNormalized, hintNormalized}
	orderedArgs = append(orderedArgs, filterArgs...)
	orderedArgs = append(orderedArgs, queryExpression, k)
	var rows []postgresSearchRow
	if err := r.db.WithContext(ctx).Raw(query, orderedArgs...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("postgres full-text search: %w", err)
	}
	return mapPostgresRows(rows), nil
}

func postgresFilter(filter *repoInterfaces.VectorSearchFilter) (string, []any) {
	if filter == nil {
		return "", nil
	}
	var clauses []string
	var args []any
	if value := strings.TrimSpace(filter.Area); value != "" {
		clauses = append(clauses, "area = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.TypeName); value != "" {
		clauses = append(clauses, "type_name = ?")
		args = append(args, value)
	}
	if filter.MaxPrice > 0 {
		clauses = append(clauses, "avg_price <= ?")
		args = append(args, filter.MaxPrice)
	}
	if filter.MinPrice > 0 {
		clauses = append(clauses, "avg_price >= ?")
		args = append(args, filter.MinPrice)
	}
	if filter.MinScore > 0 {
		clauses = append(clauses, "score >= ?")
		args = append(args, filter.MinScore)
	}
	if filter.MinComments > 0 {
		clauses = append(clauses, "comments >= ?")
		args = append(args, filter.MinComments)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func mapPostgresRows(rows []postgresSearchRow) []repoInterfaces.ShopSearchResult {
	results := make([]repoInterfaces.ShopSearchResult, 0, len(rows))
	for _, row := range rows {
		results = append(results, repoInterfaces.ShopSearchResult{
			ShopID: row.ShopID, Name: row.Name, TypeName: row.TypeName, Area: row.Area,
			TextContent: row.TextContent, AvgPrice: row.AvgPrice, ShopScore: row.ShopScore,
			Comments: row.Comments, Sold: row.Sold, Score: row.RankScore,
			ExactNameMatch: row.ExactNameMatch,
		})
	}
	return results
}

// lexicalDocument performs deterministic language-independent tokenization.
// Chinese sequences produce unigrams, bigrams and the complete term; Latin
// sequences produce lowercase words. Both indexing and querying use it.
func lexicalDocument(value string) string {
	return strings.Join(lexicalTokens(value), " ")
}

func lexicalQuery(value string) string {
	tokens := lexicalTokens(value)
	quoted := make([]string, 0, len(tokens))
	for _, token := range tokens {
		// Tokens contain letters/numbers only; quoting still makes the tsquery
		// construction explicit and independent from operator characters.
		quoted = append(quoted, "'"+strings.ReplaceAll(token, "'", "''")+"'")
	}
	return strings.Join(quoted, " | ")
}

func lexicalTokens(value string) []string {
	var groups []string
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		groups = append(groups, strings.ToLower(string(current)))
		current = nil
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			current = append(current, r)
		} else {
			flush()
		}
	}
	flush()
	seen := map[string]struct{}{}
	var tokens []string
	add := func(token string) {
		if token == "" {
			return
		}
		if _, exists := seen[token]; exists {
			return
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}
	for _, group := range groups {
		runes := []rune(group)
		add(group)
		if containsHan(runes) {
			for _, r := range runes {
				add(string(r))
			}
			for i := 0; i+1 < len(runes); i++ {
				add(string(runes[i : i+2]))
			}
		}
	}
	sort.Strings(tokens)
	return tokens
}

func containsHan(value []rune) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Han) {
			return true
		}
	}
	return false
}

func extractCornerQuoted(s string) string {
	start := strings.Index(s, "「")
	if start < 0 {
		return ""
	}
	rest := []rune(s[start+len("「"):])
	for index, r := range rest {
		if r == '」' {
			return strings.TrimSpace(string(rest[:index]))
		}
	}
	return ""
}

func extractNameHint(s string) string {
	s = strings.TrimSpace(s)
	for _, marker := range []string{"精确找", "看看", "查", "找"} {
		if index := strings.Index(s, marker); index >= 0 {
			s = s[index+len(marker):]
			break
		}
	}
	s = strings.TrimLeft(s, "「『\"' ")
	for _, separator := range []string{"的评价", "的无障碍", "，", ",", "。", "？", "?", "不要", "不能"} {
		if index := strings.Index(s, separator); index > 0 {
			s = s[:index]
		}
	}
	s = strings.Trim(s, "」』\"' ")
	if len([]rune(s)) < 3 {
		return ""
	}
	return s
}

func normalizeNameForMatch(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
