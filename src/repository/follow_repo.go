package repository

import (
	"context"
	"local-review-go/src/model"
	"local-review-go/src/repository/interface"
	"time"

	"gorm.io/gorm"
)

type followRepo struct {
	db *gorm.DB
}

// NewFollowRepo 创建关注 Repository
func NewFollowRepo(db *gorm.DB) interfaces.FollowRepo {
	return &followRepo{db: db}
}

func (r *followRepo) Create(ctx context.Context, follow *model.Follow) error {
	if follow.CreateTime.IsZero() {
		follow.CreateTime = time.Now()
	}
	return r.db.WithContext(ctx).Table(follow.TableName()).Create(follow).Error
}

func (r *followRepo) Delete(ctx context.Context, userID, followUserID int64) error {
	return r.db.WithContext(ctx).
		Table((&model.Follow{}).TableName()).
		Where("user_id = ? AND follow_user_id = ?", userID, followUserID).
		Delete(&model.Follow{}).Error
}

func (r *followRepo) Exists(ctx context.Context, userID, followUserID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table((&model.Follow{}).TableName()).
		Where("user_id = ? AND follow_user_id = ?", userID, followUserID).
		Count(&count).Error
	return count > 0, err
}

func (r *followRepo) ListByFollowUserID(ctx context.Context, followUserID int64) ([]model.Follow, error) {
	var follows []model.Follow
	err := r.db.WithContext(ctx).
		Table((&model.Follow{}).TableName()).
		Where("follow_user_id = ?", followUserID).
		Find(&follows).Error
	return follows, err
}
