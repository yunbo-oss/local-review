package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"local-review-go/src/dto"
	"local-review-go/src/middleware"
	"local-review-go/src/service"
	"net/http"
	"strconv"
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
		c.JSON(http.StatusOK, dto.Fail[string]("the id is empty!"))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("type transform failed!"))
		return
	}

	isFollowStr := c.Param("isFollow")
	if isFollowStr == "" {
		logrus.Error("the follow is empty!")
		c.JSON(http.StatusOK, dto.Fail[string]("the follow str is empty!"))
		return
	}

	isFollow, err := strconv.ParseBool(isFollowStr)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("type transform failed!"))
		return
	}

	user, err := middleware.GetUserInfo(c)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("failed to get user id"))
		return
	}

	userId := user.Id

	err = service.FollowManager.Follow(id, userId, isFollow)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("failed to follow!"))
		return
	}
	c.JSON(http.StatusOK, dto.Ok[string]())
}

// @Description: get the common follow
// @Router: /follow/common/:id [GET]
func (*FollowHandler) FollowCommons(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		logrus.Error("the id is empty")
		c.JSON(http.StatusOK, dto.Fail[string]("the id is empty!"))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("the type transform failed!"))
		return
	}

	user, err := middleware.GetUserInfo(c)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("get user info failed!"))
		return
	}

	userId := user.Id

	users, err := service.FollowManager.FollowCommons(id, userId)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("find common filed!"))
		return
	}

	c.JSON(http.StatusOK, dto.OkWithData(users))
}

// @Description: judge if or not follow
// @Router: /follow/or/not/:id [GET]
func (*FollowHandler) IsFollow(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		logrus.Error("the id is empty")
		c.JSON(http.StatusOK, dto.Fail[string]("the id is empty!"))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("the type transform failed!"))
		return
	}

	user, err := middleware.GetUserInfo(c)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("get user info failed!"))
		return
	}

	userId := user.Id

	result, err := service.FollowManager.IsFollow(id, userId)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("failed to follow"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData(result))
}
