package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"local-review-go/internal/config/mysql"
	redisClient "local-review-go/internal/config/redis"
	"local-review-go/internal/model"
	"local-review-go/internal/repository"
	repoInterfaces "local-review-go/internal/repository/interface"
	"local-review-go/pkg/utils/redisx"
)

type ShopTypeLogic interface {
	QueryShopTypeList(ctx context.Context) ([]model.ShopType, error)
}

type shopTypeLogic struct {
	shopTypeRepo repoInterfaces.ShopTypeRepo
}

// ShopTypeLogicDeps 用于实例化 shopTypeLogic 的依赖
type ShopTypeLogicDeps struct {
	ShopTypeRepo repoInterfaces.ShopTypeRepo
}

func NewShopTypeLogic(deps ShopTypeLogicDeps) ShopTypeLogic {
	shopTypeRepo := deps.ShopTypeRepo
	if shopTypeRepo == nil {
		shopTypeRepo = repository.NewShopTypeRepo(mysql.GetMysqlDB())
	}
	return &shopTypeLogic{shopTypeRepo: shopTypeRepo}
}

func (l *shopTypeLogic) QueryShopTypeList(ctx context.Context) ([]model.ShopType, error) {
	redisKey := redisx.CACHE_SHOP_LIST

	shopStrList, err := redisClient.GetRedisClient().LRange(ctx, redisKey, 0, -1).Result()
	if err != nil {
		return []model.ShopType{}, fmt.Errorf("redis lrange shop types: %w", err)
	}

	if len(shopStrList) > 0 {
		var shoplist []model.ShopType
		for _, value := range shopStrList {
			var shopType model.ShopType
			err = json.Unmarshal([]byte(value), &shopType)
			if err != nil {
				return []model.ShopType{}, fmt.Errorf("unmarshal shop type cache: %w", err)
			}
			shoplist = append(shoplist, shopType)
		}
		return shoplist, nil
	}

	if len(shopStrList) == 0 {
		shoplist, err := l.shopTypeRepo.ListAll(ctx)
		if err != nil {
			return []model.ShopType{}, fmt.Errorf("db query shop type list: %w", err)
		}

		for _, value := range shoplist {
			redisValue, err := json.Marshal(value)
			if err != nil {
				return []model.ShopType{}, fmt.Errorf("marshal shop type: %w", err)
			}

			err = redisClient.GetRedisClient().RPush(ctx, redisKey, string(redisValue)).Err()
			if err != nil {
				return []model.ShopType{}, fmt.Errorf("rpush shop type cache: %w", err)
			}
		}

		return shoplist, nil
	}

	return []model.ShopType{}, errors.New("unexpected shop type cache state")
}
