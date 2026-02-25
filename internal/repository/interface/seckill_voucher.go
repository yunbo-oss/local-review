package interfaces

import (
	"context"
	"local-review-go/internal/model"

	"gorm.io/gorm"
)

// SeckillVoucherRepo 秒杀优惠券数据访问接口
type SeckillVoucherRepo interface {
	GetByID(ctx context.Context, voucherID int64) (*model.SecKillVoucher, error)
	Create(ctx context.Context, sv *model.SecKillVoucher, tx *gorm.DB) error
	DecrStock(ctx context.Context, voucherID int64, tx *gorm.DB) error
	IncrStock(ctx context.Context, voucherID int64, tx *gorm.DB) error
}
