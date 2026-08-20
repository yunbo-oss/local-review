package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"local-review-go/internal/agent"
	"local-review-go/internal/llm"
	"local-review-go/internal/logic"
	"local-review-go/internal/middleware"
	"local-review-go/pkg/httpx"
)

// RecommendHandler 统一推荐入口：Router → RAG / Agent
type RecommendHandler struct {
	router RecommendRouterFace
	agent  *AgentHandler
	rag    *RAGHandler
}

// RecommendRouterFace 便于测试
type RecommendRouterFace interface {
	Route(in logic.RouteInput) logic.RouteDecision
}

// NewRecommendHandler 创建统一入口
func NewRecommendHandler(router RecommendRouterFace, agent *AgentHandler, rag *RAGHandler) *RecommendHandler {
	return &RecommendHandler{router: router, agent: agent, rag: rag}
}

// Recommend POST /api/recommend
func (h *RecommendHandler) Recommend(c *gin.Context) {
	user, err := middleware.GetUserInfo(c)
	if err != nil || user.Id <= 0 {
		c.JSON(http.StatusUnauthorized, httpx.Fail[string]("未登录"))
		return
	}
	var req RecommendReq
	if err := httpx.BindJSON(c, &req); err != nil {
		return
	}
	if errMsg := validateRecommendReq(req); errMsg != "" {
		c.JSON(http.StatusBadRequest, httpx.Fail[string](errMsg))
		return
	}
	routeInput := logic.RouteInput{
		Question: req.Question, ForceRoute: req.ForceRoute,
	}
	if h.agent != nil && h.agent.logic != nil {
		if provider, ok := h.agent.logic.(logic.RecommendRouteContextProvider); ok {
			enriched, enrichErr := provider.RecommendationRouteInput(
				c.Request.Context(), user.Id, req.SessionID, req.Question, req.ForceRoute,
			)
			if enrichErr != nil {
				// Memory is an enrichment signal. A transient read failure still
				// allows the deterministic/LLM router to use the current request.
				logrus.Warnf("recommend route context lookup failed user=%d session=%s: %v", user.Id, req.SessionID, enrichErr)
			} else {
				routeInput = enriched
			}
		} else if logic.NeedsSessionHistory(req.Question) {
			hasHistory, historyErr := h.agent.logic.HasSessionHistory(c.Request.Context(), user.Id, req.SessionID)
			if historyErr != nil {
				logrus.Warnf("recommend route session lookup failed user=%d session=%s: %v", user.Id, req.SessionID, historyErr)
			} else {
				routeInput.HasHistory = hasHistory
			}
		}
	}
	d := h.router.Route(routeInput)
	intentSpec := agent.FallbackIntentSpec(req.Question, string(d.Route))
	var understandingUsage llm.TokenUsage
	if contextual, ok := h.router.(logic.ContextRecommendRouter); ok {
		var routeErr error
		d, intentSpec, understandingUsage, routeErr = contextual.RouteContext(c.Request.Context(), routeInput)
		if routeErr != nil {
			// RouteContext already returns the deterministic fallback decision.
			logrus.Warnf("adaptive recommend route fell back: %v", routeErr)
		}
		c.Request = c.Request.WithContext(agent.WithIntentResult(c.Request.Context(), intentSpec, understandingUsage))
	}
	if d.Route == logic.RouteClarify {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)
		clarification := strings.TrimSpace(intentSpec.ClarificationQuestion)
		if clarification == "" {
			clarification = "请补充想找的区域、店铺类型或具体店名；如果是在追问上一轮，请在同一会话中提供可识别的对象。"
		}
		h.recordNonAgentTurn(c.Request.Context(), user.Id, req.SessionID, req.Question, "推荐结果：无\n"+clarification)
		c.SSEvent("message", clarification)
		done, _ := json.Marshal(map[string]any{
			"route":        string(d.Route),
			"route_reason": d.Reason,
			"forced":       d.Forced,
			"intent":       intentSpec.Intent,
			"confidence":   intentSpec.Confidence,
		})
		c.SSEvent("done", string(done))
		c.Writer.Flush()
		return
	}
	route := d.Route

	if logic.IsAgentRoute(route) {
		if h.agent == nil {
			c.JSON(http.StatusServiceUnavailable, httpx.Fail[string]("Agent 服务未配置"))
			return
		}
		// 强制沿用统一入口已判定的路由（含 force）
		if strings.TrimSpace(req.ForceRoute) == "" {
			req.ForceRoute = string(route)
		}
		h.agent.streamRecommend(c, user.Id, req)
		return
	}
	if h.rag == nil || h.rag.ragLogic == nil {
		c.JSON(http.StatusServiceUnavailable, httpx.Fail[string]("RAG 服务未配置"))
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	ctx := c.Request.Context()
	var answer strings.Builder
	err = h.rag.ragLogic.ChatForUser(ctx, user.Id, req.Question, nil, func(chunk string) {
		answer.WriteString(chunk)
		c.SSEvent("message", chunk)
		c.Writer.Flush()
	})
	if err != nil {
		c.SSEvent("error", err.Error())
		c.Writer.Flush()
		return
	}
	h.recordNonAgentTurn(ctx, user.Id, req.SessionID, req.Question, answer.String())
	done, _ := json.Marshal(map[string]any{
		"route":        string(route),
		"route_reason": d.Reason,
		"forced":       d.Forced,
		"intent":       intentSpec.Intent,
		"confidence":   intentSpec.Confidence,
	})
	c.SSEvent("done", string(done))
	c.Writer.Flush()
}

func (h *RecommendHandler) recordNonAgentTurn(ctx context.Context, userID int64, sessionID, question, answer string) {
	if h == nil || h.agent == nil || h.agent.logic == nil {
		return
	}
	recorder, ok := h.agent.logic.(logic.RecommendTurnRecorder)
	if !ok {
		return
	}
	if err := recorder.RecordRecommendationTurn(ctx, userID, sessionID, question, answer); err != nil {
		logrus.Warnf("record routed recommendation turn user=%d session=%s: %v", userID, sessionID, err)
	}
}
