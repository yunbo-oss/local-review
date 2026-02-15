package repository

import (
	"context"
	"local-review-go/src/model"
	"local-review-go/src/repository/interface"

	"gorm.io/gorm"
)

type shopTypeRepo struct {
	db *gorm.DB
}

// NewShopTypeRepo 创建店铺类型 Repository
func NewShopTypeRepo(db *gorm.DB) interfaces.ShopTypeRepo {
	return &shopTypeRepo{db: db}
}

func (r *shopTypeRepo) ListAll(ctx context.Context) ([]model.ShopType, error) {
	var shopType model.ShopType
	var list []model.ShopType
	err := r.db.WithContext(ctx).
		Table(shopType.TableName()).
		Order("sort asc").
		Find(&list).Error
	return list, err
}
