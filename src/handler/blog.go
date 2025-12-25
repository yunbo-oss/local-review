package handler

import (
	"local-review-go/src/httpx"
	"local-review-go/src/logic"
	"local-review-go/src/middleware"
	"local-review-go/src/model"
	"local-review-go/src/utils/redisx"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type BlogHandler struct {
	logic logic.BlogLogic
}

func NewBlogHandler(blogLogic logic.BlogLogic) *BlogHandler {
	return &BlogHandler{logic: blogLogic}
}

// @Description: save the blog
// @Router:  /blog [POST]
func (h *BlogHandler) SaveBlog(c *gin.Context) {
	var blog model.Blog
	err := c.ShouldBindJSON(&blog)
	if err != nil {
		logrus.Error("[Blog handler] bind json failed!")
		c.JSON(http.StatusOK, httpx.Fail[string]("insert failed!"))
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
	id, err := h.logic.SaveBlog(ctx, userId, &blog)
	if err != nil {
		logrus.Error("[Blog handler] insert data into database failed!")
		c.JSON(http.StatusOK, httpx.Fail[string]("insert failed!"))
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData(id))
}

// @Description: modify the number of linked
// @Router:  /blog/like/:id  [PUT]
func (h *BlogHandler) LikeBlog(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		logrus.Error("[Blog Handler] Give a empty string")
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("blog id is required"))
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
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
	err = h.logic.LikeBlog(ctx, id, userId)

	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusInternalServerError, httpx.Fail[string]("like failed!"))
		return
	}
	c.JSON(http.StatusOK, httpx.Ok[string]())
}

// @Description: get user rank of the blog
// @Reouter: /blog/likes/:id  [GET]
func (h *BlogHandler) QueryUserLiked(c *gin.Context) {
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
	ctx := c.Request.Context()
	users, err := h.logic.QueryUserLike(ctx, id)

	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, httpx.Fail[string]("the type transform failed!"))
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData(users))
}

// @Description: query my blog
// @Router: /blog/of/me [GET]
func (h *BlogHandler) QueryMyBlog(c *gin.Context) {
	var current string
	current = c.Query("current")

	if current == "" {
		current = "1"
	}

	currentPage, err := strconv.Atoi(current)
	if err != nil {
		logrus.Error("type transform failed!")
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("invalid parameter"))
		return
	}

	user, err := middleware.GetUserInfo(c)

	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusUnauthorized, httpx.Fail[string]("unauthorized"))
		return
	}

	ctx := c.Request.Context()
	blogs, err := h.logic.QueryMyBlog(ctx, user.Id, currentPage)
	if err != nil {
		logrus.Error("page query failed!")
		c.JSON(http.StatusInternalServerError, httpx.Fail[string]("page query failed!"))
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData[[]model.Blog](blogs))
}

// @Description: query the hot blog
// @Router: /blog/hot [GET]
func (h *BlogHandler) QueryHotBlog(c *gin.Context) {
	var currentStr = "1"
	currentStr = c.Query("current")
	if currentStr == "" {
		currentStr = "1"
	}
	current, err := strconv.Atoi(currentStr)
	if err != nil {
		logrus.Error("transform type failed!")
		c.JSON(http.StatusOK, httpx.Fail[string]("transform type failed!"))
		return
	}
	ctx := c.Request.Context()
	blogs, err := h.logic.QueryHotBlogs(ctx, current)
	if err != nil {
		logrus.Error("query hot blogs failed!")
		c.JSON(http.StatusInternalServerError, httpx.Fail[string]("query hot blogs failed!"))
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData[[]model.Blog](blogs))
}

// @Description: Get Blog by id
// @Router: /blog/:id  [GET]
func (h *BlogHandler) GetBlogById(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		logrus.Error("id str is empty")
		c.JSON(http.StatusOK, httpx.Fail[string]("id str is empty"))
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, httpx.Fail[string]("type transform is failed!"))
		return
	}
	ctx := c.Request.Context()
	blog, err := h.logic.GetBlogById(ctx, id)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, httpx.Fail[string]("get blog by id failed!"))
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData(blog))
}

// @Description: get the blog info of followed people
// @Router: /blog/of/follow [GET]
func (h *BlogHandler) QueryBlogOfFollow(c *gin.Context) {
	lastIdStr := c.Query("lastId")
	lastId, err := strconv.ParseInt(lastIdStr, 10, 64)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("invalid parameter"))
		return
	}

	offsetStr := c.Query("offset")
	if offsetStr == "" {
		offsetStr = "1"
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("invalid parameter"))
		return
	}

	user, err := middleware.GetUserInfo(c)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, httpx.Fail[string]("failed to get user info"))
		return
	}

	userId := user.Id

	ctx := c.Request.Context()
	r, err := h.logic.QueryBlogOfFollow(ctx, lastId, offset, userId, redisx.DEFAULTPAGESIZE)

	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusOK, httpx.Fail[string]("failed to get result"))
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData(r))
}
