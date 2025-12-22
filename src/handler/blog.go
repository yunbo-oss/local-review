package handler

import (
	"local-review-go/src/dto"
	"local-review-go/src/middleware"
	"local-review-go/src/model"
	"local-review-go/src/service"
	"local-review-go/src/utils"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type BlogHandler struct {
}

var blogHandler *BlogHandler

// @Description: save the blog
// @Router:  /blog [POST]
func (*BlogHandler) SaveBlog(c *gin.Context) {
	var blog model.Blog
	err := c.ShouldBindJSON(&blog)
	if err != nil {
		logrus.Error("[Blog handler] bind json failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("insert failed!"))
		return
	}

	user, err := middleware.GetUserInfo(c)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusUnauthorized, dto.Fail[string]("unauthorized"))
		return
	}
	userId := user.Id

	id, err := service.BlogManager.SaveBlog(userId, &blog)
	if err != nil {
		logrus.Error("[Blog handler] insert data into database failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("insert failed!"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData(id))
}

// @Description: modify the number of linked
// @Router:  /blog/like/:id  [PUT]
func (*BlogHandler) LikeBlog(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		logrus.Error("[Blog Handler] Give a empty string")
		c.JSON(http.StatusBadRequest, dto.Fail[string]("blog id is required"))
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusBadRequest, dto.Fail[string]("invalid parameter"))
		return
	}

	user, err := middleware.GetUserInfo(c)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusUnauthorized, dto.Fail[string]("unauthorized"))
		return
	}

	userId := user.Id

	err = service.BlogManager.LikeBlog(id, userId)

	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusInternalServerError, dto.Fail[string]("like failed!"))
		return
	}
	c.JSON(http.StatusOK, dto.Ok[string]())
}

// @Description: get user rank of the blog
// @Reouter: /blog/likes/:id  [GET]
func (*BlogHandler) QueryUserLiked(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		logrus.Error("the id is empty")
		c.JSON(http.StatusBadRequest, dto.Fail[string]("id is required"))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusBadRequest, dto.Fail[string]("invalid parameter"))
		return
	}
	users, err := service.BlogManager.QueryUserLike(id)

	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("the type transform failed!"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData(users))
}

// @Description: query my blog
// @Router: /blog/of/me [GET]
func (*BlogHandler) QueryMyBlog(c *gin.Context) {
	var current string
	current = c.Query("current")

	if current == "" {
		current = "1"
	}

	currentPage, err := strconv.Atoi(current)
	if err != nil {
		logrus.Error("type transform failed!")
		c.JSON(http.StatusBadRequest, dto.Fail[string]("invalid parameter"))
		return
	}

	user, err := middleware.GetUserInfo(c)

	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusUnauthorized, dto.Fail[string]("unauthorized"))
		return
	}

	blogs, err := service.BlogManager.QueryMyBlog(user.Id, currentPage)
	if err != nil {
		logrus.Error("page query failed!")
		c.JSON(http.StatusInternalServerError, dto.Fail[string]("page query failed!"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData[[]model.Blog](blogs))
}

// @Description: query the hot blog
// @Router: /blog/hot [GET]
func (*BlogHandler) QueryHotBlog(c *gin.Context) {
	var currentStr = "1"
	currentStr = c.Query("current")
	if currentStr == "" {
		currentStr = "1"
	}
	current, err := strconv.Atoi(currentStr)
	if err != nil {
		logrus.Error("transform type failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("transform type failed!"))
		return
	}
	blogs, err := service.BlogManager.QueryHotBlogs(current)
	if err != nil {
		logrus.Error("query hot blogs failed!")
		c.JSON(http.StatusInternalServerError, dto.Fail[string]("query hot blogs failed!"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData[[]model.Blog](blogs))
}

// @Description: Get Blog by id
// @Router: /blog/:id  [GET]
func (*BlogHandler) GetBlogById(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		logrus.Error("id str is empty")
		c.JSON(http.StatusOK, dto.Fail[string]("id str is empty"))
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("type transform is failed!"))
		return
	}
	blog, err := service.BlogManager.GetBlogById(id)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("get blog by id failed!"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData(blog))
}

// @Description: get the blog info of followed people
// @Router: /blog/of/follow [GET]
func (*BlogHandler) QueryBlogOfFollow(c *gin.Context) {
	lastIdStr := c.Query("lastId")
	lastId, err := strconv.ParseInt(lastIdStr, 10, 64)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusBadRequest, dto.Fail[string]("invalid parameter"))
		return
	}

	offsetStr := c.Query("offset")
	if offsetStr == "" {
		offsetStr = "1"
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusBadRequest, dto.Fail[string]("invalid parameter"))
		return
	}

	user, err := middleware.GetUserInfo(c)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("failed to get user info"))
		return
	}

	userId := user.Id

	r, err := service.BlogManager.QueryBlogOfFollow(lastId, offset, userId, utils.DEFAULTPAGESIZE)

	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, dto.Fail[string]("failed to get result"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData(r))
}
