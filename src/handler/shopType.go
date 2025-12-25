package handler

import (
	"local-review-go/src/httpx"
	"local-review-go/src/logic"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type ShopTypeHandler struct {
	logic logic.ShopTypeLogic
}

func NewShopTypeHandler(shopTypeLogic logic.ShopTypeLogic) *ShopTypeHandler {
	return &ShopTypeHandler{logic: shopTypeLogic}
}

// @Description: query shop type list
// @Router: /shop-type/list  [GET]
// TODO Add cache
func (h *ShopTypeHandler) QueryShopTypeList(c *gin.Context) {
	ctx := c.Request.Context()
	shopTypeList, err := h.logic.QueryShopTypeList(ctx)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusInternalServerError, httpx.Fail[string]("failed to get type list"))
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData(shopTypeList))
}
