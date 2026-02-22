package interfaces

import (
	"context"
	"local-review-go/internal/model"
)

// UserRepo 用户数据访问接口
type UserRepo interface {
	GetByID(ctx context.Context, id int64) (*model.User, error)
	GetByPhone(ctx context.Context, phone string) (*model.User, error)
	Create(ctx context.Context, user *model.User) error
	GetByIDs(ctx context.Context, ids []int64) ([]model.User, error)
}
