package interfaces

import (
	"context"
	"local-review-go/internal/model"

	"gorm.io/gorm"
)

// VoucherOrderRepo 优惠券订单数据访问接口
type VoucherOrderRepo interface {
	HasPurchased(ctx context.Context, userID, voucherID int64, tx *gorm.DB) (bool, error)
	Create(ctx context.Context, order *model.VoucherOrder, tx *gorm.DB) error
	GetByID(ctx context.Context, orderID int64) (*model.VoucherOrder, error)
	UpdateStatus(ctx context.Context, orderID int64, fromStatus, toStatus int, tx *gorm.DB) (int64, error)
}
