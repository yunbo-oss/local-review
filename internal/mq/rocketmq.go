package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"local-review-go/internal/config/rocketmq"
	"local-review-go/internal/model"
	"os"
	"strconv"

	rmq "github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

// SeckillOrderMsg 秒杀订单消息体（与 Lua 脚本约定一致）
type SeckillOrderMsg struct {
	UserId     int64 `json:"userId"`
	VoucherId  int64 `json:"voucherId"`
	OrderId    int64 `json:"id"`
}

// ToVoucherOrder 转为 model.VoucherOrder（仅填充必要字段，其余用默认值）
func (m *SeckillOrderMsg) ToVoucherOrder() model.VoucherOrder {
	return model.VoucherOrder{
		Id:        m.OrderId,
		UserId:    m.UserId,
		VoucherId: m.VoucherId,
		Status:    model.NOTPAYED,
	}
}

// SeckillTransactionListener 事务监听器：ExecuteLocalTransaction 执行 Lua 扣 Redis，CheckLocalTransaction 用于崩溃恢复
type SeckillTransactionListener struct {
	redis  *redis.Client
	script *redis.Script
}

// ExecuteLocalTransaction 半消息发送成功后执行：Lua 预减 Redis 库存，成功则 Commit，失败则 Rollback
func (l *SeckillTransactionListener) ExecuteLocalTransaction(msg *primitive.Message) primitive.LocalTransactionState {
	var orderMsg SeckillOrderMsg
	if err := json.Unmarshal(msg.Body, &orderMsg); err != nil {
		logrus.Errorf("事务消息解析失败: %v", err)
		return primitive.RollbackMessageState
	}
	ctx := context.Background()
	keys := []string{}
	values := []interface{}{
		strconv.FormatInt(orderMsg.VoucherId, 10),
		strconv.FormatInt(orderMsg.UserId, 10),
		strconv.FormatInt(orderMsg.OrderId, 10),
	}
	result, err := l.script.Run(ctx, l.redis, keys, values...).Result()
	if err != nil {
		logrus.Errorf("事务消息 Lua 执行失败: %v", err)
		return primitive.RollbackMessageState
	}
	r := result.(int64)
	if r == 0 {
		return primitive.CommitMessageState
	}
	return primitive.RollbackMessageState
}

// CheckLocalTransaction Broker 主动回查时调用（如 Producer 崩溃未返回 Commit/Rollback）：根据 Redis 状态判断
func (l *SeckillTransactionListener) CheckLocalTransaction(msg *primitive.MessageExt) primitive.LocalTransactionState {
	var orderMsg SeckillOrderMsg
	if err := json.Unmarshal(msg.Body, &orderMsg); err != nil {
		logrus.Errorf("回查消息解析失败: %v", err)
		return primitive.RollbackMessageState
	}
	orderKey := fmt.Sprintf("seckill:order:%d", orderMsg.VoucherId)
	ok, err := l.redis.SIsMember(context.Background(), orderKey, orderMsg.UserId).Result()
	if err != nil {
		logrus.Warnf("回查 Redis 失败: %v", err)
		return primitive.UnknowState
	}
	if ok {
		return primitive.CommitMessageState
	}
	return primitive.RollbackMessageState
}

// SeckillProducer 秒杀订单事务生产者，实现 logic.RocketMQProducer 接口
type SeckillProducer struct {
	tp rmq.TransactionProducer
}

// NewSeckillProducer 创建秒杀事务生产者（需传入 Redis 客户端和 Lua 脚本路径）
func NewSeckillProducer(redisClient *redis.Client, scriptPath string) (*SeckillProducer, error) {
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("读取 Lua 脚本失败: %w", err)
	}
	listener := &SeckillTransactionListener{
		redis:  redisClient,
		script: redis.NewScript(string(scriptBytes)),
	}
	tp, err := rmq.NewTransactionProducer(listener,
		producer.WithGroupName(rocketmq.ProducerGroup),
		producer.WithNsResolver(primitive.NewPassthroughResolver(rocketmq.GetNameServerSlice())),
	)
	if err != nil {
		return nil, err
	}
	if err := tp.Start(); err != nil {
		return nil, err
	}
	return &SeckillProducer{tp: tp}, nil
}

// SendSeckillOrder 发送秒杀订单事务消息：先发半消息，再在 ExecuteLocalTransaction 中执行 Lua
func (s *SeckillProducer) SendSeckillOrder(ctx context.Context, msg *SeckillOrderMsg) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	m := primitive.NewMessage(rocketmq.TopicSeckill, body)
	m.WithKeys([]string{fmt.Sprintf("order:%d", msg.OrderId)})
	res, err := s.tp.SendMessageInTransaction(ctx, m)
	if err != nil {
		logrus.Errorf("RocketMQ 事务消息发送失败: %v", err)
		return err
	}
	if res.State == primitive.RollbackMessageState {
		return fmt.Errorf("the condition is not meet")
	}
	if res.State == primitive.UnknowState {
		return fmt.Errorf("transaction state unknown, please retry")
	}
	return nil
}

// SeckillOrderHandler 秒杀订单消费回调，返回 nil 表示成功，非 nil 将触发 RocketMQ 重试
type SeckillOrderHandler func(ctx context.Context, msg *SeckillOrderMsg) error

// StartSeckillConsumer 启动秒杀订单消费者（阻塞调用，应在 goroutine 中运行）
func StartSeckillConsumer(handler SeckillOrderHandler) error {
	c, err := consumer.NewPushConsumer(
		consumer.WithGroupName(rocketmq.ConsumerGroup),
		consumer.WithNsResolver(primitive.NewPassthroughResolver(rocketmq.GetNameServerSlice())),
		consumer.WithConsumerModel(consumer.Clustering),
		consumer.WithConsumeFromWhere(consumer.ConsumeFromFirstOffset),
	)
	if err != nil {
		return err
	}
	err = c.Subscribe(rocketmq.TopicSeckill, consumer.MessageSelector{}, func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		for _, msg := range msgs {
			var orderMsg SeckillOrderMsg
			if err := json.Unmarshal(msg.Body, &orderMsg); err != nil {
				logrus.Errorf("解析秒杀订单消息失败: %v", err)
				return consumer.ConsumeRetryLater, err
			}
			if err := handler(ctx, &orderMsg); err != nil {
				logrus.Warnf("处理秒杀订单失败(orderId=%d): %v", orderMsg.OrderId, err)
				return consumer.ConsumeRetryLater, err
			}
		}
		return consumer.ConsumeSuccess, nil
	})
	if err != nil {
		return err
	}
	return c.Start()
}
