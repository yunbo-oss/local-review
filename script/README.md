# script 脚本说明

## 数据与压测

| 脚本 | 用法 | 说明 |
|------|------|------|
| `seed.sql` | `make seed` | 基础数据：25 店铺、优惠券、秒杀券、45 点评 |
| `seed-load-test.sql` | `make seed-load-test` | 压测扩展：151 用户、25 秒杀券 |
| `seed-reset-load-test.sql` | `make seed-reset-load-test` | 重置订单、恢复库存（含 seed-redis） |
| `seed-redis.sh` | `make seed-redis` | Redis 秒杀库存 + 验证码 123456 |
| `load-test-seckill.js` | `make load-test-seckill` | k6 秒杀压测 |

## RAG 智能点评

| 脚本 | 用法 | 说明 |
|------|------|------|
| `rag.sh` | `make init-rag` | 一键初始化：seed + seed-redis + seed-vector + 后台启动服务 |
| `rag.sh` | `make demo-rag` | 展示（3 问题流式） |
| `rag.sh` | `make drop-vector-index` | 删除向量索引（schema 变更后） |

## 运维

| 脚本 | 说明 |
|------|------|
| `rocketmq-init-topic.sh` | 首次启动 RocketMQ 后创建秒杀 Topic |
| `api-test.sh` | 接口冒烟测试 |
| `docker-entrypoint.sh` | Docker 容器入口（解析 RocketMQ 地址） |

## Lua（Redis）

| 脚本 | 说明 |
|------|------|
| `voucher_script.lua` | 秒杀预减库存 + 防重复购买 |
| `order_timeout_rollback.lua` | 订单超时回滚 Redis 库存 |
