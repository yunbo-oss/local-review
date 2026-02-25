package rocketmq

import (
	"os"
	"strconv"
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
	// TopicOrderTimeout 订单超时关单 Topic（延迟消息）
	TopicOrderTimeout string
	// ProducerGroup 生产者组
	ProducerGroup string
	// ConsumerGroup 消费者组
	ConsumerGroup string
	// ProducerGroupOrderTimeout 订单超时生产者组
	ProducerGroupOrderTimeout string
	// ConsumerGroupOrderTimeout 订单超时消费者组
	ConsumerGroupOrderTimeout string
	// DelayTimeLevel 延迟级别，16=30min，4=30s（测试用）
	DelayTimeLevel int
)

func Init() {
	NameServerAddr = getEnv("ROCKETMQ_NAMESRV_ADDR", "127.0.0.1:9876")
	TopicSeckill = getEnv("ROCKETMQ_TOPIC_SECKILL", "seckill-orders")
	TopicOrderTimeout = getEnv("ROCKETMQ_TOPIC_ORDER_TIMEOUT", "order-timeout")
	ProducerGroup = getEnv("ROCKETMQ_PRODUCER_GROUP", "seckill-producer-group")
	ConsumerGroup = getEnv("ROCKETMQ_CONSUMER_GROUP", "seckill-consumer-group")
	ProducerGroupOrderTimeout = getEnv("ROCKETMQ_PRODUCER_GROUP_ORDER_TIMEOUT", "order-timeout-producer-group")
	ConsumerGroupOrderTimeout = getEnv("ROCKETMQ_CONSUMER_GROUP_ORDER_TIMEOUT", "order-timeout-consumer-group")
	DelayTimeLevel = getEnvInt("ROCKETMQ_DELAY_TIME_LEVEL", 16) // 16=30min，4=30s（测试用）
}

func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

// GetNameServerSlice 返回 NameServer 地址切片（支持多节点）
func GetNameServerSlice() []string {
	if NameServerAddr == "" {
		return []string{"127.0.0.1:9876"}
	}
	return strings.Split(NameServerAddr, ";")
}
