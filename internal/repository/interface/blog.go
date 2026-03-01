package interfaces

import (
	"context"
	"local-review-go/internal/model"
)

// BlogRepo 博客数据访问接口
type BlogRepo interface {
	Create(ctx context.Context, blog *model.Blog) (int64, error)
	ListByUserID(ctx context.Context, userID int64, current int) ([]model.Blog, error)
	ListHots(ctx context.Context, current int) ([]model.Blog, error)
	GetByID(ctx context.Context, id int64) (*model.Blog, error)
	ListByIDs(ctx context.Context, ids []int64) ([]model.Blog, error)
	// ListByShopID 按店铺 ID 查询探店笔记（用户点评），limit 为条数上限
	ListByShopID(ctx context.Context, shopID int64, limit int) ([]model.Blog, error)
	IncrLike(ctx context.Context, id int64) error
	DecrLike(ctx context.Context, id int64) error
}
