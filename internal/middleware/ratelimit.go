package middleware

import (
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

var (
	seckillLimiter     *rate.Limiter
	seckillLimiterOnce sync.Once
)

// getSeckillRateLimit 从环境变量读取秒杀限流配置，默认 1000 QPS，burst 2000
func getSeckillRateLimit() (r rate.Limit, b int) {
	r = 1000
	b = 2000
	if v, ok := os.LookupEnv("SECKILL_RATE_LIMIT"); ok {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			r = rate.Limit(i)
		}
	}
	if v, ok := os.LookupEnv("SECKILL_RATE_BURST"); ok {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			b = i
		}
	}
	return r, b
}

// SeckillRateLimit 秒杀接口限流中间件，超限返回 429
func SeckillRateLimit() gin.HandlerFunc {
	seckillLimiterOnce.Do(func() {
		r, b := getSeckillRateLimit()
		seckillLimiter = rate.NewLimiter(r, b)
	})
	return func(c *gin.Context) {
		if !seckillLimiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success":  false,
				"errorMsg": "系统繁忙，请稍后重试",
				"data":     nil,
				"total":    0,
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
