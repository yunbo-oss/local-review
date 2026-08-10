package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
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
	hasHistory := false
	if logic.NeedsSessionHistory(req.Question) && h.agent != nil && h.agent.logic != nil {
		hasHistory, err = h.agent.logic.HasSessionHistory(c.Request.Context(), user.Id, req.SessionID)
		if err != nil {
			// 记忆是路由增强信号，读取失败时保留一次性 RAG 的降级能力。
			logrus.Warnf("recommend route session lookup failed user=%d session=%s: %v", user.Id, req.SessionID, err)
			hasHistory = false
		}
	}
	d := h.router.Route(logic.RouteInput{
		Question: req.Question, ForceRoute: req.ForceRoute, HasHistory: hasHistory,
	})
	if d.Route == logic.RouteClarify {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)
		c.SSEvent("message", "请补充想找的区域、店铺类型或具体店名；如果是在追问上一轮，请在同一会话中提供可识别的对象。")
		done, _ := json.Marshal(map[string]any{
			"route":        string(d.Route),
			"route_reason": d.Reason,
			"forced":       d.Forced,
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
	err = h.rag.ragLogic.ChatForUser(ctx, user.Id, req.Question, nil, func(chunk string) {
		c.SSEvent("message", chunk)
		c.Writer.Flush()
	})
	if err != nil {
		c.SSEvent("error", err.Error())
		c.Writer.Flush()
		return
	}
	done, _ := json.Marshal(map[string]any{
		"route":        string(route),
		"route_reason": d.Reason,
		"forced":       d.Forced,
	})
	c.SSEvent("done", string(done))
	c.Writer.Flush()
}
