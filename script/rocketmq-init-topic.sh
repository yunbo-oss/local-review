#!/bin/bash
# 创建 RocketMQ 秒杀 Topic（RocketMQ 5.x 首次发送可能自动创建，若失败可手动执行）
# 需先 docker-compose up -d 启动 RocketMQ

docker exec local-review-rocketmq-broker sh mqadmin updateTopic -n rocketmq-namesrv:9876 -t seckill-orders -c DefaultCluster 2>/dev/null || echo "若 Topic 已存在或 Broker 未就绪，可忽略此错误"
