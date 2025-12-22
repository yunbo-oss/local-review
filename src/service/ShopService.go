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

	redisConfig "github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type ShopService struct {
}

var ShopManager *ShopService

var distLock *utils.DistributedLock

// 全局布隆过滤器实例
var shopBloomFilter *utils.BloomFilter

// SetShopBloomFilter 设置布隆过滤器实例（由 main.go 在初始化时调用）
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
	if err != nil {
		logrus.Errorf("Failed to save shop to database: %v, shop data: %+v", err, shop)
		return err
	}

	// 数据库保存成功后，同步更新布隆过滤器
	if shopBloomFilter != nil && shop.Id > 0 {
		if err := shopBloomFilter.Add(shop.Id); err != nil {
			// 布隆过滤器更新失败不影响主流程，但记录警告日志
			logrus.Warnf("Failed to add shop %d to Bloom Filter after save: %v", shop.Id, err)
		} else {
			logrus.Debugf("Successfully added shop %d to Bloom Filter", shop.Id)
		}
	}

	return nil
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
			// Redis 查询失败时，记录警告但继续查询缓存/数据库，避免因网络问题导致误判
			logrus.Warnf("BloomFilter check failed for shop %d: %v, proceeding to cache/DB", id, err)
		} else if !exists {
			// 布隆过滤器判定不存在，直接返回错误
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

		// 防御性编程：如果数据库查询成功，确保布隆过滤器中也存在
		// 防止因预热遗漏或并发问题导致的数据不一致
		if shopBloomFilter != nil && shop.Id > 0 {
			exists, bfErr := shopBloomFilter.Contains(shop.Id)
			if bfErr == nil && !exists {
				// 布隆过滤器未命中但数据库有数据，添加到布隆过滤器
				if addErr := shopBloomFilter.Add(shop.Id); addErr != nil {
					logrus.Warnf("Failed to add shop %d to Bloom Filter after DB query: %v", shop.Id, addErr)
				} else {
					logrus.Debugf("Defensively added shop %d to Bloom Filter after DB query", shop.Id)
				}
			}
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
			// Redis 查询失败时，记录警告但继续查询缓存/数据库，避免因网络问题导致误判
			logrus.Warnf("BloomFilter check failed for shop %d: %v, proceeding to cache/DB", id, err)
		} else if !exists {
			// 布隆过滤器判定不存在，直接返回错误
			logrus.Infof("Bloom Filter blocked shop %d (not exists)", id)
			return model.Shop{}, errors.New("shop not found (blocked by Bloom Filter)")
		} else {
			logrus.Debugf("Bloom Filter passed for shop %d (exists)", id)
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
			// 数据库不存在，设置空缓存防止缓存穿透
			err = redisClient.GetRedisClient().Set(ctx, redisKey, "", time.Minute).Err()
			if err != nil {
				return model.Shop{}, err
			}
			return model.Shop{}, nil
		}

		if err != nil {
			return model.Shop{}, err
		}

		// 防御性编程：如果数据库查询成功，确保布隆过滤器中也存在
		if shopBloomFilter != nil && shopInfo.Id > 0 {
			exists, bfErr := shopBloomFilter.Contains(shopInfo.Id)
			if bfErr == nil && !exists {
				// 布隆过滤器未命中但数据库有数据，添加到布隆过滤器
				if addErr := shopBloomFilter.Add(shopInfo.Id); addErr != nil {
					logrus.Warnf("Failed to add shop %d to Bloom Filter after DB query: %v", shopInfo.Id, addErr)
				} else {
					logrus.Debugf("Defensively added shop %d to Bloom Filter after DB query", shopInfo.Id)
				}
			}
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

		// 防御性编程：如果数据库查询成功，确保布隆过滤器中也存在
		if shopBloomFilter != nil && shopInfo.Id > 0 {
			exists, bfErr := shopBloomFilter.Contains(shopInfo.Id)
			if bfErr == nil && !exists {
				if addErr := shopBloomFilter.Add(shopInfo.Id); addErr != nil {
					logrus.Warnf("Failed to add shop %d to Bloom Filter after DB query: %v", shopInfo.Id, addErr)
				} else {
					logrus.Debugf("Defensively added shop %d to Bloom Filter after DB query", shopInfo.Id)
				}
			}
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
		if err != nil {
			logrus.Warnf("Failed to marshal shop data for shop %d: %v", id, err)
			continue
		}
		err = redisClient.GetRedisClient().Set(ctx, redisKey, string(redisValue), 0).Err()
		if err != nil {
			logrus.Warnf("Failed to set cache for shop %d: %v", id, err)
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
		return nil, fmt.Errorf("redis GEO 查询失败: %w", err)
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
