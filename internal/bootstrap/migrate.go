package bootstrap

import (
	"fmt"
	"os"
	"strconv"

	"local-review-go/internal/model"

	"gorm.io/gorm"
)

// Migrate creates every table required by the API, seed, Agent context and eval
// persistence. Keeping this list in one package prevents the server and Docker
// migrate job from drifting apart.
func Migrate(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("postgres db is nil")
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserInfo{},
		&model.Shop{},
		&model.ShopType{},
		&model.Blog{},
		&model.BlogComments{},
		&model.Voucher{},
		&model.SecKillVoucher{},
		&model.VoucherOrder{},
		&model.Follow{},
		&model.UserAgentProfile{},
		&model.UserAgentProfileEvent{},
		&model.AgentRun{},
		&model.AgentToolCall{},
	); err != nil {
		return err
	}
	return migrateSearchDocuments(db)
}

func migrateSearchDocuments(db *gorm.DB) error {
	// Unit tests use an in-memory SQLite database to validate the portable GORM
	// schema. pgvector and PostgreSQL full-text indexes are integration-tested
	// against the Compose PostgreSQL service.
	if db.Dialector.Name() != "postgres" {
		return nil
	}
	dimension := 384
	if raw := os.Getenv("LLM_EMBEDDING_DIM"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > 2000 {
			return fmt.Errorf("invalid LLM_EMBEDDING_DIM %q for pgvector", raw)
		}
		dimension = parsed
	}
	statements := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS shop_search_documents (
			shop_id BIGINT PRIMARY KEY REFERENCES tb_shop(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			name_normalized TEXT NOT NULL,
			type_name TEXT NOT NULL,
			area TEXT NOT NULL,
			text_content TEXT NOT NULL,
			avg_price BIGINT NOT NULL,
			score INTEGER NOT NULL,
			comments INTEGER NOT NULL,
			sold INTEGER NOT NULL,
			search_vector TSVECTOR NOT NULL,
			embedding VECTOR(%d) NOT NULL,
			embedding_model TEXT NOT NULL,
			content_hash CHAR(64) NOT NULL,
			source_version BIGINT NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`, dimension),
		`CREATE INDEX IF NOT EXISTS idx_shop_search_embedding ON shop_search_documents USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64)`,
		`CREATE INDEX IF NOT EXISTS idx_shop_search_text ON shop_search_documents USING gin (search_vector)`,
		`CREATE INDEX IF NOT EXISTS idx_shop_search_filters ON shop_search_documents (area, type_name, avg_price, score, comments)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("migrate postgres search documents: %w", err)
		}
	}
	return nil
}
