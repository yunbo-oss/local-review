package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"local-review-go/internal/logic"
)

type capturingRecommendRouter struct {
	input    logic.RouteInput
	decision logic.RouteDecision
}

func (r *capturingRecommendRouter) Route(in logic.RouteInput) logic.RouteDecision {
	r.input = in
	if r.decision.Route != "" {
		return r.decision
	}
	return logic.RouteDecision{Route: logic.RouteAgentMemory, Reason: "session_followup"}
}

func TestRecommendHandlerPassesSessionHistoryToRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := &capturingRecommendRouter{}
	agentHandler := NewAgentHandler(&fakeRecommendLogic{answer: "推荐 [shop:1]", history: true})
	h := NewRecommendHandler(router, agentHandler, nil)

	r := gin.New()
	r.POST("/api/recommend", func(c *gin.Context) {
		withUser(c, 9)
		h.Recommend(c)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/recommend", strings.NewReader(`{"question":"还是上次那种安静的","session_id":"s-history"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	if !router.input.HasHistory {
		t.Fatalf("router input did not receive existing session history: %+v", router.input)
	}
	if !strings.Contains(w.Body.String(), `"route":"agent_memory"`) {
		t.Fatalf("response did not execute memory agent route: %s", w.Body.String())
	}
}

func TestRecommendHandlerReturnsClarificationWithoutCallingRAG(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := &capturingRecommendRouter{decision: logic.RouteDecision{
		Route: logic.RouteClarify, Reason: "missing_session_context", Confidence: 0.9,
	}}
	h := NewRecommendHandler(router, nil, nil)

	r := gin.New()
	r.POST("/api/recommend", func(c *gin.Context) {
		withUser(c, 9)
		h.Recommend(c)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/recommend", strings.NewReader(`{"question":"还是上次那种","session_id":"missing"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "请补充") ||
		!strings.Contains(w.Body.String(), `"route":"clarify"`) ||
		!strings.Contains(w.Body.String(), "event:done") {
		t.Fatalf("unexpected clarify SSE: %s", w.Body.String())
	}
}
