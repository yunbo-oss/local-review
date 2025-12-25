package handler

import (
	"local-review-go/src/httpx"
	"local-review-go/src/logic"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type StatisticsHandler struct {
	logic logic.StatisticsLogic
}

func NewStatisticsHandler(statLogic logic.StatisticsLogic) *StatisticsHandler {
	return &StatisticsHandler{logic: statLogic}
}

// QueryUV 查询UV数据
// @Summary 查询UV统计
// @Description 查询指定日期的UV统计
// @Tags 统计分析
// @Param date query string true "日期（格式：YYYYMMDD）"
// @Success 200 {object} dto.Result[int64]
// @Router /statistics/uv [GET]
func (h *StatisticsHandler) QueryUV(c *gin.Context) {
	date := c.Query("date")
	if date == "" {
		c.JSON(http.StatusBadRequest, httpx.Fail[int64]("日期参数不能为空"))
		return
	}

	// 验证日期格式
	if _, err := time.Parse("20060102", date); err != nil {
		c.JSON(http.StatusBadRequest, httpx.Fail[int64]("日期格式错误，应为YYYYMMDD"))
		return
	}

	ctx := c.Request.Context()
	count, err := h.logic.QueryUV(ctx, date)
	if err != nil {
		logrus.Errorln("查询UV失败:", err)
		c.JSON(http.StatusInternalServerError, httpx.Fail[int64]("查询失败"))
		return
	}

	c.JSON(http.StatusOK, httpx.OkWithData(count))
}

// QueryCurrentUV 查询当日UV
// @Summary 查询当日UV
// @Tags 统计分析
// @Success 200 {object} dto.Result[int64]
// @Router /statistics/uv/current [GET]
func (h *StatisticsHandler) QueryCurrentUV(c *gin.Context) {
	ctx := c.Request.Context()
	count, err := h.logic.QueryCurrentUV(ctx)
	if err != nil {
		logrus.Errorln("查询当日UV失败:", err)
		c.JSON(http.StatusInternalServerError, httpx.Fail[int64]("查询失败"))
		return
	}

	c.JSON(http.StatusOK, httpx.OkWithData(count))
}
