package interfaces

import (
	"context"
	"local-review-go/src/model"

	"gorm.io/gorm"
)

// VoucherRepo 优惠券数据访问接口
type VoucherRepo interface {
	Create(ctx context.Context, voucher *model.Voucher, tx *gorm.DB) error
	ListByShopID(ctx context.Context, shopID int64) ([]model.Voucher, error)
}
