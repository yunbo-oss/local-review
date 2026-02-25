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

func (r *voucherOrderRepo) GetByID(ctx context.Context, orderID int64) (*model.VoucherOrder, error) {
	var order model.VoucherOrder
	err := r.db.WithContext(ctx).
		Table(order.TableName()).
		Where("id = ?", orderID).
		First(&order).Error
	if err != nil {
		return nil, err
	}
	return &order, nil
}

// UpdateStatus 更新订单状态，仅当 fromStatus 匹配时更新，返回影响行数
func (r *voucherOrderRepo) UpdateStatus(ctx context.Context, orderID int64, fromStatus, toStatus int, tx *gorm.DB) (int64, error) {
	executor := r.db
	if tx != nil {
		executor = tx
	}
	result := executor.WithContext(ctx).
		Table((&model.VoucherOrder{}).TableName()).
		Where("id = ? AND status = ?", orderID, fromStatus).
		Update("status", toStatus)
	return result.RowsAffected, result.Error
}
