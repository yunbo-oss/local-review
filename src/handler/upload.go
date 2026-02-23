package handler

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"hash/fnv"
	"local-review-go/src/dto"
	"local-review-go/src/utils"
	"net/http"
	"os"
	"path/filepath"
)

type UploadHandler struct {
}

var uploadHandler *UploadHandler

// @Description: upload the image
// @Router: /upload/blog [POST]
func (*UploadHandler) UploadImage(c *gin.Context) {
	logrus.Info("beign to upload image")
	file, err := c.FormFile("file")
	if err != nil {
		logrus.Error("upload file failed!")
		c.JSON(http.StatusBadRequest, dto.Fail[string]("upload file failed!"))
		return
	}
	originName := file.Filename
	logrus.Info(originName)
	fileName := createNewFileName(originName)
	logrus.Info(fileName)
	logrus.Info(utils.UPLOADPATH + fileName)
	err = c.SaveUploadedFile(file, utils.UPLOADPATH+fileName)
	if err != nil {
		logrus.Error(err.Error())
		c.JSON(http.StatusInternalServerError, dto.Fail[string]("file upload failed!"))
		return
	}
	c.JSON(http.StatusOK, dto.OkWithData(fileName))
}

// @Description: delete the uploaed image
// @Router: /upload/blog/delete [GET]
func (*UploadHandler) DeleteBlogImg(c *gin.Context) {
	fileName := c.Query("name")
	if fileName == "" {
		logrus.Error("fileName is empty")
		c.JSON(http.StatusOK, dto.Fail[string]("filename is empty"))
		return
	}

	if isDir(fileName) {
		logrus.Error("error filename!")
		c.JSON(http.StatusOK, dto.Fail[string]("error filename!"))
		return
	}
	filePath := utils.UPLOADPATH + fileName
	err := os.Remove(filePath)
	if err != nil {
		logrus.Error("remove file failed!")
		c.JSON(http.StatusOK, dto.Fail[string]("remove file failed!"))
		return
	}
	c.JSON(http.StatusOK, dto.Ok[string]())
}

func createNewFileName(originName string) string {
	suffix := filepath.Ext(originName)
	name := uuid.New().String()
	h := fnv.New32a()
	h.Write([]byte(name))
	hash := h.Sum32()
	d1 := hash & 0xF
	d2 := (hash >> 4) & 0xF
	dirName := utils.UPLOADPATH + fmt.Sprintf("/blogs/%v/%v", d1, d2)
	if !dirExists(dirName) {
		os.Mkdir(dirName, 0755)
	}
	return fmt.Sprintf("/blogs/%v/%v/%v.%v", d1, d2, name, suffix)
}

func dirExists(dirname string) bool {
	info, err := os.Stat(dirname)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

func isDir(pathname string) bool {
	info, err := os.Stat(pathname)
	if err != nil {
		return false
	}
	return info.IsDir()
}
