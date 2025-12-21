package redis

import (
	"fmt"
	"os"

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
		Addr:     addr,
		Password: password,
		DB:       DBINDEX,
	})

	if rdb == nil {
		logrus.Error("get redis client failed!")
	}
	_defaultRDB = rdb

}

func GetRedisClient() *redis.Client {
	return _defaultRDB
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
