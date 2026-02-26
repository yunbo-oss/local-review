package config

import (
	"local-review-go/internal/config/log"
	"local-review-go/internal/config/mysql"
	"local-review-go/internal/config/otel"
	"local-review-go/internal/config/redis"
	"local-review-go/internal/config/rocketmq"
)

func Init() {
	log.Init() // 最先初始化日志，后续组件日志可被正确格式化
	otel.Init() // OpenTelemetry Trace，未配置 endpoint 时自动降级为 noop
	mysql.Init()
	redis.Init()
	rocketmq.Init()
}
