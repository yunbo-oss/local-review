package logic

import (
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"local-review-go/src/utils"

	"github.com/google/uuid"
)

type UploadLogic interface {
	SaveBlogImage(file *multipart.FileHeader) (string, error)
	DeleteBlogImage(name string) error
}

type uploadLogic struct{}

func NewUploadLogic() UploadLogic {
	return &uploadLogic{}
}

func (l *uploadLogic) SaveBlogImage(file *multipart.FileHeader) (string, error) {
	if file == nil {
		return "", errors.New("file is nil")
	}

	fileName := createNewFileName(file.Filename)
	// fileName 形如 /blogs/1/2/xxx.jpg，Join 时需去掉前导 /
	relPath := strings.TrimPrefix(fileName, "/")
	destPath := filepath.Clean(filepath.Join(utils.UPLOADPATH, relPath))

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", fmt.Errorf("create dir failed: %w", err)
	}

	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("open upload file failed: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("create dest file failed: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("write file failed: %w", err)
	}

	return fileName, nil
}

func (l *uploadLogic) DeleteBlogImage(name string) error {
	if name == "" {
		return errors.New("filename is empty")
	}
	// 前端传入 /imgs/blogs/1/2/xxx.jpg，需去掉 /imgs 前缀
	relPath := strings.TrimPrefix(strings.TrimPrefix(name, "/imgs"), "/")
	if relPath == "" {
		return errors.New("invalid filename")
	}
	if isDir(relPath) {
		return errors.New("invalid filename")
	}

	destPath := filepath.Clean(filepath.Join(utils.UPLOADPATH, relPath))
	if err := os.Remove(destPath); err != nil {
		return fmt.Errorf("remove file failed: %w", err)
	}
	return nil
}

func createNewFileName(originName string) string {
	suffix := filepath.Ext(originName)
	name := uuid.New().String()
	h := fnv.New32a()
	h.Write([]byte(name))
	hash := h.Sum32()
	d1 := hash & 0xF
	d2 := (hash >> 4) & 0xF
	dirName := filepath.Join("blogs", fmt.Sprintf("%v", d1), fmt.Sprintf("%v", d2))
	// 返回相对路径如 /blogs/1/2/xxx.jpg，前端会拼接为 /imgs/blogs/1/2/xxx.jpg
	return filepath.ToSlash(filepath.Join("/", dirName, fmt.Sprintf("%s%s", name, suffix)))
}

func isDir(pathname string) bool {
	info, err := os.Stat(filepath.Clean(filepath.Join(utils.UPLOADPATH, pathname)))
	if err != nil {
		return false
	}
	return info.IsDir()
}
