package handler

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	redisClient "local-review-go/src/config/redis"
	"local-review-go/src/dto"
	"local-review-go/src/utils"
	"net/http"
	"time"
)

type StatisticsHandler struct {
}

var statisticsHandler *StatisticsHandler

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
		c.JSON(http.StatusBadRequest, dto.Fail[int64]("日期参数不能为空"))
		return
	}

	// 验证日期格式
	if _, err := time.Parse("20060102", date); err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[int64]("日期格式错误，应为YYYYMMDD"))
		return
	}

	key := utils.UVKeyPrefix + date
	count, err := redisClient.GetRedisClient().PFCount(context.Background(), key).Result()
	if err != nil {
		logrus.Errorln("查询UV失败:", err)
		c.JSON(http.StatusInternalServerError, dto.Fail[int64]("查询失败"))
		return
	}

	c.JSON(http.StatusOK, dto.OkWithData(count))
}

// QueryCurrentUV 查询当日UV
// @Summary 查询当日UV
// @Tags 统计分析
// @Success 200 {object} dto.Result[int64]
// @Router /statistics/uv/current [GET]
func (h *StatisticsHandler) QueryCurrentUV(c *gin.Context) {
	date := time.Now().Format("20060102")
	key := utils.UVKeyPrefix + date
	count, err := redisClient.GetRedisClient().PFCount(context.Background(), key).Result()
	if err != nil {
		logrus.Errorln("查询当日UV失败:", err)
		c.JSON(http.StatusInternalServerError, dto.Fail[int64]("查询失败"))
		return
	}

	c.JSON(http.StatusOK, dto.OkWithData(count))
}
