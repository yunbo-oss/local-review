package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"local-review-go/src/dto"
	"local-review-go/src/model"
	"local-review-go/src/service"
	"net/http"
	"strconv"
)

type VoucherHandler struct {
}

var voucherHandler *VoucherHandler

// @Description: add the normal voucher
// @Router: /voucher [POST]
func (*VoucherHandler) AddVoucher(c *gin.Context) {
	var voucher model.Voucher
	err := c.ShouldBindJSON(&voucher)
	if err != nil {
		logrus.Error("bind json failed")
		c.JSON(http.StatusOK, dto.Fail[string]("bind json failed"))
		return
	}
	err = service.VoucherManager.AddVoucher(&voucher)
	if err != nil {
		logrus.Error("add voucher failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("add voucher failed!"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData(voucher.Id))
}

// @Description: add seckill voucher
// @Router: /voucher/seckill [POST]
func (*VoucherHandler) AddSecKillVoucher(c *gin.Context) {
	var voucher model.Voucher
	err := c.ShouldBindJSON(&voucher)
	if err != nil {
		logrus.Error("failed to bind json")
		c.JSON(http.StatusOK, dto.Fail[string]("failed to bind json"))
	}
	err = service.VoucherManager.AddSeckillVoucher(&voucher)
	if err != nil {
		logrus.Error("add seckill voucher failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("add seckill failed!"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData(voucher.Id))
}

// @Description: query voucher by shop
// @Router: /voucher/list/:shopId [GET]
func (*VoucherHandler) QueryVoucherOfShop(c *gin.Context) {
	idStr := c.Param("shopId")
	if idStr == "" {
		logrus.Error("the id is empty")
		c.JSON(http.StatusOK, dto.Fail[string]("the id is empty"))
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logrus.Error("parse int failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("parse int failed!"))
		return
	}
	vouchers, err := service.VoucherManager.QueryVoucherOfShop(id)

	if err != nil {
		logrus.Error("get voucher failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("get voucher failed!"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData(vouchers))
}
