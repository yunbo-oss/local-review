package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"local-review-go/internal/agent"
	"local-review-go/internal/logic"
	"local-review-go/internal/middleware"
	"local-review-go/pkg/httpx"
)

// AgentHandler 推荐 Agent SSE
type AgentHandler struct {
	logic logic.RecommendAgentLogic
}

// NewAgentHandler 创建
func NewAgentHandler(l logic.RecommendAgentLogic) *AgentHandler {
	return &AgentHandler{logic: l}
}

// RecommendReq 请求体
type RecommendReq struct {
	Question  string `json:"question" binding:"required"`
	SessionID string `json:"session_id" binding:"required"`
}

// Recommend POST /api/agent/recommend
func (h *AgentHandler) Recommend(c *gin.Context) {
	if h.logic == nil {
		c.JSON(http.StatusServiceUnavailable, httpx.Fail[string]("Agent 服务未配置"))
		return
	}
	user, err := middleware.GetUserInfo(c)
	if err != nil || user.Id <= 0 {
		c.JSON(http.StatusUnauthorized, httpx.Fail[string]("未登录"))
		return
	}
	var req RecommendReq
	if err := httpx.BindJSON(c, &req); err != nil {
		return
	}
	if req.Question == "" || req.SessionID == "" {
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("question 与 session_id 不能为空"))
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	ctx := c.Request.Context()
	res, err := h.logic.Recommend(ctx, user.Id, req.SessionID, req.Question, func(st agent.ToolStatus) {
		c.SSEvent("status", string(st))
		c.Writer.Flush()
	})
	if err != nil {
		c.SSEvent("error", err.Error())
		c.Writer.Flush()
		return
	}
	c.SSEvent("message", res.Answer)
	c.Writer.Flush()
	done, _ := json.Marshal(map[string]any{
		"steps":      res.Steps,
		"tool_calls": res.ToolCalls,
		"tokens":     res.Usage.TotalTokens,
	})
	c.SSEvent("done", string(done))
	c.Writer.Flush()
}
