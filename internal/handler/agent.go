package handler

import (
	"encoding/json"
	"net/http"
	"strings"

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
	Question   string `json:"question" binding:"required"`
	SessionID  string `json:"session_id" binding:"required"`
	ForceRoute string `json:"force_route"` // 可选：rag_oneshot|agent_multistep|agent_memory|clarify
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
	if errMsg := validateRecommendReq(req); errMsg != "" {
		c.JSON(http.StatusBadRequest, httpx.Fail[string](errMsg))
		return
	}
	h.streamRecommend(c, user.Id, req)
}

// streamRecommend 已解析请求后的 SSE 执行（供统一入口复用，避免二次 BindJSON）
func (h *AgentHandler) streamRecommend(c *gin.Context, userID int64, req RecommendReq) {
	if h.logic == nil {
		c.JSON(http.StatusServiceUnavailable, httpx.Fail[string]("Agent 服务未配置"))
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	ctx := c.Request.Context()
	if traceID := agent.TraceIDFromContext(ctx); traceID != "" {
		started, _ := json.Marshal(map[string]any{"trace_id": traceID})
		c.SSEvent("run_started", string(started))
		c.Writer.Flush()
	}
	res, err := h.logic.Recommend(ctx, userID, req.SessionID, req.Question, req.ForceRoute, func(st agent.ToolStatus) {
		c.SSEvent("status", string(st))
		c.Writer.Flush()
	})
	if err != nil {
		c.SSEvent("error", agent.PublicMessage(err))
		c.Writer.Flush()
		return
	}
	c.SSEvent("message", res.Answer)
	c.Writer.Flush()
	donePayload := map[string]any{
		"steps":                res.Steps,
		"tool_calls":           res.ToolCalls,
		"tokens":               res.Usage.TotalTokens,
		"trace_id":             res.TraceID,
		"route":                res.Route,
		"route_reason":         res.RouteReason,
		"intent":               res.Intent.Intent,
		"replans":              res.Replans,
		"plan_versions":        len(res.Plans),
		"runtime_version":      res.RuntimeVersion,
		"runtime_status":       res.RuntimeStatus,
		"retrieval_decision":   res.Retrieval.Decision,
		"retrieval_confidence": res.Retrieval.Confidence,
	}
	if res.Degraded {
		donePayload["degraded"] = true
		donePayload["degraded_reason"] = res.DegradedReason
	}
	done, _ := json.Marshal(donePayload)
	c.SSEvent("done", string(done))
	c.Writer.Flush()
}

func validateRecommendReq(req RecommendReq) string {
	if req.Question == "" || req.SessionID == "" {
		return "question 与 session_id 不能为空"
	}
	if fr := strings.TrimSpace(req.ForceRoute); fr != "" {
		switch fr {
		case string(logic.RouteRAGOneshot), string(logic.RouteAgentMultistep),
			string(logic.RouteAgentMemory), string(logic.RouteClarify):
		default:
			return "force_route 非法"
		}
	}
	return ""
}
