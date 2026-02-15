package interfaces

import (
	"context"
	"local-review-go/src/model"

	"gorm.io/gorm"
)

// VoucherOrderRepo 优惠券订单数据访问接口
type VoucherOrderRepo interface {
	HasPurchased(ctx context.Context, userID, voucherID int64, tx *gorm.DB) (bool, error)
	Create(ctx context.Context, order *model.VoucherOrder, tx *gorm.DB) error
}
