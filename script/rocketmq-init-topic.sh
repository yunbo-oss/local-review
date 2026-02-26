#!/bin/bash
# 预创建 RocketMQ Topic（RocketMQ 5.x 首次发送通常自动创建，若失败可手动执行）
# 需先 docker-compose up -d 启动 RocketMQ

BROKER="local-review-rocketmq-broker"
NS="rocketmq-namesrv:9876"
CLUSTER="DefaultCluster"

for topic in seckill-orders order-timeout; do
  docker exec $BROKER sh mqadmin updateTopic -n $NS -t $topic -c $CLUSTER 2>/dev/null || echo "Topic $topic: 已存在或 Broker 未就绪，可忽略"
done
