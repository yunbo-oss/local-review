package handler

import (
	"local-review-go/src/httpx"
	"local-review-go/src/logic"
	"local-review-go/src/middleware"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type VoucherOrderHandler struct {
	logic logic.VoucherOrderLogic
}

func NewVoucherOrderHandler(voucherOrderLogic logic.VoucherOrderLogic) *VoucherOrderHandler {
	return &VoucherOrderHandler{logic: voucherOrderLogic}
}

// @Description: get the voucher id
// @Router: /voucher-order/seckill/:id
func (h *VoucherOrderHandler) SeckillVoucher(c *gin.Context) {

	idStr := c.Param("id")
	if idStr == "" {
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("voucher id is required"))
		return
	}

	var id int64
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("voucher id is invalid"))
		return
	}

	userInfo, err := middleware.GetUserInfo(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, httpx.Fail[string]("unauthorized"))
		return
	}

	userId := userInfo.Id
	ctx := c.Request.Context()
	err = h.logic.SeckillVoucher(ctx, id, userId)

	if err != nil {
		// 根据错误类型判断状态码
		errorMsg := err.Error()
		if errorMsg == "秒杀尚未开始" || errorMsg == "秒杀已结束" {
			c.JSON(http.StatusBadRequest, httpx.Fail[string](errorMsg))
		} else if errorMsg == "the condition is not meet" {
			c.JSON(http.StatusConflict, httpx.Fail[string]("seckill failed: stock insufficient or already purchased"))
		} else {
			c.JSON(http.StatusInternalServerError, httpx.Fail[string](errorMsg))
		}
		return
	}

	c.JSON(http.StatusOK, httpx.Ok[string]())
}
