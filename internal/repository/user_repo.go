package repository

import (
	"context"
	"local-review-go/internal/model"
	"local-review-go/internal/repository/interface"
	"time"

	"gorm.io/gorm"
)

type userRepo struct {
	db *gorm.DB
}

// NewUserRepo 创建用户 Repository
func NewUserRepo(db *gorm.DB) interfaces.UserRepo {
	return &userRepo{db: db}
}

func (r *userRepo) GetByID(ctx context.Context, id int64) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Table(user.TableName()).Where("id = ?", id).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepo) GetByPhone(ctx context.Context, phone string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Table(user.TableName()).Where("phone = ?", phone).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepo) Create(ctx context.Context, user *model.User) error {
	now := time.Now()
	if user.CreateTime.IsZero() {
		user.CreateTime = now
	}
	if user.UpdateTime.IsZero() {
		user.UpdateTime = now
	}
	return r.db.WithContext(ctx).Table(user.TableName()).Create(user).Error
}

func (r *userRepo) GetByIDs(ctx context.Context, ids []int64) ([]model.User, error) {
	if len(ids) == 0 {
		return []model.User{}, nil
	}
	var users []model.User
	err := r.db.WithContext(ctx).
		Table((&model.User{}).TableName()).
		Where("id IN ?", ids).
		Find(&users).Error
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]model.User, len(users))
	for _, user := range users {
		byID[user.Id] = user
	}
	ordered := make([]model.User, 0, len(users))
	for _, id := range ids {
		if user, ok := byID[id]; ok {
			ordered = append(ordered, user)
		}
	}
	return ordered, nil
}
