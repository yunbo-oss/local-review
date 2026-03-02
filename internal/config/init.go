package config

import (
	"bufio"
	"os"
	"strings"

	"local-review-go/internal/config/log"
	"local-review-go/internal/config/mysql"
	"local-review-go/internal/config/otel"
	"local-review-go/internal/config/redis"
	"local-review-go/internal/config/rocketmq"
)

// loadEnvFromFile 从 .env 加载环境变量（若存在），供 seed-vector、make run 等使用
func loadEnvFromFile() {
	f, err := os.Open(".env")
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "="); i > 0 {
			key := strings.TrimSpace(line[:i])
			val := strings.TrimSpace(line[i+1:])
			if key != "" && os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}
}

func Init() {
	loadEnvFromFile()
	log.Init() // 最先初始化日志，后续组件日志可被正确格式化
	otel.Init() // OpenTelemetry Trace，未配置 endpoint 时自动降级为 noop
	mysql.Init()
	redis.Init()
	rocketmq.Init()
}
