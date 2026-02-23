package handler

import (
	"github.com/gin-gonic/gin"
	"local-review-go/src/dto"
	"local-review-go/src/middleware"
	"local-review-go/src/service"
	"net/http"
	"strconv"
)

type VoucherOrderHandler struct {
}

var voucherOrderHandler *VoucherOrderHandler

// @Description: get the voucher id
// @Router: /voucher-order/seckill/:id
func (*VoucherOrderHandler) SeckillVoucher(c *gin.Context) {

	idStr := c.Param("id")
	if idStr == "" {
		c.JSON(http.StatusOK, dto.Fail[string]("the id is empty!"))
		return
	}

	var id int64
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("type transform failed!"))
		return
	}

	userInfo, err := middleware.GetUserInfo(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("get user info failed!"))
		return
	}

	userId := userInfo.Id
	err = service.VoucherOrderManager.SeckillVoucher(id, userId)

	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.Ok[string]())
}
