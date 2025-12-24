package handler

import (
	"local-review-go/src/httpx"
	"local-review-go/src/service"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type ShopTypeHandler struct {
}

var shopTypeHandler *ShopTypeHandler

// @Description: query shop type list
// @Router: /shop-type/list  [GET]
// TODO Add cache
func (*ShopTypeHandler) QueryShopTypeList(c *gin.Context) {
	// shopTypeList , err := service.ShopTypeManager.QueryShopTypeList()
	// shopTypeList, err := service.ShopTypeManager.QueryShopTypeListWithCache()
	ctx := c.Request.Context()
	shopTypeList, err := service.ShopTypeManager.QueryTypeListWithCacheList(ctx)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusInternalServerError, httpx.Fail[string]("failed to get type list"))
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData(shopTypeList))
}
