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

type ShopHandler struct {
}

var shopHandler *ShopHandler

// @Descirption: query shop by id
// @Router: /shop/{id} [GET]
// TODO add cache
func (*ShopHandler) QueryShopById(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		logrus.Error("id is empty!")
		c.JSON(http.StatusOK, dto.Fail[string]("id is emtpy!"))
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logrus.Error("transform failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("transform type failed!"))
		return
	}
	// shop, err := service.ShopManager.QueryShopById(id)

	// shop , err := service.ShopManager.QueryShopByIdWithCache(id)
	shop, err := service.ShopManager.QueryShopByIdWithCacheNull(id)

	if err != nil {
		logrus.Error("query failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("query failed!"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData[model.Shop](shop))
}

// @Descirption: save the shop info
// @Router: /shop [POST]
func (*ShopHandler) SaveShop(c *gin.Context) {
	var shop model.Shop
	err := c.ShouldBindJSON(&shop)
	if err != nil {
		logrus.Error("bind json failed")
		c.JSON(http.StatusOK, dto.Fail[string]("bind json failed!"))
		return
	}
	err = service.ShopManager.SaveShop(&shop)
	if err != nil {
		logrus.Error("save data failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("save data failed!"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData(shop.Id))
}

// @Descirption: update the shop info
// @Router: /shop [PUT]
func (*ShopHandler) UpdateShop(c *gin.Context) {
	var shop model.Shop
	err := c.ShouldBindJSON(&shop)
	if err != nil {
		logrus.Error("failed to bind data")
		c.JSON(http.StatusOK, dto.Fail[string]("failed to bind data"))
		return
	}
	// err = service.ShopManager.UpdateShop(&shop)
	err = service.ShopManager.UpdateShopWithCache(&shop)
	if err != nil {
		logrus.Error("failed to update shop")
		c.JSON(http.StatusOK, dto.Fail[string]("failed to update shop"))
		return
	}
	c.JSON(http.StatusOK, dto.Ok[string]())
}

// @Descirption: query the shop info by the type of the shop
// @Router: /shop/of/type [GET]
func (*ShopHandler) QueryShopByType(c *gin.Context) {
	typeIdStr := c.Query("typeId")
	if typeIdStr == "" {
		logrus.Error("typeId str is empty")
		c.JSON(http.StatusOK, dto.Fail[string]("typeId is empty"))
		return
	}

	var currentStr = "1"
	currentStr = c.Query("current")
	if currentStr == "" {
		currentStr = "1"
	}

	typeId, err := strconv.Atoi(typeIdStr)
	if err != nil {
		logrus.Error("typeId Str is not a number")
		c.JSON(http.StatusOK, dto.Fail[string]("typeId str is empty"))
		return
	}

	current, err := strconv.Atoi(currentStr)
	if err != nil {
		logrus.Error("currentStr is not a number")
		c.JSON(http.StatusOK, dto.Fail[string]("currentStr is not a number"))
		return
	}

	xStr := c.Query("x")
	yStr := c.Query("y")
	if xStr == "" {
		xStr = "0"
	}
	if yStr == "" {
		yStr = "0"
	}
	x, err := strconv.ParseFloat(xStr, 64)
	if err != nil {
		logrus.Error("xStr or yStr is not a number")
		c.JSON(http.StatusOK, dto.Fail[string]("xStr or yStr is not a number"))
		return
	}
	y, err := strconv.ParseFloat(yStr, 64)
	if err != nil {
		logrus.Error("yStr is not a number")
		c.JSON(http.StatusOK, dto.Fail[string]("yStr is not a number"))
		return
	}

	shops, err := service.ShopManager.QueryShopByType(typeId, current, x, y)
	if err != nil {
		logrus.Error("not find shop!")
		c.JSON(http.StatusOK, dto.Fail[string]("not find shop!"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData(shops))
}

// @Descirption: query the shop ny name
// @Router: /shop/of/name [GET]
func (*ShopHandler) QueryShopByName(c *gin.Context) {
	name := c.Query("name")
	if name == "" {
		name = "%%"
	}

	currentStr := c.Query("current")
	if currentStr == "" {
		currentStr = "1"
	}

	current, err := strconv.Atoi(currentStr)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("type transform failed"))
		return
	}

	shops, err := service.ShopManager.QueryByName(name, current)
	if err != nil {
		logrus.Error("query shop by name failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("query shop failed!"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData(shops))
}
