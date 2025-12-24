package service

import (
	"context"
	"encoding/json"
	"errors"
	redisClient "local-review-go/src/config/redis"
	"local-review-go/src/model"
	"local-review-go/src/utils"

	redisConfig "github.com/redis/go-redis/v9"
)

type ShopTypeService struct {
}

var ShopTypeManager *ShopTypeService

func (*ShopTypeService) QueryShopTypeList() ([]model.ShopType, error) {
	var shopTypeUtils model.ShopType
	shopTypeList, err := shopTypeUtils.QueryTypeList()
	return shopTypeList, err
}

func (*ShopTypeService) QueryShopTypeListWithCache(ctx context.Context) ([]model.ShopType, error) {
	redisKey := utils.CACHE_SHOP_LIST

	shopListStr, err := redisClient.GetRedisClient().Get(ctx, redisKey).Result()

	if err == nil {
		var shoplist []model.ShopType
		err = json.Unmarshal([]byte(shopListStr), &shoplist)
		if err != nil {
			return []model.ShopType{}, err
		}
		return shoplist, nil
	}

	if errors.Is(err, redisConfig.Nil) {
		var shoplist []model.ShopType
		var shopTypeUtils model.ShopType
		shoplist, err = shopTypeUtils.QueryTypeList()
		if err != nil {
			return []model.ShopType{}, nil
		}

		redisValue, err := json.Marshal(shoplist)
		if err != nil {
			return []model.ShopType{}, err
		}

		err = redisClient.GetRedisClient().Set(ctx, redisKey, string(redisValue), 0).Err()

		if err != nil {
			return []model.ShopType{}, err
		}

		return shoplist, err
	}

	return []model.ShopType{}, err
}

func (*ShopTypeService) QueryTypeListWithCacheList(ctx context.Context) ([]model.ShopType, error) {
	redisKey := utils.CACHE_SHOP_LIST

	shopStrList, err := redisClient.GetRedisClient().LRange(ctx, redisKey, 0, -1).Result()

	if err != nil {
		return []model.ShopType{}, err
	}

	if len(shopStrList) > 0 {
		var shoplist []model.ShopType
		for _, value := range shopStrList {
			var shopType model.ShopType
			err = json.Unmarshal([]byte(value), &shopType)
			if err != nil {
				return []model.ShopType{}, err
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
			return []model.ShopType{}, err
		}

		for _, value := range shoplist {
			redisValue, err := json.Marshal(value)
			if err != nil {
				return []model.ShopType{}, err
			}

			err = redisClient.GetRedisClient().RPush(ctx, redisKey, string(redisValue)).Err()

			if err != nil {
				return []model.ShopType{}, err
			}
		}

		return shoplist, nil
	}

	return []model.ShopType{}, err
}
