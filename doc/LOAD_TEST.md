# local-review-go 压测方式与压测报告

## 一、压测方式

### 1.1 环境准备

| 步骤 | 命令 | 说明 |
|------|------|------|
| 1 | `docker compose up -d` 或 `docker compose -f docker-compose.yml -f docker-compose.distributed.minimal.yml up -d` | 启动依赖与服务 |
| 2 | `make seed` | 插入 MySQL 基础数据（店铺、优惠券、秒杀券、1 个用户） |
| 3 | `make seed-load-test` | 多用户 50 个 + 秒杀券 10 个 |
| 4 | `make seed-redis` | 初始化 Redis 秒杀库存 + 所有用户验证码（123456） |
| 5 | `make seed-reset-load-test` | （重复压测时）清空订单、恢复库存 |
| 6 | 等待服务就绪 | 分布式部署约 50s，本地约 10s |

### 1.2 压测脚本

| 脚本 | 场景 | 默认配置 |
|------|------|----------|
| `script/load-test-seckill.js` | 秒杀（多用户+多券） | 100 QPS，60s |

### 1.3 执行命令

```bash
make load-test-seckill
# 或
k6 run -e BASE_URL=http://localhost:80 script/load-test-seckill.js

# 自定义参数
k6 run --vus 20 --duration 60s -e BASE_URL=http://localhost:80 script/load-test-seckill.js
```

### 1.4 限流配置（按内存调整）

| 内存 | SECKILL_RATE_LIMIT | SECKILL_RATE_BURST | 说明 |
|------|--------------------|--------------------|------|
| 8G | 50 | 80 | 每实例 50 QPS，3 实例约 150 总 QPS |
| 16G | 100 | 150 | 可适当提高 |
| 32G+ | 200 | 300 | 高并发场景 |

通过环境变量或 `docker-compose.distributed.minimal.yml` 的 `environment` 配置。

---

## 二、压测报告

### 2.1 多用户秒杀压测（2025-02-28）

**环境**：1 Nginx + 3 Go 实例，100 QPS，60s，51 用户 × 13 秒杀券

| 指标 | 结果 |
|------|------|
| 目标 QPS | 100（低于限流 150，便于成功走 MQ） |
| http_reqs | ~6,000（约 100/s） |
| seckill_success | 5–9（成功抢购，走完整 RocketMQ 异步下单） |
| http_req_duration p(95) | 10–13ms |
| 阈值 | p(95) < 3000ms ✓ |

**说明**：
- 之前 5,699 QPS 为总请求速率，大量 429 限流，成功抢购极少。
- 现用 `constant-arrival-rate` 控制 100 QPS，每个 VU 轮询 13 张券，分散抢购。
- http_req_failed 高为预期（400 已抢购、429 限流）。
- 重复压测前需 `make seed-reset-load-test` 清空订单并恢复库存。

**结论**：系统稳定；成功抢购走完整 Lua 预减 → 事务消息 → 消费者写 MySQL 流程。

---

## 三、注意事项

### 3.1 前置条件

- **种子数据**：压测前必须执行 `make seed`、`make seed-load-test`、`make seed-redis`，否则登录失败、秒杀券不存在。
- **验证码有效期**：`seed-redis` 设置的验证码 123456 有效期为 2 分钟，超时需重新执行。
- **布隆过滤器**：若先启动服务再执行 seed，需重启 Go 实例以刷新店铺 ID 布隆过滤器。

### 3.2 多用户压测

- **基础数据**：`make seed` 提供 1 个用户（13800138000）、3 个秒杀券（6/7/8）。
- **扩展数据**：`make seed-load-test` 增加 50 个用户（13800138001-50）、10 个秒杀券（9-18，每券库存 100）。
- **压测逻辑**：每个 VU 使用不同用户，随机选择 13 个秒杀券之一，模拟多用户并发抢购。

### 3.3 限流与登录

- **秒杀限流**：每实例独立计数，3 实例总限流约 150 QPS；超限返回 429。
- **登录限流**：每 IP 每分钟 5 次，高 VU 时部分 VU 可能无法获取 token，属预期现象。

### 3.4 结果解读

- **http_req_failed 高**：多为 400（已抢购）或 429（限流），属业务/限流正常。
- **p(95) 延迟**：超过 3000ms 需排查 Redis、MySQL、RocketMQ 等依赖。

### 3.5 常见问题

| 问题 | 可能原因 | 处理 |
|------|----------|------|
| 登录失败 | 验证码过期或未设置 | 执行 `make seed-redis` |
| 店铺 404 | 布隆过滤器未预热 | 先 `make seed` 再启动，或 seed 后重启 Go 实例 |
| 大量 429 | 超过限流阈值 | 正常，或调高 SECKILL_RATE_LIMIT |
| 大量 500 | 依赖异常 | 检查容器日志与依赖健康 |
