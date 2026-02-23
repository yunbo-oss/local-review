package handler

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"local-review-go/src/dto"
	"local-review-go/src/middleware"
	"local-review-go/src/service"
	"net/http"
	"strconv"
	"time"
)

type UserHandler struct {
}

var userHandler *UserHandler

// @Description: send the phone code
// @Router: /user/code [POST]
func (*UserHandler) SendCode(c *gin.Context) {
	phoneStr := c.Query("phone")
	if phoneStr == "" {
		logrus.Warn("phone is empty")
		c.JSON(http.StatusOK, dto.Fail[string]("phone is empty"))
		return
	}
	err := service.UserManager.SaveCode(phoneStr)
	if err != nil {
		logrus.Warn("phone is not valid")
		c.JSON(http.StatusOK, dto.Fail[string]("phone is not valid"))
		return
	}
	c.JSON(http.StatusOK, dto.Ok[string]())
}

// @Description: user login in
// @Router: /user/login  [POST]
func (*UserHandler) Login(c *gin.Context) {
	// TODO 实现登录功能
	var loginInfo dto.LoginFormDto
	err := c.ShouldBindJSON(&loginInfo)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("bind json failed!"))
		return
	}
	token, err := service.UserManager.Login(&loginInfo)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("get token failed!"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData(token))
}

// @Description: user layout
// @Router: /user/logout [POST]
func (*UserHandler) Logout(c *gin.Context) {
	// TODO 实现注销功能
	c.JSON(http.StatusOK, dto.Fail[string]("this function is not finished"))
}

// @Description: get the info of me
// @Router: /user/me [GET]
func (*UserHandler) Me(c *gin.Context) {
	// TODO 获取当前登录的用户
	userDTO, err := middleware.GetUserInfo(c)
	if err != nil {
		logrus.Error("the user info is empty!")
		c.JSON(http.StatusOK, dto.Fail[string]("the user info is empty!"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData(userDTO))
}

// @Description: get the info of user by user Id
// @Router /user/info/:id [GET]
func (*UserHandler) Info(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		logrus.Error("id str is empty!")
		c.JSON(http.StatusOK, dto.Fail[string]("id str is empty!"))
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logrus.Error("parse int failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("id parse failed!"))
		return
	}
	userInfo, err := service.UserInfoManager.GetUserInfoById(id)
	if err != nil {
		logrus.Error("get user info failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("get user info failed!"))
		return
	}
	userInfo.CreateTime = time.Time{}
	userInfo.UpdateTime = time.Time{}
	c.JSON(http.StatusOK, dto.OkWithData(userInfo))
}

// @Description: sign
// @Router /user/sign [GET]
func (*UserHandler) sign(c *gin.Context) {
	userInfo, err := middleware.GetUserInfo(c)
	if err != nil {
		logrus.Error("get user info failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("get user info failed!"))
	}
	err = service.UserManager.Sign(userInfo.Id)
	if err != nil {
		logrus.Error("sign user failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("sign user failed!"))
	}
	c.JSON(http.StatusOK, dto.OkWithData[string]("签到成功!"))
}

// @Description: 获取当月连续签到的次数
// @Router /user/sign/count
func (*UserHandler) SignCount(c *gin.Context) {
	userInfo, err := middleware.GetUserInfo(c)
	if err != nil {
		logrus.Error("get user info failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("get user info failed!"))
	}
	count, err := service.UserManager.GetSignCount(userInfo.Id)
	if err != nil {
		logrus.Error("get user sign count failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("get user sign count failed!"))
	}
	c.JSON(http.StatusOK, dto.OkWithData[string](fmt.Sprintf("get user sign count is %d", count)))

}
