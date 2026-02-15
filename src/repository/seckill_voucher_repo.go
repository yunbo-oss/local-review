package repository

import (
	"context"
	"local-review-go/src/model"
	"local-review-go/src/repository/interface"

	"gorm.io/gorm"
)

type seckillVoucherRepo struct {
	db *gorm.DB
}

// NewSeckillVoucherRepo 创建秒杀优惠券 Repository
func NewSeckillVoucherRepo(db *gorm.DB) interfaces.SeckillVoucherRepo {
	return &seckillVoucherRepo{db: db}
}

func (r *seckillVoucherRepo) GetByID(ctx context.Context, voucherID int64) (*model.SecKillVoucher, error) {
	var sv model.SecKillVoucher
	err := r.db.WithContext(ctx).
		Table(sv.TableName()).
		Where("voucher_id = ?", voucherID).
		First(&sv).Error
	if err != nil {
		return nil, err
	}
	return &sv, nil
}

func (r *seckillVoucherRepo) Create(ctx context.Context, sv *model.SecKillVoucher, tx *gorm.DB) error {
	executor := r.db
	if tx != nil {
		executor = tx
	}
	return executor.WithContext(ctx).Table(sv.TableName()).Create(sv).Error
}

func (r *seckillVoucherRepo) DecrStock(ctx context.Context, voucherID int64, tx *gorm.DB) error {
	executor := r.db
	if tx != nil {
		executor = tx
	}
	result := executor.WithContext(ctx).Exec(`
		UPDATE tb_seckill_voucher 
		SET stock = stock - 1 
		WHERE voucher_id = ? AND stock > 0
	`, voucherID)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return model.ErrStockNotEnough
	}
	return nil
}
