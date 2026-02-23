package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"local-review-go/src/dto"
	"local-review-go/src/service"
	"net/http"
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
	shopTypeList, err := service.ShopTypeManager.QueryTypeListWithCacheList()
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("failed to get type list"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData(shopTypeList))
}
