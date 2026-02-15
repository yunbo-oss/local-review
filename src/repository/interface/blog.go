package interfaces

import (
	"context"
	"local-review-go/src/model"
)

// BlogRepo 博客数据访问接口
type BlogRepo interface {
	Create(ctx context.Context, blog *model.Blog) (int64, error)
	ListByUserID(ctx context.Context, userID int64, current int) ([]model.Blog, error)
	ListHots(ctx context.Context, current int) ([]model.Blog, error)
	GetByID(ctx context.Context, id int64) (*model.Blog, error)
	ListByIDs(ctx context.Context, ids []int64) ([]model.Blog, error)
	IncrLike(ctx context.Context, id int64) error
	DecrLike(ctx context.Context, id int64) error
}
