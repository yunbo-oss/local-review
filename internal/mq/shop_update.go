package mq

import (
	"context"
	"encoding/json"
	"fmt"

	"local-review-go/internal/config/rocketmq"

	rmq "github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"
	"github.com/sirupsen/logrus"
)

// ShopUpdateMsg 店铺更新消息体（创建/更新时发送）
type ShopUpdateMsg struct {
	ShopID int64 `json:"shopId"`
}

// ShopUpdateProducer 店铺更新生产者
type ShopUpdateProducer struct {
	p rmq.Producer
}

// NewShopUpdateProducer 创建店铺更新生产者
func NewShopUpdateProducer() (*ShopUpdateProducer, error) {
	p, err := rmq.NewProducer(
		producer.WithGroupName(rocketmq.ProducerGroupShopUpdate),
		producer.WithNsResolver(primitive.NewPassthroughResolver(rocketmq.GetNameServerSlice())),
	)
	if err != nil {
		return nil, err
	}
	if err := p.Start(); err != nil {
		return nil, err
	}
	return &ShopUpdateProducer{p: p}, nil
}

// SendShopUpdate 发送店铺更新消息
func (s *ShopUpdateProducer) SendShopUpdate(ctx context.Context, shopID int64) error {
	body, err := json.Marshal(&ShopUpdateMsg{ShopID: shopID})
	if err != nil {
		return err
	}
	m := primitive.NewMessage(rocketmq.TopicShopUpdate, body)
	m.WithKeys([]string{fmt.Sprintf("shop:%d", shopID)})
	_, err = s.p.SendSync(ctx, m)
	if err != nil {
		logrus.Errorf("店铺更新消息发送失败 shopId=%d: %v", shopID, err)
		return err
	}
	return nil
}

// ShopUpdateCacheHandler 缓存消费者回调：删除 Redis 缓存
type ShopUpdateCacheHandler func(ctx context.Context, msg *ShopUpdateMsg) error

// StartShopUpdateCacheConsumer 启动店铺更新-缓存消费者（异步删缓存）
func StartShopUpdateCacheConsumer(handler ShopUpdateCacheHandler) error {
	c, err := consumer.NewPushConsumer(
		consumer.WithGroupName(rocketmq.ConsumerGroupShopUpdateCache),
		consumer.WithNsResolver(primitive.NewPassthroughResolver(rocketmq.GetNameServerSlice())),
		consumer.WithConsumerModel(consumer.Clustering),
		consumer.WithConsumeFromWhere(consumer.ConsumeFromFirstOffset),
	)
	if err != nil {
		return err
	}
	err = c.Subscribe(rocketmq.TopicShopUpdate, consumer.MessageSelector{}, func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		for _, msg := range msgs {
			var shopMsg ShopUpdateMsg
			if err := json.Unmarshal(msg.Body, &shopMsg); err != nil {
				logrus.Errorf("解析店铺更新消息失败: %v", err)
				return consumer.ConsumeRetryLater, err
			}
			if err := handler(ctx, &shopMsg); err != nil {
				logrus.Warnf("处理店铺缓存失效失败(shopId=%d): %v", shopMsg.ShopID, err)
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

// ShopUpdateRAGHandler RAG 向量消费者回调：更新 Redis 向量
type ShopUpdateRAGHandler func(ctx context.Context, msg *ShopUpdateMsg) error

// StartShopUpdateRAGConsumer 启动店铺更新-RAG 向量消费者
func StartShopUpdateRAGConsumer(handler ShopUpdateRAGHandler) error {
	c, err := consumer.NewPushConsumer(
		consumer.WithGroupName(rocketmq.ConsumerGroupShopUpdateRAG),
		consumer.WithNsResolver(primitive.NewPassthroughResolver(rocketmq.GetNameServerSlice())),
		consumer.WithConsumerModel(consumer.Clustering),
		consumer.WithConsumeFromWhere(consumer.ConsumeFromFirstOffset),
	)
	if err != nil {
		return err
	}
	err = c.Subscribe(rocketmq.TopicShopUpdate, consumer.MessageSelector{}, func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		for _, msg := range msgs {
			var shopMsg ShopUpdateMsg
			if err := json.Unmarshal(msg.Body, &shopMsg); err != nil {
				logrus.Errorf("解析店铺更新消息失败: %v", err)
				return consumer.ConsumeRetryLater, err
			}
			if err := handler(ctx, &shopMsg); err != nil {
				logrus.Warnf("处理店铺 RAG 向量更新失败(shopId=%d): %v", shopMsg.ShopID, err)
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
