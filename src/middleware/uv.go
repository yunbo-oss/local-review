package middleware

import (
	"context"
	"github.com/gin-gonic/gin"
	"local-review-go/src/utils/redisx"

	"github.com/sirupsen/logrus"
	redisClient "local-review-go/src/config/redis"
	"strconv"
	"time"
)

// UVStatisticsMiddleware UV统计中间件
func UVStatisticsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 只统计GET请求（根据需求调整）
		if c.Request.Method != "GET" {
			c.Next()
			return
		}

		// 获取当前日期
		today := time.Now().Format("20060102")
		key := redisx.UVKeyPrefix + today

		// 获取访问者标识
		visitor := getVisitorIdentity(c)

		// 使用Pipeline提高性能
		pipe := redisClient.GetRedisClient().Pipeline()
		pipe.PFAdd(context.Background(), key, visitor)
		pipe.Expire(context.Background(), key, 365*24*time.Hour) // 1年过期
		_, err := pipe.Exec(context.Background())

		if err != nil {
			logrus.Errorln("UV统计失败:", err)
		}

		c.Next()
	}
}

// 获取访问者唯一标识
func getVisitorIdentity(c *gin.Context) string {
	// 优先使用已登录用户ID
	if claims, exists := c.Get("claims"); exists {
		if customClaims, ok := claims.(*CustomClaims); ok {
			return strconv.FormatInt(customClaims.AuthUser.Id, 10)
		}
	}

	// 未登录用户使用IP+UserAgent组合
	return c.ClientIP() + "|" + c.Request.UserAgent()
}
