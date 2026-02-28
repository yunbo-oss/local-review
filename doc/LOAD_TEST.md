# local-review-go 压测方式与压测报告

## 一、压测方式

### 1.1 环境准备

| 步骤 | 命令 | 说明 |
|------|------|------|
| 1 | `docker compose up -d` 或 `docker compose -f docker-compose.yml -f docker-compose.distributed.yml up -d` | 单机依赖 / 分布式+Jaeger |
| 2 | `make seed` | 插入 MySQL 基础数据 |
| 3 | `make seed-load-test` | 151 用户 + 25 秒杀券 |
| 4 | `make seed-redis` | 初始化 Redis 库存 + 验证码 |
| 5 | `make seed-reset-load-test` | （重复压测）清空订单、恢复库存 |
| 6 | 新增券后 | `docker compose -f docker-compose.yml -f docker-compose.distributed.yml restart go-app-1 go-app-2 go-app-3` 刷新布隆过滤器 |

### 1.2 执行命令

```bash
# 标准压测（sleep 0.4，约 112 QPS）
make seed-reset-load-test && make load-test-seckill

# 全速压测
make seed-load-test && make seed-reset-load-test && \
docker compose -f docker-compose.yml -f docker-compose.distributed.yml restart go-app-1 go-app-2 go-app-3 && \
make load-test-seckill-max
```



## 二、压测报告（2025-02-28）

**环境**：1 Nginx + 3 Go 实例，151 用户 × 25 秒杀券，全速（NO_SLEEP=1）

| 指标 | 结果 |
|------|------|
| **总 QPS** | **~1160** |
| **成功购买 QPS** | **~58** |
| seckill_success | **3775**（理论最大，未发生少卖超卖） |
| 耗时 | ~65s |
| p(95) | 88ms ✓ |

**结论**：系统稳定；成功抢购走完整 Lua 预减 → 事务消息 → 消费者写 MySQL 流程。

---
