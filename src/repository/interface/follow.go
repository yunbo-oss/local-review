package interfaces

import (
	"context"
	"local-review-go/src/model"
)

// FollowRepo 关注数据访问接口
type FollowRepo interface {
	Create(ctx context.Context, follow *model.Follow) error
	Delete(ctx context.Context, userID, followUserID int64) error
	Exists(ctx context.Context, userID, followUserID int64) (bool, error)
	ListByFollowUserID(ctx context.Context, followUserID int64) ([]model.Follow, error)
}
