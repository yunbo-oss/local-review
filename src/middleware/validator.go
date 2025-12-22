package middleware

import (
	"local-review-go/src/dto"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New()
	// 注册自定义标签名函数，将 json tag 作为字段名
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
}

// ValidateRequest 验证请求参数的中间件
// 使用方法：在 Handler 中使用 c.ShouldBindJSON(&req) 后调用此中间件
func ValidateRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取绑定的对象（需要在 Handler 中先调用 ShouldBindJSON）
		obj := c.MustGet("bind_object")
		if obj == nil {
			c.Next()
			return
		}

		if err := validate.Struct(obj); err != nil {
			var errors []string
			for _, err := range err.(validator.ValidationErrors) {
				errors = append(errors, getErrorMessage(err))
			}
			c.JSON(http.StatusBadRequest, dto.Fail[string](strings.Join(errors, "; ")))
			c.Abort()
			return
		}
		c.Next()
	}
}

// ValidateStruct 验证结构体的辅助函数
func ValidateStruct(obj interface{}) error {
	return validate.Struct(obj)
}

// getErrorMessage 根据验证错误生成友好的错误消息
func getErrorMessage(err validator.FieldError) string {
	field := err.Field()
	switch err.Tag() {
	case "required":
		return field + " 是必填字段"
	case "email":
		return field + " 必须是有效的邮箱地址"
	case "min":
		return field + " 的最小长度为 " + err.Param()
	case "max":
		return field + " 的最大长度为 " + err.Param()
	case "gte":
		return field + " 必须大于或等于 " + err.Param()
	case "lte":
		return field + " 必须小于或等于 " + err.Param()
	case "gt":
		return field + " 必须大于 " + err.Param()
	case "lt":
		return field + " 必须小于 " + err.Param()
	case "oneof":
		return field + " 必须是以下值之一: " + err.Param()
	default:
		return field + " 验证失败: " + err.Tag()
	}
}
