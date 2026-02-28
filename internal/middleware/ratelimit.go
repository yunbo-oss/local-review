package middleware

import (
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
	"local-review-go/pkg/httpx"
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
			c.JSON(http.StatusTooManyRequests, httpx.Fail[string]("系统繁忙，请稍后重试"))
			c.Abort()
			return
		}
		c.Next()
	}
}

// --- 登录/验证码按 IP 限流 ---

type ipVisitor struct {
	limiter    *rate.Limiter
	lastAccess time.Time
}

var (
	authLimiters   = make(map[string]*ipVisitor)
	authLimitersMu sync.RWMutex
)

const (
	authCleanupInterval = 5 * time.Minute
	authVisitorTTL      = 10 * time.Minute
)

func init() {
	go cleanupAuthLimiters()
}

func cleanupAuthLimiters() {
	ticker := time.NewTicker(authCleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		authLimitersMu.Lock()
		now := time.Now()
		for ip, v := range authLimiters {
			if now.Sub(v.lastAccess) > authVisitorTTL {
				delete(authLimiters, ip)
			}
		}
		authLimitersMu.Unlock()
	}
}

func getAuthLimiter(ip string, limit rate.Limit, burst int) *rate.Limiter {
	authLimitersMu.RLock()
	v, ok := authLimiters[ip]
	authLimitersMu.RUnlock()
	if ok {
		v.lastAccess = time.Now()
		return v.limiter
	}
	authLimitersMu.Lock()
	defer authLimitersMu.Unlock()
	// double-check
	if v, ok = authLimiters[ip]; ok {
		v.lastAccess = time.Now()
		return v.limiter
	}
	lim := rate.NewLimiter(limit, burst)
	authLimiters[ip] = &ipVisitor{limiter: lim, lastAccess: time.Now()}
	return lim
}

// LoginRateLimit 登录接口按 IP 限流，默认每 IP 每分钟 5 次
// 压测时可通过 LOGIN_RATE_LIMIT、LOGIN_RATE_BURST 调高（如 120、60 支持 51 用户快速登录）
func LoginRateLimit() gin.HandlerFunc {
	limit := 5.0 / 60
	burst := 5
	if v, ok := os.LookupEnv("LOGIN_RATE_LIMIT"); ok {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			limit = float64(i) / 60
		}
	}
	if v, ok := os.LookupEnv("LOGIN_RATE_BURST"); ok {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			burst = i
		}
	}
	return perIPRateLimit(rate.Limit(limit), burst, "请求过于频繁，请稍后重试")
}

// SendCodeRateLimit 验证码接口按 IP 限流，默认每 IP 每分钟 3 次
func SendCodeRateLimit() gin.HandlerFunc {
	return perIPRateLimit(3.0/60, 3, "验证码发送过于频繁，请稍后重试")
}

func perIPRateLimit(limit rate.Limit, burst int, errMsg string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if ip == "" {
			ip = "unknown"
		}
		limiter := getAuthLimiter(ip, limit, burst)
		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, httpx.Fail[string](errMsg))
			c.Abort()
			return
		}
		c.Next()
	}
}
