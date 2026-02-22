package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Result 通用响应结构
type Result[T any] struct {
	Success  bool   `json:"success"`
	ErrorMsg string `json:"errorMsg"`
	Data     T      `json:"data"`
	Total    int64  `json:"total"`
}

func Ok[T any]() Result[T] {
	var zeroValue T
	return Result[T]{
		Success:  true,
		ErrorMsg: "",
		Data:     zeroValue,
		Total:    0,
	}
}

func OkWithData[T any](data T) Result[T] {
	return Result[T]{
		Success:  true,
		ErrorMsg: "",
		Data:     data,
		Total:    0,
	}
}

func OkWithList[T any](data []T, total int64) Result[[]T] {
	return Result[[]T]{
		Success:  true,
		ErrorMsg: "",
		Data:     data,
		Total:    total,
	}
}

func Fail[T any](errorMsg string) Result[T] {
	var zeroValue T
	return Result[T]{
		Success:  false,
		ErrorMsg: errorMsg,
		Data:     zeroValue,
		Total:    0,
	}
}

// ScrollResult 滚动分页结果
type ScrollResult[T any] struct {
	Data    []T   `json:"list"`
	MinTime int64 `json:"minTime"`
	Offset  int   `json:"offset"`
}

// BindJSON 统一的 JSON 绑定和错误处理辅助函数
func BindJSON(c *gin.Context, obj interface{}) error {
	if err := c.ShouldBindJSON(obj); err != nil {
		c.JSON(http.StatusBadRequest, Fail[string](err.Error()))
		return err
	}
	return nil
}
