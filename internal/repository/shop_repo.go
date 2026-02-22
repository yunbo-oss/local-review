package repository

import (
	"context"
	"fmt"
	"local-review-go/internal/model"
	"local-review-go/internal/repository/interface"
	"local-review-go/pkg/utils/redisx"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type shopRepo struct {
	db *gorm.DB
}

// NewShopRepo 创建店铺 Repository
func NewShopRepo(db *gorm.DB) interfaces.ShopRepo {
	return &shopRepo{db: db}
}

func (r *shopRepo) GetByID(ctx context.Context, id int64) (*model.Shop, error) {
	var shop model.Shop
	err := r.db.WithContext(ctx).Model(&shop).Where("id = ?", id).First(&shop).Error
	if err != nil {
		return nil, err
	}
	return &shop, nil
}

func (r *shopRepo) GetByIDs(ctx context.Context, ids []int64) ([]model.Shop, error) {
	if len(ids) == 0 {
		return []model.Shop{}, nil
	}
	idStrs := make([]string, len(ids))
	for i, id := range ids {
		idStrs[i] = strconv.FormatInt(id, 10)
	}
	order := fmt.Sprintf("FIELD(id,%s)", strings.Join(idStrs, ","))

	var shops []model.Shop
	err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Order(order).
		Find(&shops).Error
	return shops, err
}

func (r *shopRepo) Create(ctx context.Context, shop *model.Shop) error {
	now := time.Now()
	if shop.CreateTime.IsZero() {
		shop.CreateTime = now
	}
	if shop.UpdateTime.IsZero() {
		shop.UpdateTime = now
	}
	return r.db.WithContext(ctx).Table(shop.TableName()).Create(shop).Error
}

func (r *shopRepo) Update(ctx context.Context, shop *model.Shop, tx *gorm.DB) error {
	executor := r.db
	if tx != nil {
		executor = tx
	}
	return executor.WithContext(ctx).Model(shop).Save(shop).Error
}

func (r *shopRepo) ListByType(ctx context.Context, typeId int, current int) ([]model.Shop, error) {
	var shop model.Shop
	var shops []model.Shop
	err := r.db.WithContext(ctx).
		Table(shop.TableName()).
		Where("type_id = ?", typeId).
		Offset((current - 1) * redisx.DEFAULTPAGESIZE).
		Limit(redisx.DEFAULTPAGESIZE).
		Find(&shops).Error
	return shops, err
}

func (r *shopRepo) ListByName(ctx context.Context, name string, current int) ([]model.Shop, error) {
	var shop model.Shop
	var shops []model.Shop
	err := r.db.WithContext(ctx).
		Table(shop.TableName()).
		Where("name LIKE ?", name).
		Offset((current - 1) * redisx.MAXPAGESIZE).
		Limit(redisx.MAXPAGESIZE).
		Find(&shops).Error
	return shops, err
}

func (r *shopRepo) ListAllIDs(ctx context.Context) ([]int64, error) {
	var shops []model.Shop
	err := r.db.WithContext(ctx).
		Model(&model.Shop{}).
		Select("id").
		Find(&shops).Error
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(shops))
	for i := range shops {
		ids[i] = shops[i].Id
	}
	return ids, nil
}
