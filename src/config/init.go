package config

import (
	"local-review-go/src/config/mysql"
	"local-review-go/src/config/redis"
)

func Init() {
	mysql.Init()
	redis.Init()
}
