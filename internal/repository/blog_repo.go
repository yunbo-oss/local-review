package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
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

type blogPageCursor struct {
	Sort       string `json:"sort"`
	Liked      int    `json:"liked,omitempty"`
	CreateTime int64  `json:"create_time,omitempty"`
	ID         int64  `json:"id"`
}

func (r *blogRepo) ListByShopIDPage(ctx context.Context, request interfaces.BlogPageRequest) (interfaces.BlogPageResult, error) {
	if request.ShopID <= 0 {
		return interfaces.BlogPageResult{}, fmt.Errorf("shop_id must be > 0")
	}
	if request.Limit <= 0 {
		request.Limit = 5
	}
	if request.Limit > 10 {
		return interfaces.BlogPageResult{}, fmt.Errorf("limit must be <= 10")
	}
	sortMode := strings.ToLower(strings.TrimSpace(request.Sort))
	if sortMode == "" {
		sortMode = "liked"
	}
	if sortMode != "liked" && sortMode != "recent" {
		return interfaces.BlogPageResult{}, fmt.Errorf("unsupported review sort %q", request.Sort)
	}
	cursor, err := decodeBlogPageCursor(request.Cursor, sortMode)
	if err != nil {
		return interfaces.BlogPageResult{}, err
	}

	query := r.db.WithContext(ctx).
		Table((&model.Blog{}).TableName()).
		Where("shop_id = ?", request.ShopID)
	if !request.FreshAfter.IsZero() {
		query = query.Where("create_time >= ?", request.FreshAfter)
	}
	if cursor != nil {
		if sortMode == "recent" {
			at := time.UnixMilli(cursor.CreateTime)
			query = query.Where("(create_time < ?) OR (create_time = ? AND id < ?)", at, at, cursor.ID)
		} else {
			query = query.Where("(liked < ?) OR (liked = ? AND id < ?)", cursor.Liked, cursor.Liked, cursor.ID)
		}
	}
	if sortMode == "recent" {
		query = query.Order("create_time DESC").Order("id DESC")
	} else {
		query = query.Order("liked DESC").Order("id DESC")
	}
	var blogs []model.Blog
	if err := query.Limit(request.Limit).Find(&blogs).Error; err != nil {
		return interfaces.BlogPageResult{}, err
	}
	result := interfaces.BlogPageResult{Blogs: blogs}
	if len(blogs) == request.Limit {
		last := blogs[len(blogs)-1]
		result.NextCursor = encodeBlogPageCursor(blogPageCursor{
			Sort: sortMode, Liked: last.Liked, CreateTime: last.CreateTime.UnixMilli(), ID: last.Id,
		})
	}
	return result, nil
}

func encodeBlogPageCursor(cursor blogPageCursor) string {
	raw, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeBlogPageCursor(raw, expectedSort string) (*blogPageCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid review cursor: %w", err)
	}
	var cursor blogPageCursor
	dec := json.NewDecoder(strings.NewReader(string(decoded)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cursor); err != nil || cursor.ID <= 0 || cursor.Sort != expectedSort {
		return nil, fmt.Errorf("invalid review cursor")
	}
	return &cursor, nil
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
