package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func TestAllowAgentRequest_SharedWindow(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()

	const max = 3
	for i := 0; i < max; i++ {
		ok, err := allowAgentRequest(ctx, rdb, 7, max, 60_000)
		if err != nil || !ok {
			t.Fatalf("i=%d ok=%v err=%v", i, ok, err)
		}
	}
	ok, err := allowAgentRequest(ctx, rdb, 7, max, 60_000)
	if err != nil || ok {
		t.Fatalf("4th should deny: ok=%v err=%v", ok, err)
	}
	// 另一用户不受影响
	ok, err = allowAgentRequest(ctx, rdb, 8, max, 60_000)
	if err != nil || !ok {
		t.Fatalf("other user: %v %v", ok, err)
	}
}

func TestAgentRateLimitMiddleware_429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	r := gin.New()
	r.POST("/x", func(c *gin.Context) {
		c.Set("claims", &CustomClaims{AuthUser: AuthUser{Id: 1}})
		c.Next()
	}, AgentRateLimitWith(rdb, 2, 60_000), func(c *gin.Context) {
		c.String(200, "ok")
	})

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("i=%d code=%d", i, w.Code)
		}
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 got %d", w.Code)
	}
}
