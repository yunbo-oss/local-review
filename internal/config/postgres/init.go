package postgres

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const DATABASE = "postgres"

var defaultDB *gorm.DB

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// Init initializes the single durable relational/vector store. Redis remains
// a cache and short-lived state store; business facts and embeddings live here.
func Init() {
	user := getEnv("POSTGRES_USER", "postgres")
	password := getEnv("POSTGRES_PASSWORD", "postgres")
	addr := getEnv("POSTGRES_ADDR", "127.0.0.1")
	port := getEnv("POSTGRES_PORT", "5432")
	database := getEnv("POSTGRES_DATABASE", "local_review_go")
	sslMode := getEnv("POSTGRES_SSLMODE", "disable")
	timeZone := getEnv("POSTGRES_TIMEZONE", "Asia/Shanghai")

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s",
		addr, user, password, database, port, sslMode, timeZone,
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		logrus.Errorf("failed to connect to PostgreSQL: %v", err)
		panic(err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		panic(err)
	}
	maxOpen := 100
	if value := getEnv("POSTGRES_MAX_OPEN_CONNS", ""); value != "" {
		if parsed, parseErr := strconv.Atoi(value); parseErr == nil && parsed > 0 {
			maxOpen = parsed
		}
	}
	maxIdle := maxOpen / 4
	if maxIdle < 5 {
		maxIdle = 5
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(15 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		panic(err)
	}
	defaultDB = db
	logrus.Infof("PostgreSQL connection pool configured: max_open=%d max_idle=%d", maxOpen, maxIdle)
}

func GetPostgresDB() *gorm.DB { return defaultDB }

func GetPostgresDBStats() any {
	if defaultDB == nil {
		return nil
	}
	sqlDB, err := defaultDB.DB()
	if err != nil {
		return nil
	}
	return sqlDB.Stats()
}
