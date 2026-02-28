#!/usr/bin/env bash
# 压测前初始化 Redis：秒杀库存 + 测试用户验证码
# 用法: ./script/seed-redis.sh 或 make seed-redis
# 依赖: Docker 中 local-review-redis 容器运行中
# 前置: make seed（基础数据），压测需 make seed-load-test（多用户+多券）

set -e
REDIS_CMD="docker exec local-review-redis redis-cli -a 8888.216"

echo "→ 清空 Redis 秒杀订单标记 (seckill:order:{voucherId})，否则 Lua 会误判已抢购"
for vid in 6 7 8 9 10 11 12 13 14 15 16 17 18; do
  $REDIS_CMD DEL "seckill:order:$vid" 2>/dev/null || true
done

echo "→ 初始化 Redis 秒杀库存 (seckill:stock:{voucherId})"
$REDIS_CMD SET seckill:stock:6 500 EX 86400 2>/dev/null || true
$REDIS_CMD SET seckill:stock:7 300 EX 86400 2>/dev/null || true
$REDIS_CMD SET seckill:stock:8 200 EX 86400 2>/dev/null || true
# 压测扩展券 9-18（需先 make seed-load-test）
for vid in 9 10 11 12 13 14 15 16 17 18; do
  $REDIS_CMD SET "seckill:stock:$vid" 100 EX 86400 2>/dev/null || true
done

echo "→ 设置测试用户验证码 (13800138000 + 13800138001-50 -> 123456, 2分钟有效)"
$REDIS_CMD SET login:code:13800138000 123456 EX 120 2>/dev/null || true
for i in $(seq 1 50); do
  phone=$(printf "138001380%02d" $i)
  $REDIS_CMD SET "login:code:$phone" 123456 EX 120 2>/dev/null || true
done

echo "✓ Redis 初始化完成"
