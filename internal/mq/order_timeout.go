package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"local-review-go/internal/config/rocketmq"
	"os"
	"strconv"

	rmq "github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

// OrderTimeoutMsg 订单超时消息体
type OrderTimeoutMsg struct {
	OrderId   int64 `json:"orderId"`
	UserId    int64 `json:"userId"`
	VoucherId int64 `json:"voucherId"`
}

// OrderTimeoutProducer 订单超时延迟消息生产者
type OrderTimeoutProducer struct {
	p rmq.Producer
}

// NewOrderTimeoutProducer 创建订单超时生产者（普通 Producer，用于发送延迟消息）
func NewOrderTimeoutProducer() (*OrderTimeoutProducer, error) {
	p, err := rmq.NewProducer(
		producer.WithGroupName(rocketmq.ProducerGroupOrderTimeout),
		producer.WithNsResolver(primitive.NewPassthroughResolver(rocketmq.GetNameServerSlice())),
	)
	if err != nil {
		return nil, err
	}
	if err := p.Start(); err != nil {
		return nil, err
	}
	return &OrderTimeoutProducer{p: p}, nil
}

// SendOrderTimeout 发送订单超时延迟消息（Level 16=30min，Level 4=30s 测试用）
func (o *OrderTimeoutProducer) SendOrderTimeout(ctx context.Context, msg *OrderTimeoutMsg) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	m := primitive.NewMessage(rocketmq.TopicOrderTimeout, body)
	m.WithKeys([]string{fmt.Sprintf("order:%d", msg.OrderId)})
	m.WithDelayTimeLevel(rocketmq.DelayTimeLevel)
	_, err = o.p.SendSync(ctx, m)
	if err != nil {
		logrus.Errorf("订单超时延迟消息发送失败 orderId=%d: %v", msg.OrderId, err)
		return err
	}
	return nil
}

// OrderTimeoutHandler 订单超时消费回调，返回 nil 表示成功
type OrderTimeoutHandler func(ctx context.Context, msg *OrderTimeoutMsg) error

// StartOrderTimeoutConsumer 启动订单超时消费者（阻塞调用，应在 goroutine 中运行）
func StartOrderTimeoutConsumer(handler OrderTimeoutHandler) error {
	c, err := consumer.NewPushConsumer(
		consumer.WithGroupName(rocketmq.ConsumerGroupOrderTimeout),
		consumer.WithNsResolver(primitive.NewPassthroughResolver(rocketmq.GetNameServerSlice())),
		consumer.WithConsumerModel(consumer.Clustering),
		consumer.WithConsumeFromWhere(consumer.ConsumeFromFirstOffset),
	)
	if err != nil {
		return err
	}
	err = c.Subscribe(rocketmq.TopicOrderTimeout, consumer.MessageSelector{}, func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		for _, msg := range msgs {
			var timeoutMsg OrderTimeoutMsg
			if err := json.Unmarshal(msg.Body, &timeoutMsg); err != nil {
				logrus.Errorf("解析订单超时消息失败: %v", err)
				return consumer.ConsumeRetryLater, err
			}
			if err := handler(ctx, &timeoutMsg); err != nil {
				logrus.Warnf("处理订单超时失败(orderId=%d): %v", timeoutMsg.OrderId, err)
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

// RunOrderTimeoutRollbackLua 执行 Redis 回滚 Lua 脚本
func RunOrderTimeoutRollbackLua(ctx context.Context, redisClient *redis.Client, voucherId, userId int64) error {
	scriptPath := "script/order_timeout_rollback.lua"
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("读取回滚 Lua 脚本失败: %w", err)
	}
	script := redis.NewScript(string(scriptBytes))
	keys := []string{}
	values := []interface{}{
		strconv.FormatInt(voucherId, 10),
		strconv.FormatInt(userId, 10),
	}
	_, err = script.Run(ctx, redisClient, keys, values...).Result()
	return err
}
