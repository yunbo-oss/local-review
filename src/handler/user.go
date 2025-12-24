package handler

import (
	"fmt"
	"local-review-go/src/httpx"
	"local-review-go/src/middleware"
	"local-review-go/src/service"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type UserHandler struct {
}

var userHandler *UserHandler

// LoginRequest 登录请求结构体
type LoginRequest struct {
	Phone    string `json:"phone" binding:"required,min=11,max=11"`
	Code     string `json:"code" binding:"required,len=6"`
	Password string `json:"password" binding:"omitempty,min=6,max=20"`
}

// @Description: send the phone code
// @Router: /user/code [POST]
func (*UserHandler) SendCode(c *gin.Context) {
	phoneStr := c.Query("phone")
	if phoneStr == "" {
		logrus.Warn("phone is empty")
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("phone is required"))
		return
	}
	ctx := c.Request.Context()
	err := service.UserManager.SaveCode(ctx, phoneStr)
	if err != nil {
		logrus.Warn("phone is not valid")
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("phone is not valid"))
		return
	}
	c.JSON(http.StatusOK, httpx.Ok[string]())
}

// @Description: user login in
// @Router: /user/login  [POST]
func (*UserHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return // 错误已处理
	}

	ctx := c.Request.Context()
	token, err := service.UserManager.Login(ctx, req.Phone, req.Code)
	if err != nil {
		logrus.Error(err.Error())
		// 根据错误类型判断状态码
		if err.Error() == "a wrong verify code!" {
			c.JSON(http.StatusUnauthorized, httpx.Fail[string]("verify code is incorrect"))
		} else {
			c.JSON(http.StatusInternalServerError, httpx.Fail[string]("login failed!"))
		}
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData(token))
}

// @Description: user layout
// @Router: /user/logout [POST]
func (*UserHandler) Logout(c *gin.Context) {
	// TODO 实现注销功能
	c.JSON(http.StatusOK, httpx.Fail[string]("this function is not finished"))
}

// @Description: get the info of me
// @Router: /user/me [GET]
func (*UserHandler) Me(c *gin.Context) {
	// TODO 获取当前登录的用户
	userDTO, err := middleware.GetUserInfo(c)
	if err != nil {
		logrus.Error("the user info is empty!")
		c.JSON(http.StatusUnauthorized, httpx.Fail[string]("unauthorized"))
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData(userDTO))
}

// @Description: get the info of user by user Id
// @Router /user/info/:id [GET]
func (*UserHandler) Info(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		logrus.Error("id str is empty!")
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("id is required"))
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logrus.Error("parse int failed!")
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("id is invalid"))
		return
	}
	userInfo, err := service.UserInfoManager.GetUserInfoById(id)
	if err != nil {
		logrus.Error("get user info failed!")
		c.JSON(http.StatusNotFound, httpx.Fail[string]("user not found"))
		return
	}
	userInfo.CreateTime = time.Time{}
	userInfo.UpdateTime = time.Time{}
	c.JSON(http.StatusOK, httpx.OkWithData(userInfo))
}

// @Description: sign
// @Router /user/sign [GET]
func (*UserHandler) sign(c *gin.Context) {
	userInfo, err := middleware.GetUserInfo(c)
	if err != nil {
		logrus.Error("get user info failed!")
		c.JSON(http.StatusUnauthorized, httpx.Fail[string]("unauthorized"))
		return
	}
	ctx := c.Request.Context()
	err = service.UserManager.Sign(ctx, userInfo.Id)
	if err != nil {
		logrus.Error("sign user failed!")
		c.JSON(http.StatusInternalServerError, httpx.Fail[string]("sign failed!"))
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData[string]("签到成功!"))
}

// @Description: 获取当月连续签到的次数
// @Router /user/sign/count
func (*UserHandler) SignCount(c *gin.Context) {
	userInfo, err := middleware.GetUserInfo(c)
	if err != nil {
		logrus.Error("get user info failed!")
		c.JSON(http.StatusUnauthorized, httpx.Fail[string]("unauthorized"))
		return
	}
	ctx := c.Request.Context()
	count, err := service.UserManager.GetSignCount(ctx, userInfo.Id)
	if err != nil {
		logrus.Error("get user sign count failed!")
		c.JSON(http.StatusInternalServerError, httpx.Fail[string]("get sign count failed!"))
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData[string](fmt.Sprintf("get user sign count is %d", count)))

}
