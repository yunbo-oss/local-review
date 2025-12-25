package handler

import (
	"fmt"
	"local-review-go/src/httpx"
	"local-review-go/src/logic"
	"local-review-go/src/model"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type ShopHandler struct {
	logic logic.ShopLogic
}

func NewShopHandler(shopLogic logic.ShopLogic) *ShopHandler {
	return &ShopHandler{
		logic: shopLogic,
	}
}

// @Descirption: query shop by id
// @Router: /shop/{id} [GET]
// TODO add cache
func (h *ShopHandler) QueryShopById(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		logrus.Error("id is empty!")
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("id is emtpy!"))
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logrus.Error("transform failed!")
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("transform type failed!"))
		return
	}
	ctx := c.Request.Context()
	shop, err := h.logic.QueryShopByIdWithCacheNull(ctx, id)

	if err != nil {
		logrus.Error("query failed!")
		// 根据错误类型判断状态码
		if err.Error() == "shop not found (blocked by Bloom Filter)" {
			c.JSON(http.StatusNotFound, httpx.Fail[string]("shop not found"))
		} else {
			c.JSON(http.StatusInternalServerError, httpx.Fail[string]("query failed!"))
		}
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData[model.Shop](shop))
}

// @Descirption: save the shop info
// @Router: /shop [POST]
func (h *ShopHandler) SaveShop(c *gin.Context) {
	var shop model.Shop
	err := c.ShouldBindJSON(&shop)
	if err != nil {
		logrus.Error("bind json failed")
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("bind json failed!"))
		return
	}
	ctx := c.Request.Context()
	err = h.logic.SaveShop(ctx, &shop)
	if err != nil {
		logrus.Errorf("save data failed! error: %v", err)
		c.JSON(http.StatusInternalServerError, httpx.Fail[string](fmt.Sprintf("save data failed! error: %v", err)))
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData(shop.Id))
}

// @Descirption: update the shop info
// @Router: /shop [PUT]
func (h *ShopHandler) UpdateShop(c *gin.Context) {
	var shop model.Shop
	err := c.ShouldBindJSON(&shop)
	if err != nil {
		logrus.Error("failed to bind data")
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("failed to bind data"))
		return
	}
	ctx := c.Request.Context()
	err = h.logic.UpdateShopWithCache(ctx, &shop)
	if err != nil {
		logrus.Error("failed to update shop")
		c.JSON(http.StatusInternalServerError, httpx.Fail[string]("failed to update shop"))
		return
	}
	c.JSON(http.StatusOK, httpx.Ok[string]())
}

// @Descirption: query the shop info by the type of the shop
// @Router: /shop/of/type [GET]
func (h *ShopHandler) QueryShopByType(c *gin.Context) {
	typeIdStr := c.Query("typeId")
	if typeIdStr == "" {
		logrus.Error("typeId str is empty")
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("typeId is required"))
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
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("typeId is invalid"))
		return
	}

	current, err := strconv.Atoi(currentStr)
	if err != nil {
		logrus.Error("currentStr is not a number")
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("current is invalid"))
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
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("x or y coordinate is invalid"))
		return
	}
	y, err := strconv.ParseFloat(yStr, 64)
	if err != nil {
		logrus.Error("yStr is not a number")
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("y coordinate is invalid"))
		return
	}

	ctx := c.Request.Context()
	shops, err := h.logic.QueryShopByType(ctx, typeId, current, x, y)
	if err != nil {
		logrus.Error("not find shop!")
		c.JSON(http.StatusInternalServerError, httpx.Fail[string]("query shop failed!"))
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData(shops))
}

// @Descirption: query the shop ny name
// @Router: /shop/of/name [GET]
func (h *ShopHandler) QueryShopByName(c *gin.Context) {
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
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("current is invalid"))
		return
	}

	ctx := c.Request.Context()
	shops, err := h.logic.QueryByName(ctx, name, current)
	if err != nil {
		logrus.Error("query shop by name failed!")
		c.JSON(http.StatusInternalServerError, httpx.Fail[string]("query shop failed!"))
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData(shops))
}
