# Local Review Go

基于 **Gin + Nginx** 的分布式点评/电商应用，涵盖用户鉴权、优惠券秒杀、商户检索、AI 智能搜索等核心模块。
秒杀模块参考了黑马点评的实现，在 Cursor 的帮助下使用 Go 进行了重构。AI 协作规范见 [.cursorrules](.cursorrules)、[AGENTS.md](AGENTS.md)、[memory-bank/](memory-bank/)。

**启动与测试**：详见 [doc/QUICKSTART.md](doc/QUICKSTART.md)。

---

## 一、分布式部署与基础设施

### 1.1 Nginx 负载均衡

- **配置**：`configs/nginx.conf`，upstream `go_backend` 指向 3 个 Go 实例
- **策略**：`least_conn` 最少连接
- **健康检查**：`/health` 端点，`max_fails=3`、`fail_timeout=30s` 被动健康检查，故障实例自动剔除
- **透传**：`X-Real-IP`、`X-Forwarded-For`、`Host` 等请求头透传

### 1.2 JWT 无状态认证

- **实现**：`internal/middleware/jwt.go`，`golang-jwt/jwt/v5`
- **Claims**：`CustomClaims` 含 `AuthUser`（id、nickName、icon）、`BufferTime`（缓冲期）、`RegisteredClaims`
- **Token 生命周期**：7 天有效，`TokenRefreshBuffer=30min` 内可刷新
- **多实例**：`JWT_SECRET_KEY` 通过 `env_file` 统一，保证各实例签发/校验一致
- **路由分组**：
  - `authGroup`：`middleware.AuthRequired()`，需登录
  - `publicGroup`：登录、验证码、热门博客等公开接口

---

## 二、高并发秒杀系统设计

### 2.1 整体流程

```
用户请求 POST /api/voucher-order/seckill/:id
    │
    ├─ 1. 令牌桶限流（中间件，超限 429）
    ├─ 2. 布隆过滤器校验 voucherId（不存在直接 404）
    ├─ 3. querySeckillVoucherById 查秒杀券
    ├─ 4. 校验秒杀时间（BeginTime/EndTime）
    ├─ 5. ensureSeckillStockInRedis（Redis 库存 key 存在则跳过，否则回填）
    ├─ 6. 发送 RocketMQ 事务消息（半消息）
    │       └─ ExecuteLocalTransaction：Lua 检查库存 + 防重复(SISMEMBER) + 预减
    │       └─ 成功 → Commit；失败 → Rollback
    ├─ 7. 立即返回「排队中」
    │
    └─ 消费者异步：lock:order:{userId} 分布式锁 → createVoucherOrder（HasPurchased + DecrStock + Create）
```

### 2.2 限流

- **秒杀接口**：`middleware.SeckillRateLimit()`，`golang.org/x/time/rate` 令牌桶，默认 1000 QPS、burst 2000，超限 429
- **登录/验证码**：按 IP 限流，`perIPRateLimit`，防暴力破解

### 2.3 Lua 脚本原子性

`script/voucher_script.lua` 在 Redis 内原子执行：

1. 检查 `seckill:stock:{voucherId}` 库存
2. 检查 `seckill:order:{voucherId}` 是否已含 userId（防重复）
3. `INCRBY stock -1`、`SADD order userId`

返回值：0 成功，1 库存不足/不存在，2 已购买。

**ensureSeckillStockInRedis 必要性**：Lua 要求 `seckill:stock:{voucherId}` 必须存在，否则直接返回 1 拒绝。key 可能因 Redis 重启、24h TTL 过期而缺失。该函数在 key 不存在时从 MySQL 回填，保证业务可恢复；使用分布式锁 `lock:rebuild:stock:{voucherId}` 防止多实例并发回填。

### 2.4 RocketMQ 事务消息

- **流程**：半消息 → `ExecuteLocalTransaction` 执行 Lua → 成功 Commit / 失败 Rollback
- **回查**：Producer 崩溃时，Broker 调用 `CheckLocalTransaction`，根据 `seckill:order:{voucherId}` 是否含 userId 判断 Commit/Rollback
- **Topic**：`seckill-orders`，消费者组 `seckill-consumer-group`

### 2.5 MySQL 乐观锁与唯一索引

- **DecrStock**：`UPDATE tb_seckill_voucher SET stock = stock - 1 WHERE voucher_id = ? AND stock > 0`，`stock > 0` 防止超卖
- **唯一索引**：`tb_voucher_order (user_id, voucher_id)` 兜底防重复下单
- **关单**：`UpdateStatus(id, NOTPAYED, CANCELED)` 条件更新，仅当状态为未支付时更新

### 2.6 订单超时处理

- **延迟消息**：下单时发送 `order-timeout` Topic，`DelayTimeLevel=16`（30 分钟）
- **消费者**：`HandleOrderTimeout` → `UpdateStatus` 关单 → MySQL `IncrStock` 回滚库存 → Lua 恢复 Redis 库存 + `SREM` 移除用户购买标记

### 2.7 压测结果

k6 压测，1 Nginx + 3 Go 实例，151 用户 × 25 秒杀券：总 QPS ~1160，无超卖少卖。详见 [doc/LOAD_TEST.md](doc/LOAD_TEST.md)。

---

## 三、缓存架构与高可用保障

### 3.1 Redis Key 设计

`pkg/utils/redisx/keys.go` 集中管理：

| Key 模式 | 用途 |
|----------|------|
| `cache:shop:{id}` | 店铺详情缓存 |
| `seckill:stock:{voucherId}` | 秒杀库存 |
| `seckill:order:{voucherId}` | 用户购买标记（Set） |
| `cache:seckill:voucher:{id}` | 秒杀券缓存 |
| `shop:lock:{id}` | 店铺缓存重建锁 |
| `lock:order:{userId}` | 秒杀订单创建锁（消费者防同一用户并发） |
| `lock:rebuild:stock:{voucherId}` | 秒杀库存回填锁（防多实例并发回填） |
| `bf:shop`、`bf:seckill-voucher` | 布隆过滤器 |
| `uv:{date}` | UV 统计（HyperLogLog） |
| `vec:shop:{id}` | RAG 店铺向量 Hash |

### 3.2 布隆过滤器

- **实现**：`pkg/utils/BloomFilter.go`，基于 Redis BitMap
- **预热**：启动时异步从 DB 加载店铺 ID、秒杀券 ID，批量 `AddBatch` 写入
- **使用**：店铺详情 `QueryShopByIdWithCacheNull`、秒杀 `SeckillVoucher` 前先 `Contains`，不存在直接返回

### 3.3 店铺缓存策略

- **当前使用**：`QueryShopByIdPassThrough`，Cache Aside + 布隆过滤器防穿透 + 分布式锁防击穿（缓存 miss 时仅一个请求查 DB 重建）
- **逻辑过期**：`QueryShopByIdWithLogicExpire` 已实现，缓存存 `RedisData{Data, ExpireTime}`，无物理 TTL；过期时抢锁，抢到则投递 `redisDataQueue` 异步重建，抢不到返回旧数据

### 3.4 秒杀券缓存

- **Key**：`cache:seckill:voucher:{id}`，TTL 5 分钟
- **回填**：`querySeckillVoucherById` 未命中时查 DB 并写入
- **库存回填**：`seckill:stock` key 不存在时，`singleflight` 防并发回填，从 MySQL 读取并 `SET`

### 3.5 分布式锁与 Watchdog

- **实现**：`pkg/utils/distributed_lock.go`
- **加锁**：`SET key token NX EX ttl`，成功则启动 Watchdog 协程
- **Watchdog**：每 `ttl/2` 执行 Lua 续期 `EXPIRE`，业务超时也不误删锁
- **解锁**：Lua 校验 token 后 `DEL`，保证仅持有者可释放

### 3.6 缓存一致性：MQ 异步删缓存

- **写路径**：`UpdateShopWithCache` → DB 更新 → 发 MQ `shop-update`（不同步删缓存）
- **消费者**：`shop-update-cache-consumer-group` 异步 `DEL cache:shop:{id}`
- **兜底**：MQ 发送失败时同步删缓存

### 3.7 店铺更新 MQ 双消费者

`shop-update` Topic 两个消费者组：

| 消费者组 | 职责 |
|----------|------|
| `shop-update-cache-consumer-group` | 异步删店铺缓存 |
| `shop-update-rag-consumer-group` | 异步 Embedding + 更新 Redis 向量 |

---

## 四、AI 语义检索引擎 (RAG)

### 4.1 背景

传统关键词匹配无法理解「适合情侣的浪漫餐厅」等语义，引入向量检索 + LLM 实现智能推荐。

### 4.2 技术方案

- **向量存储**：Redis Stack RediSearch，HNSW 索引，`idx:shop:vector`
- **Schema**：`internal/config/redis/vector.go`，Hash 前缀 `vec:shop:`，字段含 `name`、`type_name`、`area`、`text_content`、`avg_price`、`score`、`comments`、`sold`、`embedding`（VECTOR HNSW COSINE）
- **Embedding**：`internal/llm/client.go`，OpenAI 兼容 API（DeepSeek/智谱/通义等）

### 4.3 检索流程

```
用户提问
    │
    ├─ 1. LLM 意图解析：提取 area、typeName、maxPrice、minScore 等 JSON
    ├─ 2. Embedding API：问题转向量
    ├─ 3. VectorRepo.SearchShops：FT.SEARCH 预过滤 + KNN
    │       └─ 预过滤：TAG(area, type_name) + NUMERIC 范围
    │       └─ 语义阈值：MaxDistance 过滤 COSINE 距离过大的结果
    ├─ 4. 组装上下文：店铺信息 + 探店笔记（BlogRepo）
    └─ 5. LLM Chat：生成推荐，SSE 流式输出
```

### 4.4 数据同步

- **实时**：店铺创建/更新发 MQ → RAG 消费者 `NewShopUpdateRAGHandler` 异步 Embedding + `StoreShop`
- **离线**：`make seed-vector` 批量导入

---

## 五、其他功能模块

### 5.1 UV 统计

- **实现**：`middleware/uv.go`，`UVStatisticsMiddleware`
- **存储**：Redis `PFADD uv:{yyyyMMdd} {visitor}`，HyperLogLog 去重
- **标识**：已登录用 userId，未登录用 `IP|UserAgent`

### 5.2 博客与关注

- **博客**：发布、点赞（`blog:like:{id}` Set）、关注流分页
- **关注**：`follow:{userId}` Set，共同关注用 `SINTER`



---

## 六、目录结构

```
local-review-go/
├── cmd/
│   └── server/main.go           # 入口：依赖注入、路由、布隆过滤器预热、MQ 消费者
├── internal/
│   ├── config/                   # MySQL、Redis、RocketMQ、OTel、env
│   ├── handler/                  # HTTP 层（shop、user、voucher、blog、rag 等）
│   ├── logic/                    # 业务逻辑层
│   ├── repository/               # 数据访问（含 interface/）
│   ├── model/                    # GORM 实体
│   ├── middleware/               # JWT、UV、限流
│   ├── mq/                       # RocketMQ 生产者/消费者（秒杀、订单超时、店铺更新）
│   └── llm/                      # Embedding、Chat 客户端
├── pkg/
│   ├── httpx/                    # Result[T]、Ok/Fail、BindJSON
│   └── utils/                    # BloomFilter、DistributedLock、redisx
├── configs/nginx.conf            # Nginx 负载均衡
├── script/                       # Lua、seed、RAG、压测脚本
└── doc/                           # 文档
```

详见 [AGENTS.md](AGENTS.md)。
