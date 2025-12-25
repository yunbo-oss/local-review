package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	redisClient "local-review-go/src/config/redis"
	"local-review-go/src/model"
	"local-review-go/src/utils/redisx"
)

type ShopTypeLogic interface {
	QueryShopTypeList(ctx context.Context) ([]model.ShopType, error)
}

type shopTypeLogic struct{}

func NewShopTypeLogic() ShopTypeLogic {
	return &shopTypeLogic{}
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
		var shoplist []model.ShopType
		var shopType model.ShopType
		shoplist, err = shopType.QueryTypeList()
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
