package config

import (
	"local-review-go/internal/config/mysql"
	"local-review-go/internal/config/redis"
	"local-review-go/internal/config/rocketmq"
)

func Init() {
	mysql.Init()
	redis.Init()
	rocketmq.Init()
}
