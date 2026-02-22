package repository

import (
	"context"
	"local-review-go/internal/model"
	"local-review-go/internal/repository/interface"
	"time"

	"gorm.io/gorm"
)

type voucherOrderRepo struct {
	db *gorm.DB
}

// NewVoucherOrderRepo 创建优惠券订单 Repository
func NewVoucherOrderRepo(db *gorm.DB) interfaces.VoucherOrderRepo {
	return &voucherOrderRepo{db: db}
}

func (r *voucherOrderRepo) HasPurchased(ctx context.Context, userID, voucherID int64, tx *gorm.DB) (bool, error) {
	executor := r.db
	if tx != nil {
		executor = tx
	}
	var count int64
	err := executor.WithContext(ctx).
		Table((&model.VoucherOrder{}).TableName()).
		Where("user_id = ? AND voucher_id = ?", userID, voucherID).
		Count(&count).Error
	return count > 0, err
}

func (r *voucherOrderRepo) Create(ctx context.Context, order *model.VoucherOrder, tx *gorm.DB) error {
	executor := r.db
	if tx != nil {
		executor = tx
	}
	now := time.Now()
	if order.CreateTime.IsZero() {
		order.CreateTime = now
	}
	if order.UpdateTime.IsZero() {
		order.UpdateTime = now
	}
	return executor.WithContext(ctx).Table(order.TableName()).Create(order).Error
}
