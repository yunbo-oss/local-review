package redis

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

const (
	DBINDEX int = 0
)

var _defaultRDB *redis.Client

func Init() {
	addrHost := getEnv("REDIS_ADDR", "127.0.0.1")
	port := getEnv("REDIS_PORT", "6379")
	password := getEnv("REDIS_PASSWORD", "8888.216")

	addr := fmt.Sprintf("%s:%s", addrHost, port)
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           DBINDEX,
		PoolSize:     100,             // 连接池大小，建议设置为 CPU 核心数的 10 倍
		MinIdleConns: 10,              // 最小空闲连接数，保持一定数量的连接以快速响应
		MaxRetries:   3,               // 最大重试次数
		DialTimeout:  5 * time.Second, // 连接超时
		ReadTimeout:  3 * time.Second, // 读取超时
		WriteTimeout: 3 * time.Second, // 写入超时
		PoolTimeout:  4 * time.Second, // 获取连接池连接的超时时间
	})

	if rdb == nil {
		logrus.Error("get redis client failed!")
		panic("failed to create redis client")
	}

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		logrus.Errorf("Failed to connect to Redis: %v", err)
		panic(err)
	}

	logrus.Info("Redis connection configured successfully")
	_defaultRDB = rdb
}

func GetRedisClient() *redis.Client {
	return _defaultRDB
}

// getEnv 获取环境变量，如果不存在则返回默认值（避免循环导入）
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
