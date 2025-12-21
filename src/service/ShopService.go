package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"local-review-go/src/config/mysql"
	redisClient "local-review-go/src/config/redis"
	"local-review-go/src/model"
	"local-review-go/src/utils"
	"strconv"
	"time"

	"github.com/jinzhu/gorm"
	redisConfig "github.com/redis/go-redis/v9"
)

type ShopService struct {
}

var ShopManager *ShopService

var distLock *utils.DistributedLock

// Global BloomFilter instance
var shopBloomFilter *utils.BloomFilter

func SetShopBloomFilter(bf *utils.BloomFilter) {
	shopBloomFilter = bf
}

const (
	MAX_REDIS_DATA_QUEUE = 10
)

var redisDataQueue chan int64

func init() {
	redisDataQueue = make(chan int64, MAX_REDIS_DATA_QUEUE)
	distLock = utils.NewDistributedLock(redisClient.GetRedisClient())
	go ShopManager.SyncUpdateCache()
}

func (*ShopService) QueryShopById(id int64) (model.Shop, error) {
	var shop model.Shop
	shop.Id = id
	err := shop.QueryShopById(id)
	return shop, err
}

func (*ShopService) SaveShop(shop *model.Shop) error {
	err := shop.SaveShop()
	return err
}

func (*ShopService) UpdateShop(shop *model.Shop) error {
	err := shop.UpdateShop(mysql.GetMysqlDB())
	return err
}

func (*ShopService) QueryByType(typeId int, current int) ([]model.Shop, error) {
	var shopUtils model.Shop
	shops, err := shopUtils.QueryShopByType(typeId, current)
	return shops, err
}

func (*ShopService) QueryByName(name string, current int) ([]model.Shop, error) {
	var shopUtils model.Shop
	shops, err := shopUtils.QueryShopByName(name, current)
	return shops, err
}

// QueryShopByIdWithCache 如果缓存未命中，则查询数据库，将数据库结果写入缓存，并设置超时时间
func (*ShopService) QueryShopByIdWithCache(id int64) (model.Shop, error) {
	// 1. Check Bloom Filter
	if shopBloomFilter != nil {
		exists, err := shopBloomFilter.Contains(id)
		if err != nil {
			// If Redis query fails, what to do? Log and proceed or fail?
			// Usually safe to proceed to avoid false negative if network issue, but here we assume critical dependency or log error.
			// Ideally log warning and proceed to cache/db
			fmt.Printf("BloomFilter check failed: %v\n", err)
		} else if !exists {
			// If definitely not in Bloom Filter
			return model.Shop{}, errors.New("shop not found (blocked by Bloom Filter)")
		}
	}

	redisKey := utils.CACHE_SHOP_KEY + strconv.FormatInt(id, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shopInfo, err := redisClient.GetRedisClient().Get(ctx, redisKey).Result()
	if err == nil {
		var shop model.Shop
		err = json.Unmarshal([]byte(shopInfo), &shop)
		if err != nil {
			return model.Shop{}, err
		}
		return shop, nil
	}

	if errors.Is(err, redisConfig.Nil) {
		var shop model.Shop
		shop.Id = id
		err = shop.QueryShopById(id)
		if err != nil {
			return model.Shop{}, err
		}

		redisValue, err := json.Marshal(shop)
		if err != nil {
			return model.Shop{}, err
		}

		// 超时剔除策略
		err = redisClient.GetRedisClient().Set(ctx, redisKey, string(redisValue), time.Minute).Err()

		if err != nil {
			return model.Shop{}, err
		}
		return shop, nil
	}

	return model.Shop{}, err
}

// UpdateShopWithCacheCallBack 缓存更新的最佳实践方法
func (*ShopService) UpdateShopWithCacheCallBack(db *gorm.DB, shop *model.Shop) error {
	return db.Transaction(func(tx *gorm.DB) error {
		err := shop.QueryShopById(shop.Id)
		if err != nil {
			return err
		}

		// update the database
		err = shop.UpdateShop(tx)
		if err != nil {
			return err
		}

		// delete the cache
		redisKey := utils.CACHE_SHOP_KEY + strconv.FormatInt(shop.Id, 10)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err = redisClient.GetRedisClient().Del(ctx, redisKey).Err()

		if err != nil {
			return err
		}

		return nil
	})
}

func (*ShopService) UpdateShopWithCache(shop *model.Shop) error {
	return ShopManager.UpdateShopWithCacheCallBack(mysql.GetMysqlDB(), shop)
}

// QueryShopByIdWithCacheNull 缓存穿透的解决方法: 缓存空对象
func (*ShopService) QueryShopByIdWithCacheNull(id int64) (model.Shop, error) {
	// 1. Check Bloom Filter
	if shopBloomFilter != nil {
		exists, err := shopBloomFilter.Contains(id)
		if err != nil {
			fmt.Printf("BloomFilter check failed: %v\n", err)
		} else if !exists {
			return model.Shop{}, errors.New("shop not found (blocked by Bloom Filter)")
		}
	}

	redisKey := utils.CACHE_SHOP_KEY + strconv.FormatInt(id, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shopInfoStr, err := redisClient.GetRedisClient().Get(ctx, redisKey).Result()

	if err == nil {
		var shopInfo model.Shop
		if shopInfoStr == "" {
			return model.Shop{}, nil
		}
		err = json.Unmarshal([]byte(shopInfoStr), &shopInfo)
		if err != nil {
			return model.Shop{}, err
		}
		return shopInfo, nil
	}

	if errors.Is(err, redisConfig.Nil) {
		var shopInfo model.Shop
		shopInfo.Id = id
		err = shopInfo.QueryShopById(id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = redisClient.GetRedisClient().Set(ctx, redisKey, "", time.Minute).Err()
			if err != nil {
				return model.Shop{}, err
			}
			return model.Shop{}, nil
		}

		redisValue, err := json.Marshal(shopInfo)
		if err != nil {
			return model.Shop{}, err
		}
		err = redisClient.GetRedisClient().Set(ctx, redisKey, string(redisValue), time.Minute).Err()
		if err != nil {
			return model.Shop{}, err
		}
		return shopInfo, nil
	}
	return model.Shop{}, nil
}

// QueryShopByIdPassThrough 利用互斥锁解决热点 Key 问题(也就是缓存击穿问题)
func (*ShopService) QueryShopByIdPassThrough(id int64) (model.Shop, error) {
	redisKey := utils.CACHE_SHOP_KEY + strconv.FormatInt(id, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shopInfoStr, err := redisClient.GetRedisClient().Get(ctx, redisKey).Result()

	if err == nil {
		if shopInfoStr == "" {
			return model.Shop{}, err
		}

		var shopInfo model.Shop
		err = json.Unmarshal([]byte(shopInfoStr), &shopInfo)
		if err != nil {
			return model.Shop{}, err
		}
		return shopInfo, err
	}

	if errors.Is(err, redisConfig.Nil) {
		lockKey := utils.CACHE_LOCK_KEY + strconv.FormatInt(id, 10)
		ctx := context.Background()
		flag, token, err := distLock.LockWithWatchDog(ctx, lockKey, 10*time.Second)
		if err != nil {
			return model.Shop{}, err
		}

		// 没有获取到锁
		if !flag {
			time.Sleep(time.Millisecond * 50)
			return ShopManager.QueryShopByIdPassThrough(id)
		}

		// 重新建立缓存
		defer distLock.UnlockWithWatchDog(ctx, lockKey, token)
		var shopInfo model.Shop
		err = shopInfo.QueryShopById(id)

		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = redisClient.GetRedisClient().Set(ctx, redisKey, "", time.Minute).Err()
			return model.Shop{}, err
		}

		if err != nil {
			return model.Shop{}, err
		}

		redisValue, err := json.Marshal(shopInfo)
		if err != nil {
			return model.Shop{}, err
		}

		err = redisClient.GetRedisClient().Set(ctx, redisKey, string(redisValue), time.Minute).Err()
		if err != nil {
			return model.Shop{}, err
		}
		return shopInfo, nil
	}
	return model.Shop{}, err
}

// @Description: use the logic expire to deal with the cache pass through
// 注意：逻辑过期一定要先进行数据预热，将我们热点数据加载到缓存中
func (*ShopService) QueryShopByIdWithLogicExpire(id int64) (model.Shop, error) {
	redisKey := utils.CACHE_SHOP_KEY + strconv.FormatInt(id, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	redisDataStr, err := redisClient.GetRedisClient().Get(ctx, redisKey).Result()

	// hot key is in redis
	if errors.Is(err, redisConfig.Nil) {
		return model.Shop{}, nil
	}

	if err == nil {
		if redisDataStr == "" {
			return model.Shop{}, nil
		}

		var redisData utils.RedisData[model.Shop]
		err = json.Unmarshal([]byte(redisDataStr), &redisData)
		if err != nil {
			return model.Shop{}, err
		}

		if redisData.ExpireTime.After(time.Now()) {
			return redisData.Data, nil
		}
		// 否则过期,需要重新建立缓存

		lockKey := utils.CACHE_LOCK_KEY + strconv.FormatInt(id, 10)
		ctx := context.Background()
		flag, token, err := distLock.LockWithWatchDog(ctx, lockKey, 10*time.Second)
		if err != nil {
			return model.Shop{}, err
		}

		// if not get the lock
		if !flag {
			return redisData.Data, nil
		}

		// if get the lock
		defer distLock.UnlockWithWatchDog(ctx, lockKey, token)
		redisDataQueue <- id
		// go func() {
		// 	var shopInfo model.Shop
		// 	err = shopInfo.QueryShopById(id)
		// 	if err != nil {
		// 		return
		// 	}
		// 	var redisDataToSave utils.RedisData[model.Shop]
		//
		// 	redisDataToSave.Data = shopInfo
		// 	// the time of hot key exists
		// 	redisDataToSave.ExpireTime = time.Now().Add(time.Second * utils.HOT_KEY_EXISTS_TIME)
		//
		// 	redisValue,err := json.Marshal(redisDataToSave)
		// 	err = redisClient.GetRedisClient().Set(ctx , redisKey , string(redisValue) , 0).Err()
		// 	if err != nil {
		// 		return
		// 	}
		// 	return
		// }()
		//
		return redisData.Data, nil
	}

	return model.Shop{}, err
}

func (*ShopService) SyncUpdateCache() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for {
		id := <-redisDataQueue

		redisKey := utils.CACHE_SHOP_KEY + strconv.FormatInt(id, 10)

		var shopInfo model.Shop
		err := shopInfo.QueryShopById(id)

		if err != nil {
			continue
		}

		var redisDataToSave utils.RedisData[model.Shop]

		redisDataToSave.Data = shopInfo
		// the time of hot key exists
		redisDataToSave.ExpireTime = time.Now().Add(time.Second * utils.HOT_KEY_EXISTS_TIME)

		redisValue, err := json.Marshal(redisDataToSave)
		err = redisClient.GetRedisClient().Set(ctx, redisKey, string(redisValue), 0).Err()
		if err != nil {
			continue
		}
	}
}

// 查询店铺列表（支持地理位置排序）
func (s *ShopService) QueryShopByType(typeID, current int,
	x, y float64) ([]model.Shop, error) {

	// 1. 无坐标，按类型分页查询
	if x == 0 || y == 0 {
		return new(model.Shop).QueryShopByType(typeID, current)
	}

	const pageSize = utils.DEFAULTPAGESIZE
	from := (current - 1) * pageSize
	to := current * pageSize // slice 上界（开区间）

	key := utils.SHOP_GEO_KEY + strconv.Itoa(typeID)

	// 2. Redis GEO 查询
	query := &redisConfig.GeoSearchLocationQuery{
		GeoSearchQuery: redisConfig.GeoSearchQuery{
			Longitude:  x,
			Latitude:   y,
			Radius:     5000, // 5km
			RadiusUnit: "m",
			Sort:       "ASC",
			Count:      to, // 先取够 “当前页” 之前的全部数据
		},
		WithDist: true, // 让 Redis 返回距离
	}

	ctx := context.Background()
	locs, err := redisClient.GetRedisClient().
		GeoSearchLocation(ctx, key, query).Result()
	if err != nil && !errors.Is(err, redisConfig.Nil) {
		return nil, fmt.Errorf("Redis GEO 查询失败: %w", err)
	}
	if len(locs) == 0 || len(locs) <= from {
		return []model.Shop{}, nil // 空页
	}

	// 3. 截取当前页，收集 id & 距离
	if to > len(locs) {
		to = len(locs)
	}
	ids := make([]int64, 0, to-from)
	dist := make(map[int64]float64, to-from)
	for _, loc := range locs[from:to] {
		id, _ := strconv.ParseInt(loc.Name, 10, 64)
		ids = append(ids, id)
		dist[id] = loc.Dist
	}

	// 4. 数据库一次性查询
	shops, err := new(model.Shop).QueryShopByIds(ids)
	if err != nil {
		return nil, fmt.Errorf("数据库查询失败: %w", err)
	}
	for i := range shops {
		shops[i].Distance = dist[shops[i].Id] // ⚠️ 字段是 Id
	}
	return shops, nil
}
