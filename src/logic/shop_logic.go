package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"local-review-go/src/config/mysql"
	redisClient "local-review-go/src/config/redis"
	"local-review-go/src/model"
	"local-review-go/src/utils"
	"local-review-go/src/utils/redisx"
	"strconv"
	"time"

	redisv9 "github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

const (
	maxRedisDataQueue = 10
)

// ShopLogic 封装店铺领域的业务流程。
type ShopLogic interface {
	QueryShopById(ctx context.Context, id int64) (model.Shop, error)
	SaveShop(ctx context.Context, shop *model.Shop) error
	UpdateShop(ctx context.Context, shop *model.Shop) error
	UpdateShopWithCache(ctx context.Context, shop *model.Shop) error
	QueryByType(typeId int, current int) ([]model.Shop, error)
	QueryByName(ctx context.Context, name string, current int) ([]model.Shop, error)

	QueryShopByIdWithCache(ctx context.Context, id int64) (model.Shop, error)
	QueryShopByIdWithCacheNull(ctx context.Context, id int64) (model.Shop, error)
	QueryShopByIdPassThrough(ctx context.Context, id int64) (model.Shop, error)
	QueryShopByIdWithLogicExpire(ctx context.Context, id int64) (model.Shop, error)
	QueryShopByType(ctx context.Context, typeID, current int, x, y float64) ([]model.Shop, error)

	SetBloomFilter(filter *utils.BloomFilter)
}

// ShopLogicDeps 用于实例化 shopLogic 的依赖。
type ShopLogicDeps struct {
	Redis       *redisv9.Client
	DB          *gorm.DB
	BloomFilter *utils.BloomFilter
}

type shopLogic struct {
	redis          *redisv9.Client
	db             *gorm.DB
	distLock       *utils.DistributedLock
	bloomFilter    *utils.BloomFilter
	redisDataQueue chan int64
}

// NewShopLogic 构建店铺业务层。
func NewShopLogic(deps ShopLogicDeps) ShopLogic {
	redisCli := deps.Redis
	if redisCli == nil {
		redisCli = redisClient.GetRedisClient()
	}

	db := deps.DB
	if db == nil {
		db = mysql.GetMysqlDB()
	}

	l := &shopLogic{
		redis:          redisCli,
		db:             db,
		distLock:       utils.NewDistributedLock(redisCli),
		bloomFilter:    deps.BloomFilter,
		redisDataQueue: make(chan int64, maxRedisDataQueue),
	}

	// 启动缓存异步更新协程
	go l.syncUpdateCache()

	return l
}

func (s *shopLogic) SetBloomFilter(filter *utils.BloomFilter) {
	s.bloomFilter = filter
}

func (s *shopLogic) QueryShopById(_ context.Context, id int64) (model.Shop, error) {
	var shop model.Shop
	shop.Id = id
	err := shop.QueryShopById(id)
	if err != nil {
		return shop, fmt.Errorf("db query shop %d: %w", id, err)
	}
	return shop, nil
}

func (s *shopLogic) SaveShop(ctx context.Context, shop *model.Shop) error {
	if err := shop.SaveShop(); err != nil {
		logrus.Errorf("Failed to save shop to database: %v, shop data: %+v", err, shop)
		return fmt.Errorf("db save shop: %w", err)
	}

	// 数据库保存成功后，同步更新布隆过滤器
	if s.bloomFilter != nil && shop.Id > 0 {
		if err := s.bloomFilter.Add(shop.Id); err != nil {
			logrus.Warnf("Failed to add shop %d to Bloom Filter after save: %v", shop.Id, err)
		} else {
			logrus.Debugf("Successfully added shop %d to Bloom Filter", shop.Id)
		}
	}

	return nil
}

func (s *shopLogic) UpdateShop(ctx context.Context, shop *model.Shop) error {
	if err := shop.UpdateShop(mysql.GetMysqlDB()); err != nil {
		return fmt.Errorf("db update shop %d: %w", shop.Id, err)
	}
	return nil
}

func (s *shopLogic) QueryByType(typeId int, current int) ([]model.Shop, error) {
	var shopUtils model.Shop
	shops, err := shopUtils.QueryShopByType(typeId, current)
	if err != nil {
		return nil, fmt.Errorf("db query shop by type %d page %d: %w", typeId, current, err)
	}
	return shops, nil
}

func (s *shopLogic) QueryByName(ctx context.Context, name string, current int) ([]model.Shop, error) {
	var shopUtils model.Shop
	shops, err := shopUtils.QueryShopByName(name, current)
	if err != nil {
		return nil, fmt.Errorf("db query shop by name %s page %d: %w", name, current, err)
	}
	return shops, nil
}

// QueryShopByIdWithCache 如果缓存未命中，则查询数据库，将数据库结果写入缓存，并设置超时时间
func (s *shopLogic) QueryShopByIdWithCache(ctx context.Context, id int64) (model.Shop, error) {
	// 1. Check Bloom Filter
	if s.bloomFilter != nil {
		exists, err := s.bloomFilter.Contains(id)
		if err != nil {
			logrus.Warnf("BloomFilter check failed for shop %d: %v, proceeding to cache/DB", id, err)
		} else if !exists {
			return model.Shop{}, errors.New("shop not found (blocked by Bloom Filter)")
		}
	}

	redisKey := redisx.CACHE_SHOP_KEY + strconv.FormatInt(id, 10)

	shopInfo, err := s.redis.Get(ctx, redisKey).Result()
	if err == nil {
		var shop model.Shop
		err = json.Unmarshal([]byte(shopInfo), &shop)
		if err != nil {
			return model.Shop{}, fmt.Errorf("unmarshal shop cache %d: %w", id, err)
		}
		return shop, nil
	}

	if errors.Is(err, redisv9.Nil) {
		var shop model.Shop
		shop.Id = id
		err = shop.QueryShopById(id)
		if err != nil {
			return model.Shop{}, fmt.Errorf("db query shop %d: %w", id, err)
		}

		// 防御性编程：如果数据库查询成功，确保布隆过滤器中也存在
		if s.bloomFilter != nil && shop.Id > 0 {
			exists, bfErr := s.bloomFilter.Contains(shop.Id)
			if bfErr == nil && !exists {
				if addErr := s.bloomFilter.Add(shop.Id); addErr != nil {
					logrus.Warnf("Failed to add shop %d to Bloom Filter after DB query: %v", shop.Id, addErr)
				} else {
					logrus.Debugf("Defensively added shop %d to Bloom Filter after DB query", shop.Id)
				}
			}
		}

		redisValue, err := json.Marshal(shop)
		if err != nil {
			return model.Shop{}, fmt.Errorf("marshal shop %d: %w", id, err)
		}

		// 超时剔除策略
		err = s.redis.Set(ctx, redisKey, string(redisValue), time.Minute).Err()

		if err != nil {
			return model.Shop{}, fmt.Errorf("set shop cache %d: %w", id, err)
		}
		return shop, nil
	}

	return model.Shop{}, fmt.Errorf("get shop cache %d: %w", id, err)
}

// UpdateShopWithCacheCallBack 缓存更新的最佳实践方法
func (s *shopLogic) UpdateShopWithCacheCallBack(ctx context.Context, db *gorm.DB, shop *model.Shop) error {
	return db.Transaction(func(tx *gorm.DB) error {
		err := shop.QueryShopById(shop.Id)
		if err != nil {
			return fmt.Errorf("db query shop %d: %w", shop.Id, err)
		}

		// update the database
		err = shop.UpdateShop(tx)
		if err != nil {
			return fmt.Errorf("db update shop %d: %w", shop.Id, err)
		}

		// delete the cache
		redisKey := redisx.CACHE_SHOP_KEY + strconv.FormatInt(shop.Id, 10)
		err = s.redis.Del(ctx, redisKey).Err()

		if err != nil {
			return fmt.Errorf("del shop cache %d: %w", shop.Id, err)
		}

		return nil
	})
}

func (s *shopLogic) UpdateShopWithCache(ctx context.Context, shop *model.Shop) error {
	return s.UpdateShopWithCacheCallBack(ctx, mysql.GetMysqlDB(), shop)
}

// QueryShopByIdWithCacheNull 缓存穿透的解决方法: 缓存空对象，布隆过滤器已解决
func (s *shopLogic) QueryShopByIdWithCacheNull(ctx context.Context, id int64) (model.Shop, error) {
	// 1. Check Bloom Filter
	if s.bloomFilter != nil {
		exists, err := s.bloomFilter.Contains(id)
		if err != nil {
			logrus.Warnf("BloomFilter check failed for shop %d: %v, proceeding to cache/DB", id, err)
		} else if !exists {
			logrus.Infof("Bloom Filter blocked shop %d (not exists)", id)
			return model.Shop{}, errors.New("shop not found (blocked by Bloom Filter)")
		} else {
			logrus.Debugf("Bloom Filter passed for shop %d (exists)", id)
		}
	}

	redisKey := redisx.CACHE_SHOP_KEY + strconv.FormatInt(id, 10)

	shopInfoStr, err := s.redis.Get(ctx, redisKey).Result()

	if err == nil {
		var shopInfo model.Shop
		if shopInfoStr == "" {
			return model.Shop{}, nil
		}
		err = json.Unmarshal([]byte(shopInfoStr), &shopInfo)
		if err != nil {
			return model.Shop{}, fmt.Errorf("unmarshal shop cache %d: %w", id, err)
		}
		return shopInfo, nil
	}

	if errors.Is(err, redisv9.Nil) {
		var shopInfo model.Shop
		shopInfo.Id = id
		err = shopInfo.QueryShopById(id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 数据库不存在，设置空缓存防止缓存穿透
			err = s.redis.Set(ctx, redisKey, "", time.Minute).Err()
			if err != nil {
				return model.Shop{}, fmt.Errorf("set empty shop cache %d: %w", id, err)
			}
			return model.Shop{}, nil
		}

		if err != nil {
			return model.Shop{}, fmt.Errorf("db query shop %d: %w", id, err)
		}

		// 防御性编程：如果数据库查询成功，确保布隆过滤器中也存在
		if s.bloomFilter != nil && shopInfo.Id > 0 {
			exists, bfErr := s.bloomFilter.Contains(shopInfo.Id)
			if bfErr == nil && !exists {
				if addErr := s.bloomFilter.Add(shopInfo.Id); addErr != nil {
					logrus.Warnf("Failed to add shop %d to Bloom Filter after DB query: %v", shopInfo.Id, addErr)
				} else {
					logrus.Debugf("Defensively added shop %d to Bloom Filter after DB query", shopInfo.Id)
				}
			}
		}

		redisValue, err := json.Marshal(shopInfo)
		if err != nil {
			return model.Shop{}, fmt.Errorf("marshal shop %d: %w", id, err)
		}
		err = s.redis.Set(ctx, redisKey, string(redisValue), time.Minute).Err()
		if err != nil {
			return model.Shop{}, fmt.Errorf("set shop cache %d: %w", id, err)
		}
		return shopInfo, nil
	}
	return model.Shop{}, fmt.Errorf("get shop cache %d: %w", id, err)
}

// QueryShopByIdPassThrough 利用互斥锁解决热点 Key 问题(也就是缓存击穿问题)
func (s *shopLogic) QueryShopByIdPassThrough(ctx context.Context, id int64) (model.Shop, error) {
	redisKey := redisx.CACHE_SHOP_KEY + strconv.FormatInt(id, 10)

	for {
		shopInfoStr, err := s.redis.Get(ctx, redisKey).Result()
		if err == nil {
			if shopInfoStr == "" {
				return model.Shop{}, nil
			}

			var shopInfo model.Shop
			if err = json.Unmarshal([]byte(shopInfoStr), &shopInfo); err != nil {
				return model.Shop{}, fmt.Errorf("unmarshal shop cache %d: %w", id, err)
			}
			return shopInfo, nil
		}

		if !errors.Is(err, redisv9.Nil) {
			return model.Shop{}, fmt.Errorf("get shop cache %d: %w", id, err)
		}

		lockKey := redisx.CACHE_LOCK_KEY + strconv.FormatInt(id, 10)
		flag, token, lockErr := s.distLock.LockWithWatchDog(ctx, lockKey, 10*time.Second)
		if lockErr != nil {
			return model.Shop{}, fmt.Errorf("lock hot shop %d: %w", id, lockErr)
		}

		// 没有获取到锁，稍后重试
		if !flag {
			time.Sleep(time.Millisecond * 50)
			continue
		}

		// 重新建立缓存
		defer s.distLock.UnlockWithWatchDog(ctx, lockKey, token)
		var shopInfo model.Shop
		err = shopInfo.QueryShopById(id)

		if errors.Is(err, gorm.ErrRecordNotFound) {
			if setErr := s.redis.Set(ctx, redisKey, "", time.Minute).Err(); setErr != nil {
				return model.Shop{}, fmt.Errorf("set empty shop cache %d: %w", id, setErr)
			}
			return model.Shop{}, nil
		}

		if err != nil {
			return model.Shop{}, fmt.Errorf("db query shop %d: %w", id, err)
		}

		// 防御性编程：如果数据库查询成功，确保布隆过滤器中也存在
		if s.bloomFilter != nil && shopInfo.Id > 0 {
			exists, bfErr := s.bloomFilter.Contains(shopInfo.Id)
			if bfErr == nil && !exists {
				if addErr := s.bloomFilter.Add(shopInfo.Id); addErr != nil {
					logrus.Warnf("Failed to add shop %d to Bloom Filter after DB query: %v", shopInfo.Id, addErr)
				} else {
					logrus.Debugf("Defensively added shop %d to Bloom Filter after DB query", shopInfo.Id)
				}
			}
		}

		redisValue, err := json.Marshal(shopInfo)
		if err != nil {
			return model.Shop{}, fmt.Errorf("marshal shop %d: %w", id, err)
		}

		if err = s.redis.Set(ctx, redisKey, string(redisValue), time.Minute).Err(); err != nil {
			return model.Shop{}, fmt.Errorf("set shop cache %d: %w", id, err)
		}
		return shopInfo, nil
	}
}

// QueryShopByIdWithLogicExpire 逻辑过期方案
func (s *shopLogic) QueryShopByIdWithLogicExpire(ctx context.Context, id int64) (model.Shop, error) {
	redisKey := redisx.CACHE_SHOP_KEY + strconv.FormatInt(id, 10)

	redisDataStr, err := s.redis.Get(ctx, redisKey).Result()

	// hot key is in redis
	if errors.Is(err, redisv9.Nil) {
		return model.Shop{}, nil
	}

	if err == nil {
		if redisDataStr == "" {
			return model.Shop{}, nil
		}

		var redisData redisx.RedisData[model.Shop]
		err = json.Unmarshal([]byte(redisDataStr), &redisData)
		if err != nil {
			return model.Shop{}, fmt.Errorf("unmarshal logic-expire shop %d: %w", id, err)
		}

		if redisData.ExpireTime.After(time.Now()) {
			return redisData.Data, nil
		}
		// 否则过期,需要重新建立缓存

		lockKey := redisx.CACHE_LOCK_KEY + strconv.FormatInt(id, 10)
		baseCtx := context.Background()
		flag, token, lockErr := s.distLock.LockWithWatchDog(baseCtx, lockKey, 10*time.Second)
		if lockErr != nil {
			return model.Shop{}, fmt.Errorf("lock logic-expire shop %d: %w", id, lockErr)
		}

		// if not get the lock
		if !flag {
			return redisData.Data, nil
		}

		// if get the lock
		defer s.distLock.UnlockWithWatchDog(baseCtx, lockKey, token)
		s.redisDataQueue <- id
		return redisData.Data, nil
	}

	return model.Shop{}, fmt.Errorf("get logic-expire cache %d: %w", id, err)
}

func (s *shopLogic) syncUpdateCache() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for {
		id := <-s.redisDataQueue

		redisKey := redisx.CACHE_SHOP_KEY + strconv.FormatInt(id, 10)

		var shopInfo model.Shop
		err := shopInfo.QueryShopById(id)

		if err != nil {
			logrus.Warnf("syncUpdateCache query shop %d failed: %v", id, err)
			continue
		}

		var redisDataToSave redisx.RedisData[model.Shop]

		redisDataToSave.Data = shopInfo
		// the time of hot key exists
		redisDataToSave.ExpireTime = time.Now().Add(time.Second * redisx.HOT_KEY_EXISTS_TIME)

		redisValue, err := json.Marshal(redisDataToSave)
		if err != nil {
			logrus.Warnf("Failed to marshal shop data for shop %d: %v", id, err)
			continue
		}
		err = s.redis.Set(ctx, redisKey, string(redisValue), 0).Err()
		if err != nil {
			logrus.Warnf("Failed to set cache for shop %d: %v", id, err)
			continue
		}
	}
}

// QueryShopByType 查询店铺列表（支持地理位置排序）
func (s *shopLogic) QueryShopByType(ctx context.Context, typeID, current int, x, y float64) ([]model.Shop, error) {

	// 1. 无坐标，按类型分页查询
	if x == 0 || y == 0 {
		shops, err := new(model.Shop).QueryShopByType(typeID, current)
		if err != nil {
			return nil, fmt.Errorf("db query shop by type %d page %d: %w", typeID, current, err)
		}
		return shops, nil
	}

	const pageSize = redisx.DEFAULTPAGESIZE
	from := (current - 1) * pageSize
	to := current * pageSize // slice 上界（开区间）

	key := redisx.SHOP_GEO_KEY + strconv.Itoa(typeID)

	// 2. Redis GEO 查询
	query := &redisv9.GeoSearchLocationQuery{
		GeoSearchQuery: redisv9.GeoSearchQuery{
			Longitude:  x,
			Latitude:   y,
			Radius:     5000, // 5km
			RadiusUnit: "m",
			Sort:       "ASC",
			Count:      to, // 先取够 “当前页” 之前的全部数据
		},
		WithDist: true, // 让 Redis 返回距离
	}

	locs, err := s.redis.GeoSearchLocation(ctx, key, query).Result()
	if err != nil && !errors.Is(err, redisv9.Nil) {
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
		return nil, fmt.Errorf("db query shops by ids %v: %w", ids, err)
	}
	for i := range shops {
		shops[i].Distance = dist[shops[i].Id] // ⚠️ 字段是 Id
	}
	return shops, nil
}
