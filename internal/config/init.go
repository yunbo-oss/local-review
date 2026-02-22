package config

import (
	"local-review-go/internal/config/mysql"
	"local-review-go/internal/config/redis"
)

func Init() {
	mysql.Init()
	redis.Init()
}
