package handler

import (
	"local-review-go/src/httpx"
	"local-review-go/src/logic"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type UploadHandler struct {
	logic logic.UploadLogic
}

func NewUploadHandler(uploadLogic logic.UploadLogic) *UploadHandler {
	return &UploadHandler{logic: uploadLogic}
}

// @Description: upload the image
// @Router: /upload/blog [POST]
func (h *UploadHandler) UploadImage(c *gin.Context) {
	logrus.Info("beign to upload image")
	file, err := c.FormFile("file")
	if err != nil {
		logrus.Error("upload file failed!")
		c.JSON(http.StatusBadRequest, httpx.Fail[string]("upload file failed!"))
		return
	}
	fileName, err := h.logic.SaveBlogImage(file)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusInternalServerError, httpx.Fail[string]("file upload failed!"))
		return
	}
	c.JSON(http.StatusOK, httpx.OkWithData(fileName))
}

// @Description: delete the uploaed image
// @Router: /upload/blog/delete [GET]
func (h *UploadHandler) DeleteBlogImg(c *gin.Context) {
	fileName := c.Query("name")
	if fileName == "" {
		logrus.Error("fileName is empty")
		c.JSON(http.StatusOK, httpx.Fail[string]("filename is empty"))
		return
	}

	err := h.logic.DeleteBlogImage(fileName)
	if err != nil {
		logrus.Error("remove file failed!")
		c.JSON(http.StatusOK, httpx.Fail[string]("remove file failed!"))
		return
	}
	c.JSON(http.StatusOK, httpx.Ok[string]())
}
