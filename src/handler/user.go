package handler

import (
	"fmt"
	"local-review-go/src/dto"
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

// @Description: send the phone code
// @Router: /user/code [POST]
func (*UserHandler) SendCode(c *gin.Context) {
	phoneStr := c.Query("phone")
	if phoneStr == "" {
		logrus.Warn("phone is empty")
		c.JSON(http.StatusBadRequest, dto.Fail[string]("phone is required"))
		return
	}
	err := service.UserManager.SaveCode(phoneStr)
	if err != nil {
		logrus.Warn("phone is not valid")
		c.JSON(http.StatusBadRequest, dto.Fail[string]("phone is not valid"))
		return
	}
	c.JSON(http.StatusOK, dto.Ok[string]())
}

// @Description: user login in
// @Router: /user/login  [POST]
func (*UserHandler) Login(c *gin.Context) {
	// TODO 实现登录功能
	var loginInfo dto.LoginFormDto
	if err := c.ShouldBindJSON(&loginInfo); err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusBadRequest, dto.Fail[string]("bind json failed!"))
		return
	}

	// 使用 validator 验证
	if err := middleware.ValidateStruct(&loginInfo); err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[string](err.Error()))
		return
	}
	token, err := service.UserManager.Login(&loginInfo)
	if err != nil {
		logrus.Error(err.Error())
		// 根据错误类型判断状态码
		if err.Error() == "a wrong verify code!" {
			c.JSON(http.StatusUnauthorized, dto.Fail[string]("verify code is incorrect"))
		} else {
			c.JSON(http.StatusInternalServerError, dto.Fail[string]("login failed!"))
		}
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
		c.JSON(http.StatusUnauthorized, dto.Fail[string]("unauthorized"))
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
		c.JSON(http.StatusBadRequest, dto.Fail[string]("id is required"))
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logrus.Error("parse int failed!")
		c.JSON(http.StatusBadRequest, dto.Fail[string]("id is invalid"))
		return
	}
	userInfo, err := service.UserInfoManager.GetUserInfoById(id)
	if err != nil {
		logrus.Error("get user info failed!")
		c.JSON(http.StatusNotFound, dto.Fail[string]("user not found"))
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
		c.JSON(http.StatusUnauthorized, dto.Fail[string]("unauthorized"))
		return
	}
	err = service.UserManager.Sign(userInfo.Id)
	if err != nil {
		logrus.Error("sign user failed!")
		c.JSON(http.StatusInternalServerError, dto.Fail[string]("sign failed!"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData[string]("签到成功!"))
}

// @Description: 获取当月连续签到的次数
// @Router /user/sign/count
func (*UserHandler) SignCount(c *gin.Context) {
	userInfo, err := middleware.GetUserInfo(c)
	if err != nil {
		logrus.Error("get user info failed!")
		c.JSON(http.StatusUnauthorized, dto.Fail[string]("unauthorized"))
		return
	}
	count, err := service.UserManager.GetSignCount(userInfo.Id)
	if err != nil {
		logrus.Error("get user sign count failed!")
		c.JSON(http.StatusInternalServerError, dto.Fail[string]("get sign count failed!"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData[string](fmt.Sprintf("get user sign count is %d", count)))

}
