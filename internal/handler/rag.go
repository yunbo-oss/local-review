package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"local-review-go/internal/logic"
	"local-review-go/pkg/httpx"
)

// RAGHandler RAG 智能点评 Handler
type RAGHandler struct {
	ragLogic logic.RAGLogic
}

// NewRAGHandler 创建 RAG Handler
func NewRAGHandler(ragLogic logic.RAGLogic) *RAGHandler {
	return &RAGHandler{ragLogic: ragLogic}
}

// ChatReq 请求体
type ChatReq struct {
	Question string `json:"question" binding:"required"`
}

// Chat 智能点评对话（SSE 流式）
// POST /api/rag/chat
func (h *RAGHandler) Chat(c *gin.Context) {
	if h.ragLogic == nil {
		c.JSON(http.StatusServiceUnavailable, httpx.Fail[string]("RAG 服务未配置"))
		return
	}

	var req ChatReq
	if err := httpx.BindJSON(c, &req); err != nil {
		return // BindJSON 已写入响应
	}
	if req.Question == "" {
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("问题不能为空"))
		return
	}

	// SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 禁用 Nginx 缓冲
	c.Status(http.StatusOK)
	c.Writer.Flush()

	ctx := c.Request.Context()
	err := h.ragLogic.Chat(ctx, req.Question, func(chunk string) {
		c.SSEvent("message", chunk)
		c.Writer.Flush()
	})
	if err != nil {
		c.SSEvent("error", err.Error())
		c.Writer.Flush()
		return
	}
	c.SSEvent("done", "")
	c.Writer.Flush()
}
