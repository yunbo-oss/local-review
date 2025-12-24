package handler

import (
	"local-review-go/src/dto"
	"local-review-go/src/middleware"
	"local-review-go/src/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type VoucherOrderHandler struct {
}

var voucherOrderHandler *VoucherOrderHandler

// @Description: get the voucher id
// @Router: /voucher-order/seckill/:id
func (*VoucherOrderHandler) SeckillVoucher(c *gin.Context) {

	idStr := c.Param("id")
	if idStr == "" {
		c.JSON(http.StatusBadRequest, dto.Fail[string]("voucher id is required"))
		return
	}

	var id int64
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[string]("voucher id is invalid"))
		return
	}

	userInfo, err := middleware.GetUserInfo(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.Fail[string]("unauthorized"))
		return
	}

	userId := userInfo.Id
	ctx := c.Request.Context()
	err = service.VoucherOrderManager.SeckillVoucher(ctx, id, userId)

	if err != nil {
		// 根据错误类型判断状态码
		errorMsg := err.Error()
		if errorMsg == "秒杀尚未开始" || errorMsg == "秒杀已结束" {
			c.JSON(http.StatusBadRequest, dto.Fail[string](errorMsg))
		} else if errorMsg == "the condition is not meet" {
			c.JSON(http.StatusConflict, dto.Fail[string]("seckill failed: stock insufficient or already purchased"))
		} else {
			c.JSON(http.StatusInternalServerError, dto.Fail[string](errorMsg))
		}
		return
	}

	c.JSON(http.StatusOK, dto.Ok[string]())
}
