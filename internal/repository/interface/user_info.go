package interfaces

import (
	"context"
	"local-review-go/internal/model"
)

// UserInfoRepo 用户详情数据访问接口
type UserInfoRepo interface {
	GetByUserID(ctx context.Context, userID int64) (*model.UserInfo, error)
}
