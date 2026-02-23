package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"local-review-go/internal/config/mysql"
	redisClient "local-review-go/internal/config/redis"
	"local-review-go/internal/model"
	"local-review-go/internal/mq"
	"local-review-go/internal/repository"
	repoInterfaces "local-review-go/internal/repository/interface"
	"local-review-go/pkg/utils"
	"local-review-go/pkg/utils/redisx"
	"strconv"
	"time"

	redisConfig "github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type VoucherOrderLogic interface {
	SeckillVoucher(ctx context.Context, voucherID, userID int64) error
	StartConsumers()
}

type voucherOrderLogic struct {
	redis              *redisConfig.Client
	voucherOrderRepo   repoInterfaces.VoucherOrderRepo
	seckillVoucherRepo repoInterfaces.SeckillVoucherRepo
	producer           RocketMQProducer
}

// RocketMQProducer 秒杀订单消息发送接口，便于测试时 mock
type RocketMQProducer interface {
	SendSeckillOrder(ctx context.Context, msg *mq.SeckillOrderMsg) error
}

// VoucherOrderLogicDeps 用于实例化 voucherOrderLogic 的依赖
type VoucherOrderLogicDeps struct {
	VoucherOrderRepo   repoInterfaces.VoucherOrderRepo
	SeckillVoucherRepo repoInterfaces.SeckillVoucherRepo
	Producer           RocketMQProducer
}

func NewVoucherOrderLogic(deps VoucherOrderLogicDeps) VoucherOrderLogic {
	voucherOrderRepo := deps.VoucherOrderRepo
	if voucherOrderRepo == nil {
		voucherOrderRepo = repository.NewVoucherOrderRepo(mysql.GetMysqlDB())
	}
	seckillVoucherRepo := deps.SeckillVoucherRepo
	if seckillVoucherRepo == nil {
		seckillVoucherRepo = repository.NewSeckillVoucherRepo(mysql.GetMysqlDB())
	}
	return &voucherOrderLogic{
		redis:              redisClient.GetRedisClient(),
		voucherOrderRepo:   voucherOrderRepo,
		seckillVoucherRepo: seckillVoucherRepo,
		producer:           deps.Producer,
	}
}

func (l *voucherOrderLogic) StartConsumers() {
	go func() {
		err := mq.StartSeckillConsumer(func(ctx context.Context, msg *mq.SeckillOrderMsg) error {
			order := msg.ToVoucherOrder()
			return l.processOrder(ctx, order)
		})
		if err != nil {
			logrus.Errorf("RocketMQ 秒杀消费者启动失败: %v", err)
		}
	}()
}

func (l *voucherOrderLogic) SeckillVoucher(ctx context.Context, voucherID int64, userID int64) error {
	voucher, err := l.querySeckillVoucherById(ctx, voucherID)
	if err != nil {
		return fmt.Errorf("query seckill voucher %d: %w", voucherID, err)
	}
	now := time.Now()
	if now.Before(voucher.BeginTime) {
		return errors.New("秒杀尚未开始")
	}
	if now.After(voucher.EndTime) {
		return errors.New("秒杀已结束")
	}
	orderId, err := redisx.RedisWork.NextId("order")
	if err != nil {
		return fmt.Errorf("generate order id: %w", err)
	}

	// 事务消息：先发半消息，再在 ExecuteLocalTransaction 中执行 Lua
	// 保证「扣 Redis」与「发消息」原子性，避免 Redis 扣了但消息未发的情况
	if l.producer != nil {
		if err := l.producer.SendSeckillOrder(ctx, &mq.SeckillOrderMsg{
			UserId:    userID,
			VoucherId: voucherID,
			OrderId:   orderId,
		}); err != nil {
			return err
		}
	}
	return nil
}

// processOrder 处理订单（分布式锁 + 写库），供 RocketMQ 消费者调用
func (l *voucherOrderLogic) processOrder(ctx context.Context, order model.VoucherOrder) error {
	lockKey := fmt.Sprintf("lock:order:%d", order.UserId)
	lock := utils.NewDistributedLock(l.redis)

	lockCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	acquired, token, err := lock.LockWithWatchDog(lockCtx, lockKey, 10*time.Second)
	if err != nil || !acquired {
		if err != nil {
			return fmt.Errorf("lock order user=%d: %w", order.UserId, err)
		}
		return errors.New("系统繁忙，请重试")
	}
	defer lock.UnlockWithWatchDog(ctx, lockKey, token)

	return l.createVoucherOrder(ctx, order)
}

// createVoucherOrder 创建优惠券订单（事务：校验重复 + 扣库存 + 插入）
func (l *voucherOrderLogic) createVoucherOrder(ctx context.Context, order model.VoucherOrder) error {
	return mysql.GetMysqlDB().Transaction(func(tx *gorm.DB) error {
		purchasedFlag, err := l.voucherOrderRepo.HasPurchased(ctx, order.UserId, order.VoucherId, tx)
		if err != nil || purchasedFlag {
			if err != nil {
				return fmt.Errorf("check duplicate order user=%d voucher=%d: %w", order.UserId, order.VoucherId, err)
			}
			return model.ErrDuplicateOrder
		}

		if err := l.seckillVoucherRepo.DecrStock(ctx, order.VoucherId, tx); err != nil {
			return fmt.Errorf("decrease voucher stock %d: %w", order.VoucherId, err)
		}

		if err := l.voucherOrderRepo.Create(ctx, &order, tx); err != nil {
			return fmt.Errorf("create voucher order: %w", err)
		}
		return nil
	})
}

// querySeckillVoucherById 查询秒杀优惠券，优先走 Redis 缓存，减轻 MySQL 压力
func (l *voucherOrderLogic) querySeckillVoucherById(ctx context.Context, id int64) (model.SecKillVoucher, error) {
	redisKey := redisx.CACHE_SECKILL_VOUCHER_KEY + strconv.FormatInt(id, 10)
	cached, err := l.redis.Get(ctx, redisKey).Result()
	if err == nil {
		var sv model.SecKillVoucher
		if err := json.Unmarshal([]byte(cached), &sv); err != nil {
			logrus.Warnf("秒杀优惠券缓存解析失败 voucher=%d: %v", id, err)
		} else {
			return sv, nil
		}
	}

	// 缓存未命中，查 DB 并回填
	result, err := l.seckillVoucherRepo.GetByID(ctx, id)
	if err != nil {
		return model.SecKillVoucher{}, fmt.Errorf("db query seckill voucher %d: %w", id, err)
	}
	sv := *result
	if data, err := json.Marshal(sv); err == nil {
		ttl := time.Duration(redisx.CACHE_SECKILL_VOUCHER_TTL) * time.Second
		_ = l.redis.Set(ctx, redisKey, string(data), ttl).Err()
	}
	return sv, nil
}
