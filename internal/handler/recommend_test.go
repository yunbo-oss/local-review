package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"local-review-go/internal/logic"
	repoInterfaces "local-review-go/internal/repository/interface"
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

type routedRAGStub struct {
	answer string
}

func (s routedRAGStub) Chat(_ context.Context, _ string, onChunk func(string)) error {
	onChunk(s.answer)
	return nil
}

func (s routedRAGStub) ChatWithFilter(_ context.Context, _ string, _ *repoInterfaces.VectorSearchFilter, onChunk func(string)) error {
	onChunk(s.answer)
	return nil
}

func (s routedRAGStub) ChatForUser(_ context.Context, _ int64, _ string, _ *repoInterfaces.VectorSearchFilter, onChunk func(string)) error {
	onChunk(s.answer)
	return nil
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
	logicStub := &fakeRecommendLogic{}
	h := NewRecommendHandler(router, NewAgentHandler(logicStub), nil)

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
	if logicStub.recordedSession != "missing" || !strings.Contains(logicStub.recordedAnswer, "推荐结果：无") {
		t.Fatalf("clarification turn was not recorded: %+v", logicStub)
	}
}

func TestRecommendHandlerPersistsRAGAnswerForFollowupRouting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := &capturingRecommendRouter{decision: logic.RouteDecision{
		Route: logic.RouteRAGOneshot, Reason: "simple_search", Confidence: 0.9,
	}}
	logicStub := &fakeRecommendLogic{}
	h := NewRecommendHandler(
		router,
		NewAgentHandler(logicStub),
		NewRAGHandler(routedRAGStub{answer: "推荐结果：[shop:7]\n七号咖啡符合要求。"}),
	)

	r := gin.New()
	r.POST("/api/recommend", func(c *gin.Context) {
		withUser(c, 9)
		h.Recommend(c)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/recommend", strings.NewReader(`{"question":"海淀咖啡","session_id":"rag-followup"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"route":"rag_oneshot"`) {
		t.Fatalf("unexpected RAG response code=%d body=%s", w.Code, w.Body.String())
	}
	if logicStub.recordedSession != "rag-followup" || logicStub.recordedQuestion != "海淀咖啡" ||
		!strings.Contains(logicStub.recordedAnswer, "[shop:7]") {
		t.Fatalf("RAG answer was not recorded for follow-up: %+v", logicStub)
	}
}
