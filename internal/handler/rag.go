package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"local-review-go/internal/logic"
	repoInterfaces "local-review-go/internal/repository/interface"
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
	// 可选：预过滤条件（硬性条件，必须满足）
	Filter *ChatFilter `json:"filter,omitempty"`
}

// ChatFilter 向量检索预过滤（对应 VectorSearchFilter）
type ChatFilter struct {
	Area         string   `json:"area,omitempty"`          // 区域，如 "朝阳区"
	TypeName     string   `json:"typeName,omitempty"`       // 类型，如 "美食"
	MaxPrice     *int64   `json:"maxPrice,omitempty"`       // 人均上限
	MinPrice     *int64   `json:"minPrice,omitempty"`       // 人均下限
	MinScore     *int     `json:"minScore,omitempty"`       // 评分下限
	MinComments  *int     `json:"minComments,omitempty"`     // 评论数下限
	MaxDistance  *float64 `json:"maxDistance,omitempty"`    // 语义相似度阈值（COSINE 距离上限，越小越严）
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
	filter := chatFilterToVectorFilter(req.Filter)
	err := h.ragLogic.ChatWithFilter(ctx, req.Question, filter, func(chunk string) {
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

func chatFilterToVectorFilter(f *ChatFilter) *repoInterfaces.VectorSearchFilter {
	if f == nil {
		return nil
	}
	v := &repoInterfaces.VectorSearchFilter{
		Area: f.Area,
		TypeName: f.TypeName,
	}
	if f.MaxPrice != nil {
		v.MaxPrice = *f.MaxPrice
	}
	if f.MinPrice != nil {
		v.MinPrice = *f.MinPrice
	}
	if f.MinScore != nil {
		v.MinScore = *f.MinScore
	}
	if f.MinComments != nil {
		v.MinComments = *f.MinComments
	}
	if f.MaxDistance != nil {
		v.MaxDistance = *f.MaxDistance
	}
	// 全空则返回 nil
	if v.Area == "" && v.TypeName == "" && v.MaxPrice == 0 && v.MinPrice == 0 && v.MinScore == 0 && v.MinComments == 0 && v.MaxDistance == 0 {
		return nil
	}
	return v
}
