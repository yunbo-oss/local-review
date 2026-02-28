#!/bin/sh
# 将 ROCKETMQ_NAMESRV_ADDR 中的 hostname 解析为 IP（RocketMQ Go 客户端要求 IP:port 格式）
# 例如 rocketmq-namesrv:9876 -> 172.18.0.5:9876
if [ -n "$ROCKETMQ_NAMESRV_ADDR" ]; then
  addr="${ROCKETMQ_NAMESRV_ADDR%%:*}"
  port="${ROCKETMQ_NAMESRV_ADDR#*:}"
  if [ "$addr" = "$port" ]; then
    port="9876"
  fi
  # 优先从 /etc/hosts 解析（Docker Compose 部分环境会注入）
  ip=""
  if [ -f /etc/hosts ] && grep -q "$addr" /etc/hosts; then
    ip=$(grep "$addr" /etc/hosts | head -1 | awk '{print $1}')
  fi
  # 否则用 nslookup 通过 Docker 内置 DNS 解析（过滤掉 DNS 自身的 127.0.0.11:53）
  if [ -z "$ip" ] || [ "$ip" = "127.0.0.1" ]; then
    ip=$(nslookup "$addr" 2>/dev/null | awk '/Address:/ { print $2 }' | grep -v ':53' | tail -1)
  fi
  if [ -n "$ip" ] && [ "$ip" != "127.0.0.1" ]; then
    export ROCKETMQ_NAMESRV_ADDR="${ip}:${port}"
  fi
fi
exec "$@"
