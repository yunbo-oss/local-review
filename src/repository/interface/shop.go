package interfaces

import (
	"context"
	"local-review-go/src/model"

	"gorm.io/gorm"
)

// ShopRepo 店铺数据访问接口
type ShopRepo interface {
	GetByID(ctx context.Context, id int64) (*model.Shop, error)
	GetByIDs(ctx context.Context, ids []int64) ([]model.Shop, error)
	Create(ctx context.Context, shop *model.Shop) error
	Update(ctx context.Context, shop *model.Shop, tx *gorm.DB) error
	ListByType(ctx context.Context, typeId int, current int) ([]model.Shop, error)
	ListByName(ctx context.Context, name string, current int) ([]model.Shop, error)
	ListAllIDs(ctx context.Context) ([]int64, error)
}
