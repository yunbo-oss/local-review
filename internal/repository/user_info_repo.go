package repository

import (
	"context"
	"local-review-go/internal/model"
	"local-review-go/internal/repository/interface"

	"gorm.io/gorm"
)

type userInfoRepo struct {
	db *gorm.DB
}

// NewUserInfoRepo 创建用户详情 Repository
func NewUserInfoRepo(db *gorm.DB) interfaces.UserInfoRepo {
	return &userInfoRepo{db: db}
}

func (r *userInfoRepo) GetByUserID(ctx context.Context, userID int64) (*model.UserInfo, error) {
	var userInfo model.UserInfo
	err := r.db.WithContext(ctx).Table(userInfo.TableName()).Where("user_id = ?", userID).First(&userInfo).Error
	if err != nil {
		return nil, err
	}
	return &userInfo, nil
}
