package agent

import "fmt"

// 稳定错误类别（SSE/日志用公共文案；内部可包装 detail）
const (
	ErrGroundingNoCitation     = "grounding_no_citation"
	ErrGroundingUnknownShop    = "grounding_unknown_shop"
	ErrGroundingFactConflict   = "grounding_fact_conflict"
	ErrGroundingEmptyBlogsWash = "grounding_empty_blogs_wash"
	ErrToolInvalidArgs         = "tool_invalid_args"
	ErrToolNotAllowed          = "tool_not_allowed"
	ErrToolDuplicate           = "duplicate_tool_call"
	ErrRateLimit               = "rate_limit"
	ErrMaxToolCalls            = "max_tool_calls"
	ErrMaxSteps                = "max_steps"
)

// PublicError 对用户可见的错误
type PublicError struct {
	Code    string
	Message string
}

func (e *PublicError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

func NewPublicError(code, message string) *PublicError {
	return &PublicError{Code: code, Message: message}
}

func WrapPublic(code string, err error) *PublicError {
	if err == nil {
		return NewPublicError(code, "")
	}
	return NewPublicError(code, fmt.Sprintf("%s: %v", code, err))
}

// PublicMessage 将内部错误映射为 SSE 公共文案
func PublicMessage(err error) string {
	if err == nil {
		return "服务异常，请稍后重试"
	}
	if pe, ok := err.(*PublicError); ok {
		switch pe.Code {
		case ErrGroundingNoCitation, ErrGroundingUnknownShop, ErrGroundingFactConflict, ErrGroundingEmptyBlogsWash:
			return "回答未通过有据可查校验，请重试"
		case ErrRateLimit:
			return "请求过于频繁，请稍后再试"
		case ErrMaxToolCalls, ErrMaxSteps:
			return "本次推荐步骤过多，请简化问题后重试"
		case ErrToolNotAllowed:
			return "无法访问该店铺信息"
		default:
			if pe.Message != "" {
				return pe.Message
			}
		}
	}
	msg := err.Error()
	if containsAny(msg, "groundedness", "grounding_") {
		return "回答未通过有据可查校验，请重试"
	}
	return "推荐失败，请稍后重试"
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > 0 && (s == sub || len(s) >= len(sub) && indexOf(s, sub) >= 0) {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
