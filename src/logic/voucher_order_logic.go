package logic

import (
	"context"
	"errors"
	"fmt"
	"local-review-go/src/config/mysql"
	redisClient "local-review-go/src/config/redis"
	"local-review-go/src/model"
	"local-review-go/src/utils"
	"local-review-go/src/utils/redisx"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	redisConfig "github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// 最大重试次数配置
const (
	maxRetries = 3
	retryTTL   = 24 * time.Hour
)

type VoucherOrderLogic interface {
	SeckillVoucher(ctx context.Context, voucherID, userID int64) error
	StartConsumers()
}

type voucherOrderLogic struct {
	redis  *redisConfig.Client
	script *redisConfig.Script
}

func NewVoucherOrderLogic() VoucherOrderLogic {
	scriptBytes, err := os.ReadFile("script/voucher_script.lua")
	if err != nil {
		logrus.Errorf("读取秒杀脚本失败: %v", err)
	}
	return &voucherOrderLogic{
		redis:  redisClient.GetRedisClient(),
		script: redisConfig.NewScript(string(scriptBytes)),
	}
}

func (l *voucherOrderLogic) StartConsumers() {
	ctx := context.Background()
	_, err := l.redis.XGroupCreateMkStream(ctx, "stream.orders", "g1", "0").Result()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		logrus.Errorf("创建消费者组失败: %v", err)
	}

	go l.syncHandlerStream()
	go l.handlePendingList()
}

func (l *voucherOrderLogic) SeckillVoucher(ctx context.Context, voucherID int64, userID int64) error {
	voucher, err := l.querySeckillVoucherById(voucherID)
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

	keys := []string{}
	values := []interface{}{
		strconv.FormatInt(voucherID, 10),
		strconv.FormatInt(userID, 10),
		strconv.FormatInt(orderId, 10),
	}

	result, err := l.script.Run(ctx, l.redis, keys, values...).Result()
	if err != nil {
		return fmt.Errorf("run seckill script: %w", err)
	}

	r := result.(int64)
	if r != 0 {
		return errors.New("the condition is not meet")
	}
	return nil
}

// SyncHandlerStream 处理消息队列的goroutine
func (l *voucherOrderLogic) syncHandlerStream() {
	ctx := context.Background()
	for {
		msgs, err := l.redis.XReadGroup(ctx, &redisConfig.XReadGroupArgs{
			Group:    "g1",
			Consumer: "c1",
			Streams:  []string{"stream.orders", ">"},
			Count:    100,
			Block:    200 * time.Millisecond,
		}).Result()

		if err != nil {
			if errors.Is(err, redisConfig.Nil) {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			logrus.Errorf("XReadGroup error: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if len(msgs) == 0 || len(msgs[0].Messages) == 0 {
			continue
		}

		for _, msg := range msgs[0].Messages {
			if err := l.processVoucherMessage(msg); err != nil {
				logrus.Warnf("消息处理失败(ID:%s)，进入Pending List: %v", msg.ID, err)
			} else {
				if _, err := l.redis.XAck(ctx, "stream.orders", "g1", msg.ID).Result(); err != nil {
					logrus.Warnf("SyncHandler ACK失败: %v", err)
				}
			}
		}
	}
}

// 处理pending list中的消息（含重试逻辑）
func (l *voucherOrderLogic) handlePendingList() {
	ctx := context.Background()
	for {
		msgs, err := l.redis.XReadGroup(ctx, &redisConfig.XReadGroupArgs{
			Group:    "g1",
			Consumer: "c1",
			Streams:  []string{"stream.orders", "0"},
			Count:    50,
			Block:    5 * time.Second,
		}).Result()

		if err != nil {
			if errors.Is(err, redisConfig.Nil) {
				time.Sleep(1 * time.Second)
				continue
			}
			logrus.Errorf("PendingList XReadGroup error: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		if len(msgs) == 0 || len(msgs[0].Messages) == 0 {
			time.Sleep(1 * time.Second)
			continue
		}

		for _, msg := range msgs[0].Messages {
			retryCount := l.getRetryCount(ctx, msg.ID)

			if retryCount < maxRetries {
				if err := l.processVoucherMessage(msg); err != nil {
					l.incrRetryCount(ctx, msg.ID, retryCount)
					logrus.Warnf("Pending重试失败(ID:%s 重试%d次): %v",
						msg.ID, retryCount+1, err)
				} else {
					if _, ackErr := l.redis.XAck(ctx, "stream.orders", "g1", msg.ID).Result(); ackErr != nil {
						logrus.Warnf("PendingList ACK失败: %v", ackErr)
					}
					l.clearRetryCount(ctx, msg.ID)
				}
			} else {
				l.handleFailedMessage(msg, fmt.Errorf("达到最大重试次数%d", maxRetries))
				if _, ackErr := l.redis.XAck(ctx, "stream.orders", "g1", msg.ID).Result(); ackErr != nil {
					logrus.Warnf("死信消息ACK失败: %v", ackErr)
				}
				l.clearRetryCount(ctx, msg.ID)
			}
		}
	}
}

// 处理优惠券消息(使用自动看门狗的锁)
func (l *voucherOrderLogic) processVoucherMessage(msg redisConfig.XMessage) error {
	var order model.VoucherOrder
	if err := mapstructure.Decode(msg.Values, &order); err != nil {
		return fmt.Errorf("decode voucher order message %s: %w", msg.ID, err)
	}

	lockKey := fmt.Sprintf("lock:order:%d", order.UserId)
	lock := utils.NewDistributedLock(l.redis)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	acquired, token, err := lock.LockWithWatchDog(ctx, lockKey, 10*time.Second)
	if err != nil || !acquired {
		if err != nil {
			return fmt.Errorf("lock order user=%d: %w", order.UserId, err)
		}
		return errors.New("系统繁忙，请重试")
	}
	defer lock.UnlockWithWatchDog(ctx, lockKey, token)

	return createVoucherOrder(order)
}

// 创建优惠券订单
func createVoucherOrder(order model.VoucherOrder) error {
	return mysql.GetMysqlDB().Transaction(func(tx *gorm.DB) error {
		purchasedFlag, err := new(model.VoucherOrder).HasPurchasedVoucher(order.UserId, order.VoucherId, tx)
		if err != nil || purchasedFlag {
			if err != nil {
				return fmt.Errorf("check duplicate order user=%d voucher=%d: %w", order.UserId, order.VoucherId, err)
			}
			return model.ErrDuplicateOrder
		}

		var sv model.SecKillVoucher
		if err := sv.DecrVoucherStock(order.VoucherId, tx); err != nil {
			return fmt.Errorf("decrease voucher stock %d: %w", order.VoucherId, err)
		}

		order.CreateTime = time.Now()
		order.UpdateTime = time.Now()
		if err := order.CreateVoucherOrder(tx); err != nil {
			return fmt.Errorf("create voucher order: %w", err)
		}
		return nil
	})
}

// 获取消息重试次数
func (l *voucherOrderLogic) getRetryCount(ctx context.Context, msgID string) int {
	key := fmt.Sprintf("retry:stream.orders:%s", msgID)
	countStr, err := l.redis.Get(ctx, key).Result()
	if err != nil {
		if !errors.Is(err, redisConfig.Nil) {
			logrus.Warnf("获取重试次数失败(%s): %v", key, err)
		}
		return 0
	}
	count, _ := strconv.Atoi(countStr)
	return count
}

// 增加重试次数
func (l *voucherOrderLogic) incrRetryCount(ctx context.Context, msgID string, currentCount int) {
	key := fmt.Sprintf("retry:stream.orders:%s", msgID)
	newCount := currentCount + 1
	if err := l.redis.Set(ctx, key, newCount, retryTTL).Err(); err != nil {
		logrus.Errorf("设置重试次数失败(%s): %v", key, err)
	}
}

// 清除重试计数
func (l *voucherOrderLogic) clearRetryCount(ctx context.Context, msgID string) {
	key := fmt.Sprintf("retry:stream.orders:%s", msgID)
	if err := l.redis.Del(ctx, key).Err(); err != nil {
		logrus.Warnf("清除重试计数失败(%s): %v", key, err)
	}
}

// 处理失败消息
func (l *voucherOrderLogic) handleFailedMessage(msg redisConfig.XMessage, err error) {
	logrus.Warnf("消息处理失败(ID:%s): %v", msg.ID, err)

	ctx := context.Background()
	_, dlerr := l.redis.XAdd(ctx, &redisConfig.XAddArgs{
		Stream: "stream.orders.dead",
		Values: map[string]interface{}{
			"original_id": msg.ID,
			"values":      msg.Values,
			"error":       err.Error(),
			"time":        time.Now().Format(time.RFC3339),
		},
	}).Result()

	if dlerr != nil {
		logrus.Errorf("死信队列添加失败: %v", dlerr)
	}
}

func (l *voucherOrderLogic) querySeckillVoucherById(id int64) (model.SecKillVoucher, error) {
	var result model.SecKillVoucher
	if err := result.QuerySeckillVoucherById(id); err != nil {
		return result, fmt.Errorf("db query seckill voucher %d: %w", id, err)
	}
	return result, nil
}
