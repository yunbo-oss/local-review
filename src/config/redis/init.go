package redis

import (
	"fmt"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

const (
	ADDR     string = "127.0.0.1"
	PORT     string = "6379"
	PASSWORD string = "8888.216"
	DBINDEX  int    = 0
)

var _defaultRDB *redis.Client

func Init() {
	addr := fmt.Sprintf("%s:%s", ADDR, PORT)
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: PASSWORD,
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
