package repository

import (
	"context"
	"local-review-go/src/model"
	"local-review-go/src/repository/interface"
	"time"

	"gorm.io/gorm"
)

type voucherRepo struct {
	db *gorm.DB
}

// NewVoucherRepo 创建优惠券 Repository
func NewVoucherRepo(db *gorm.DB) interfaces.VoucherRepo {
	return &voucherRepo{db: db}
}

func (r *voucherRepo) Create(ctx context.Context, voucher *model.Voucher, tx *gorm.DB) error {
	executor := r.db
	if tx != nil {
		executor = tx
	}
	now := time.Now()
	if voucher.CreateTime.IsZero() {
		voucher.CreateTime = now
	}
	if voucher.UpdateTime.IsZero() {
		voucher.UpdateTime = now
	}
	return executor.WithContext(ctx).Table(voucher.TableName()).Create(voucher).Error
}

func (r *voucherRepo) ListByShopID(ctx context.Context, shopID int64) ([]model.Voucher, error) {
	var voucher model.Voucher
	var vouchers []model.Voucher
	err := r.db.WithContext(ctx).
		Table(voucher.TableName()).
		Where("shop_id = ?", shopID).
		Find(&vouchers).Error
	if err != nil {
		return nil, err
	}
	for i := range vouchers {
		if vouchers[i].Type == 1 {
			var seckill model.SecKillVoucher
			err = r.db.WithContext(ctx).
				Table(seckill.TableName()).
				Where("voucher_id = ?", vouchers[i].Id).
				First(&seckill).Error
			if err != nil {
				return nil, err
			}
			vouchers[i].BeginTime = seckill.BeginTime
			vouchers[i].EndTime = seckill.EndTime
			vouchers[i].Stock = seckill.Stock
		}
	}
	return vouchers, nil
}
