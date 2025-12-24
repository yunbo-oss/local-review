package handler

import (
	"local-review-go/src/httpx"
	"local-review-go/src/middleware"
	"local-review-go/src/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type FollowHandler struct {
}

var followHanlder *FollowHandler

// @Description: follow and not follow
// @Router: /follow/:id/:isFollow [PUT]
func (*FollowHandler) Follow(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		logrus.Error("the id is empty")
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("id is required"))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("invalid parameter"))
		return
	}

	isFollowStr := c.Param("isFollow")
	if isFollowStr == "" {
		logrus.Error("the follow is empty!")
		c.JSON(http.StatusOK, httpx.Fail[string]("the follow str is empty!"))
		return
	}

	isFollow, err := strconv.ParseBool(isFollowStr)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("invalid parameter"))
		return
	}

	user, err := middleware.GetUserInfo(c)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusUnauthorized, httpx.Fail[string]("unauthorized"))
		return
	}

	userId := user.Id

	ctx := c.Request.Context()
	err = service.FollowManager.Follow(ctx, id, userId, isFollow)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusInternalServerError, httpx.Fail[string]("failed to follow!"))
		return
	}
	c.JSON(http.StatusOK, httpx.Ok[string]())
}

// @Description: get the common follow
// @Router: /follow/common/:id [GET]
func (*FollowHandler) FollowCommons(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		logrus.Error("the id is empty")
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("id is required"))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, httpx.Fail[string]("the type transform failed!"))
		return
	}

	user, err := middleware.GetUserInfo(c)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, httpx.Fail[string]("get user info failed!"))
		return
	}

	userId := user.Id

	ctx := c.Request.Context()
	users, err := service.FollowManager.FollowCommons(ctx, id, userId)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, httpx.Fail[string]("find common filed!"))
		return
	}

	c.JSON(http.StatusOK, httpx.OkWithData(users))
}

// @Description: judge if or not follow
// @Router: /follow/or/not/:id [GET]
func (*FollowHandler) IsFollow(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		logrus.Error("the id is empty")
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("id is required"))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, httpx.Fail[string]("the type transform failed!"))
		return
	}

	user, err := middleware.GetUserInfo(c)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, httpx.Fail[string]("get user info failed!"))
		return
	}

	userId := user.Id

	ctx := c.Request.Context()
	result, err := service.FollowManager.IsFollow(ctx, id, userId)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, httpx.Fail[string]("failed to follow"))
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData(result))
}
