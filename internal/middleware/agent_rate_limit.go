package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	redisClient "local-review-go/internal/config/redis"
	"local-review-go/pkg/httpx"
	"local-review-go/pkg/utils/redisx"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

// Agent 用户级限流：默认每用户 10 次/分钟（多实例共享 Redis）
const (
	agentRateLimitWindowMs = 60_000
	agentRateLimitMax      = 10
)

// 滑动窗口：ZSET score=毫秒时间戳；原子 ZREMRANGE + ZCARD + ZADD
var agentRateLimitLua = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local maxn = tonumber(ARGV[3])
local member = ARGV[4]
redis.call('ZREMRANGEBYSCORE', key, 0, now - window)
local n = redis.call('ZCARD', key)
if n >= maxn then
  return 0
end
redis.call('ZADD', key, now, member)
redis.call('PEXPIRE', key, window)
return 1
`)

// AgentRateLimit 按 JWT userID 限流；超限 429，不进入 LLM
func AgentRateLimit() gin.HandlerFunc {
	return AgentRateLimitWith(redisClient.GetRedisClient(), agentRateLimitMax, agentRateLimitWindowMs)
}

// AgentRateLimitWith 可注入 Redis（单测）
func AgentRateLimitWith(rdb *redis.Client, max int, windowMs int64) gin.HandlerFunc {
	if max <= 0 {
		max = agentRateLimitMax
	}
	if windowMs <= 0 {
		windowMs = agentRateLimitWindowMs
	}
	return func(c *gin.Context) {
		user, err := GetUserInfo(c)
		if err != nil || user.Id <= 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, httpx.Fail[string]("未登录"))
			return
		}
		ok, aerr := allowAgentRequest(c.Request.Context(), rdb, user.Id, max, windowMs)
		if aerr != nil {
			// Redis 故障：降级放行并打日志，避免误伤可用性（配额仍以成功写入为准）
			logrus.Warnf("agent rate limit redis: %v", aerr)
			c.Next()
			return
		}
		if !ok {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, httpx.Fail[string]("请求过于频繁，请稍后再试"))
			return
		}
		c.Next()
	}
}

func allowAgentRequest(ctx context.Context, rdb *redis.Client, userID int64, max int, windowMs int64) (bool, error) {
	if rdb == nil {
		return true, fmt.Errorf("redis nil")
	}
	now := time.Now().UnixMilli()
	member := strconv.FormatInt(now, 10) + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	key := redisx.AgentRateLimitKey(userID)
	v, err := agentRateLimitLua.Run(ctx, rdb, []string{key}, now, windowMs, max, member).Int()
	if err != nil {
		return false, err
	}
	return v == 1, nil
}
