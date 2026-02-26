package handler

import (
	"context"
	"net/http"
	"time"

	"local-review-go/internal/config/mysql"
	"local-review-go/internal/config/redis"

	"github.com/gin-gonic/gin"
)

// Health 健康检查：供 Nginx upstream 被动健康检查使用
// 检查 MySQL、Redis 连通性，任一失败返回 503，Nginx 会将该实例从负载均衡中剔除
func Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	status := http.StatusOK
	checks := map[string]string{
		"mysql": "ok",
		"redis": "ok",
	}

	// 检查 MySQL
	sqlDB, _ := mysql.GetMysqlDB().DB()
	if sqlDB == nil {
		status = http.StatusServiceUnavailable
		checks["mysql"] = "not initialized"
	} else if err := sqlDB.PingContext(ctx); err != nil {
		status = http.StatusServiceUnavailable
		checks["mysql"] = "unreachable: " + err.Error()
	}

	// 检查 Redis
	if err := redis.GetRedisClient().Ping(ctx).Err(); err != nil {
		status = http.StatusServiceUnavailable
		checks["redis"] = "unreachable: " + err.Error()
	}

	c.JSON(status, gin.H{
		"status":  status,
		"checks":  checks,
		"service":  "local-review-go",
	})
}
