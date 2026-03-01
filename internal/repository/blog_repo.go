package repository

import (
	"context"
	"fmt"
	"local-review-go/internal/model"
	"local-review-go/internal/repository/interface"
	"local-review-go/pkg/utils/redisx"
	"strings"
	"time"

	"gorm.io/gorm"
)

type blogRepo struct {
	db *gorm.DB
}

// NewBlogRepo 创建博客 Repository
func NewBlogRepo(db *gorm.DB) interfaces.BlogRepo {
	return &blogRepo{db: db}
}

func (r *blogRepo) Create(ctx context.Context, blog *model.Blog) (int64, error) {
	blog.CreateTime = time.Now()
	blog.UpdateTime = time.Now()
	err := r.db.WithContext(ctx).Table(blog.TableName()).Create(blog).Error
	if err != nil {
		return 0, err
	}
	return blog.Id, nil
}

func (r *blogRepo) ListByUserID(ctx context.Context, userID int64, current int) ([]model.Blog, error) {
	var blog model.Blog
	var blogs []model.Blog
	err := r.db.WithContext(ctx).
		Table(blog.TableName()).
		Where("user_id = ?", userID).
		Offset((current - 1) * redisx.MAXPAGESIZE).
		Limit(redisx.MAXPAGESIZE).
		Find(&blogs).Error
	return blogs, err
}

func (r *blogRepo) ListHots(ctx context.Context, current int) ([]model.Blog, error) {
	var blogs []model.Blog
	err := r.db.WithContext(ctx).
		Table((&model.Blog{}).TableName()).
		Order("liked desc").
		Offset((current - 1) * redisx.MAXPAGESIZE).
		Limit(redisx.MAXPAGESIZE).
		Find(&blogs).Error
	return blogs, err
}

func (r *blogRepo) GetByID(ctx context.Context, id int64) (*model.Blog, error) {
	var blog model.Blog
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&blog).Error
	if err != nil {
		return nil, err
	}
	return &blog, nil
}

func (r *blogRepo) ListByIDs(ctx context.Context, ids []int64) ([]model.Blog, error) {
	if len(ids) == 0 {
		return []model.Blog{}, nil
	}
	idStrs := make([]string, len(ids))
	for i, id := range ids {
		idStrs[i] = fmt.Sprintf("%d", id)
	}
	order := fmt.Sprintf("FIELD(id , %s)", strings.Join(idStrs, ","))

	var blogs []model.Blog
	err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Order(order).
		Find(&blogs).Error
	return blogs, err
}

// ListByShopID 按店铺 ID 查询探店笔记（按点赞数排序，取前 limit 条）
func (r *blogRepo) ListByShopID(ctx context.Context, shopID int64, limit int) ([]model.Blog, error) {
	if limit <= 0 {
		limit = 5
	}
	var blogs []model.Blog
	err := r.db.WithContext(ctx).
		Table((&model.Blog{}).TableName()).
		Where("shop_id = ?", shopID).
		Order("liked desc").
		Limit(limit).
		Find(&blogs).Error
	return blogs, err
}

func (r *blogRepo) IncrLike(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).
		Table((&model.Blog{}).TableName()).
		Where("id = ?", id).
		Update("liked", gorm.Expr("liked + ?", 1)).Error
}

func (r *blogRepo) DecrLike(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).
		Table((&model.Blog{}).TableName()).
		Where("id = ?", id).
		Update("liked", gorm.Expr("liked - ?", 1)).Error
}
