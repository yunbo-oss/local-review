package middleware

import (
	"net/http"
	"sync"
	"time"

	"local-review-go/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Agent 用户级限流：默认每用户 10 次/分钟（低于普通读接口）
const (
	agentRateLimitWindow = time.Minute
	agentRateLimitMax    = 10
)

type agentLimiter struct {
	mu   sync.Mutex
	hits map[int64][]time.Time
}

var globalAgentLimiter = &agentLimiter{hits: map[int64][]time.Time{}}

// AgentRateLimit 按 JWT userID 限流；超限 429，不进入 LLM
func AgentRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := GetUserInfo(c)
		if err != nil || user.Id <= 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, httpx.Fail[string]("未登录"))
			return
		}
		if !globalAgentLimiter.allow(user.Id) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, httpx.Fail[string]("请求过于频繁，请稍后再试"))
			return
		}
		c.Next()
	}
}

func (l *agentLimiter) allow(userID int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-agentRateLimitWindow)
	arr := l.hits[userID]
	kept := arr[:0]
	for _, t := range arr {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= agentRateLimitMax {
		l.hits[userID] = kept
		return false
	}
	l.hits[userID] = append(kept, now)
	return true
}
