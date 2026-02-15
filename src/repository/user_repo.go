package repository

import (
	"context"
	"fmt"
	"local-review-go/src/model"
	"local-review-go/src/repository/interface"
	"strconv"
	"strings"
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
	idStrs := make([]string, len(ids))
	for i, id := range ids {
		idStrs[i] = strconv.FormatInt(id, 10)
	}
	order := fmt.Sprintf("FIELD(id,%s)", strings.Join(idStrs, ","))

	var users []model.User
	err := r.db.WithContext(ctx).
		Table((&model.User{}).TableName()).
		Where("id IN ?", ids).
		Order(order).
		Find(&users).Error
	return users, err
}
