package interfaces

import (
	"context"
	"local-review-go/src/model"
)

// ShopTypeRepo 店铺类型数据访问接口
type ShopTypeRepo interface {
	ListAll(ctx context.Context) ([]model.ShopType, error)
}
