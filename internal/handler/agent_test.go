package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"local-review-go/internal/agent"
	"local-review-go/internal/llm"
	"local-review-go/internal/logic"
	"local-review-go/internal/memory"
	"local-review-go/internal/middleware"
)

type fakeRecommendLogic struct {
	answer           string
	err              error
	status           []agent.ToolStatus
	history          bool
	recordedSession  string
	recordedQuestion string
	recordedAnswer   string
}

func (f *fakeRecommendLogic) HasSessionHistory(context.Context, int64, string) (bool, error) {
	return f.history, nil
}

func (f *fakeRecommendLogic) RecordRecommendationTurn(_ context.Context, _ int64, sessionID, question, answer string) error {
	f.recordedSession = sessionID
	f.recordedQuestion = question
	f.recordedAnswer = answer
	return nil
}

func (f *fakeRecommendLogic) Recommend(ctx context.Context, userID int64, sessionID, question, forceRoute string, onStatus func(agent.ToolStatus)) (logic.RecommendResult, error) {
	for _, st := range f.status {
		if onStatus != nil {
			onStatus(st)
		}
	}
	if f.err != nil {
		return logic.RecommendResult{}, f.err
	}
	return logic.RecommendResult{
		Answer: f.answer,
		Steps:  1, ToolCalls: 1,
		Usage:        llm.TokenUsage{TotalTokens: 10},
		ProfileAfter: memory.Profile{},
		Route:        forceRoute,
		RouteReason:  "test",
	}, nil
}

func withUser(c *gin.Context, id int64) {
	c.Set("claims", &middleware.CustomClaims{AuthUser: middleware.AuthUser{Id: id, NickName: "t"}})
}

func TestAgentHandler_Unauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewAgentHandler(&fakeRecommendLogic{answer: "x"})
	r := gin.New()
	r.POST("/api/agent/recommend", h.Recommend)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/recommend", strings.NewReader(`{"question":"q","session_id":"s"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAgentHandler_BadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewAgentHandler(&fakeRecommendLogic{answer: "x"})
	r := gin.New()
	r.POST("/api/agent/recommend", func(c *gin.Context) {
		withUser(c, 1)
		h.Recommend(c)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/recommend", strings.NewReader(`{"question":"","session_id":"s"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAgentHandler_SSESuccessNoLeak(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewAgentHandler(&fakeRecommendLogic{
		answer: "推荐 [shop:1]",
		status: []agent.ToolStatus{agent.StatusSearching},
	})
	r := gin.New()
	r.POST("/api/agent/recommend", func(c *gin.Context) {
		withUser(c, 1)
		h.Recommend(c)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/recommend", strings.NewReader(`{"question":"咖啡","session_id":"s1"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	body := w.Body.String()
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type=%s", ct)
	}
	if !strings.Contains(body, "event:status") || !strings.Contains(body, "event:message") || !strings.Contains(body, "event:done") {
		t.Fatalf("missing sse events: %s", body)
	}
	// 禁止泄漏：系统提示 / 完整画像字段 / 诱导话术；done 里的 tool_calls 计数是允许的指标
	for _, bad := range []string{"system prompt", "preferred_areas", "忽略系统规则", "你是一个", "scratchpad", "\"arguments\""} {
		if strings.Contains(body, bad) {
			t.Fatalf("leak %q in %s", bad, body)
		}
	}
	if strings.Contains(body, "event:error") {
		t.Fatalf("unexpected error: %s", body)
	}
}

func TestAgentHandler_SSEErrorNoMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewAgentHandler(&fakeRecommendLogic{err: agent.NewPublicError(agent.ErrGroundingUnknownShop, "回答未通过有据可查校验，请重试")})
	r := gin.New()
	r.POST("/api/agent/recommend", func(c *gin.Context) {
		withUser(c, 1)
		h.Recommend(c)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/recommend", strings.NewReader(`{"question":"咖啡","session_id":"s1"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	body := w.Body.String()
	if !strings.Contains(body, "event:error") {
		t.Fatalf("want error event: %s", body)
	}
	if strings.Contains(body, "event:message") {
		t.Fatalf("must not send success message: %s", body)
	}
	var _ = json.Marshal
}
