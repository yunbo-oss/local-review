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
	SetSeckillVoucherBloomFilter(bf *utils.BloomFilter)
}

type voucherOrderLogic struct {
	redis                    *redisConfig.Client
	voucherOrderRepo         repoInterfaces.VoucherOrderRepo
	seckillVoucherRepo      repoInterfaces.SeckillVoucherRepo
	producer                RocketMQProducer
	orderTimeoutProducer    OrderTimeoutProducer
	seckillVoucherBloomFilter *utils.BloomFilter
}

// RocketMQProducer 秒杀订单消息发送接口，便于测试时 mock
type RocketMQProducer interface {
	SendSeckillOrder(ctx context.Context, msg *mq.SeckillOrderMsg) error
}

// OrderTimeoutProducer 订单超时延迟消息发送接口，便于测试时 mock
type OrderTimeoutProducer interface {
	SendOrderTimeout(ctx context.Context, msg *mq.OrderTimeoutMsg) error
}

// VoucherOrderLogicDeps 用于实例化 voucherOrderLogic 的依赖
type VoucherOrderLogicDeps struct {
	VoucherOrderRepo       repoInterfaces.VoucherOrderRepo
	SeckillVoucherRepo     repoInterfaces.SeckillVoucherRepo
	Producer               RocketMQProducer
	OrderTimeoutProducer   OrderTimeoutProducer
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
		redis:                    redisClient.GetRedisClient(),
		voucherOrderRepo:         voucherOrderRepo,
		seckillVoucherRepo:       seckillVoucherRepo,
		producer:                 deps.Producer,
		orderTimeoutProducer:     deps.OrderTimeoutProducer,
		seckillVoucherBloomFilter: nil,
	}
}

func (l *voucherOrderLogic) SetSeckillVoucherBloomFilter(bf *utils.BloomFilter) {
	l.seckillVoucherBloomFilter = bf
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
	go func() {
		err := mq.StartOrderTimeoutConsumer(func(ctx context.Context, msg *mq.OrderTimeoutMsg) error {
			return l.HandleOrderTimeout(ctx, msg)
		})
		if err != nil {
			logrus.Errorf("RocketMQ 订单超时消费者启动失败: %v", err)
		}
	}()
}

func (l *voucherOrderLogic) SeckillVoucher(ctx context.Context, voucherID int64, userID int64) error {
	// 布隆过滤器防穿透：不存在的 voucherId 直接返回，避免击穿缓存/DB
	if l.seckillVoucherBloomFilter != nil {
		exists, err := l.seckillVoucherBloomFilter.Contains(voucherID)
		if err != nil {
			logrus.Warnf("秒杀券布隆过滤器校验失败 voucher=%d: %v", voucherID, err)
		} else if !exists {
			return errors.New("秒杀券不存在")
		}
	}

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

	if err := l.createVoucherOrder(ctx, order); err != nil {
		return err
	}
	// 下单成功后投递延迟消息，30 分钟后检查未支付则关单回滚
	if l.orderTimeoutProducer != nil {
		if err := l.orderTimeoutProducer.SendOrderTimeout(ctx, &mq.OrderTimeoutMsg{
			OrderId:   order.Id,
			UserId:    order.UserId,
			VoucherId: order.VoucherId,
		}); err != nil {
			logrus.Warnf("订单超时延迟消息投递失败 orderId=%d: %v", order.Id, err)
			// 不返回 err，订单已创建成功，仅记录日志；可后续通过定时任务补偿
		}
	}
	return nil
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

// HandleOrderTimeout 处理订单超时：未支付则关单 + 回滚 Redis + 回滚 MySQL
func (l *voucherOrderLogic) HandleOrderTimeout(ctx context.Context, msg *mq.OrderTimeoutMsg) error {
	order, err := l.voucherOrderRepo.GetByID(ctx, msg.OrderId)
	if err != nil {
		return fmt.Errorf("get order %d: %w", msg.OrderId, err)
	}
	if order.Status != model.NOTPAYED {
		// 已支付或已取消，无需处理
		return nil
	}

	// MySQL 事务：更新状态 + 回滚库存；仅当成功关单时才回滚 Redis
	var didCancel bool
	err = mysql.GetMysqlDB().Transaction(func(tx *gorm.DB) error {
		rows, err := l.voucherOrderRepo.UpdateStatus(ctx, msg.OrderId, model.NOTPAYED, model.CANCELED, tx)
		if err != nil {
			return fmt.Errorf("update order status: %w", err)
		}
		if rows == 0 {
			return nil
		}
		didCancel = true
		if err := l.seckillVoucherRepo.IncrStock(ctx, msg.VoucherId, tx); err != nil {
			return fmt.Errorf("rollback stock: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if !didCancel {
		return nil
	}

	// Redis 回滚：恢复库存 + 移除用户购买标记
	if err := mq.RunOrderTimeoutRollbackLua(ctx, l.redis, msg.VoucherId, msg.UserId); err != nil {
		logrus.Errorf("订单超时 Redis 回滚失败 orderId=%d: %v", msg.OrderId, err)
		return err
	}
	logrus.Infof("订单超时关单成功 orderId=%d voucherId=%d", msg.OrderId, msg.VoucherId)
	return nil
}
