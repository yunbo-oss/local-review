package rocketmq

import (
	"os"
	"strings"
)

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

var (
	// NameServerAddr NameServer 地址，多个用分号分隔
	NameServerAddr string
	// TopicSeckill 秒杀订单 Topic
	TopicSeckill string
	// ProducerGroup 生产者组
	ProducerGroup string
	// ConsumerGroup 消费者组
	ConsumerGroup string
)

func Init() {
	NameServerAddr = getEnv("ROCKETMQ_NAMESRV_ADDR", "127.0.0.1:9876")
	TopicSeckill = getEnv("ROCKETMQ_TOPIC_SECKILL", "seckill-orders")
	ProducerGroup = getEnv("ROCKETMQ_PRODUCER_GROUP", "seckill-producer-group")
	ConsumerGroup = getEnv("ROCKETMQ_CONSUMER_GROUP", "seckill-consumer-group")
}

// GetNameServerSlice 返回 NameServer 地址切片（支持多节点）
func GetNameServerSlice() []string {
	if NameServerAddr == "" {
		return []string{"127.0.0.1:9876"}
	}
	return strings.Split(NameServerAddr, ";")
}
