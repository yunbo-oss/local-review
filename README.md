# Local Review Go

我在cursor的帮助下用GO重构并优化了黑马点评项目。

### 快速启动

```bash
# 依赖 MySQL、Redis、RocketMQ（可用 docker-compose up -d 启动）
cp .env.example .env   # 按需修改
./script/rocketmq-init-topic.sh  # 可选：预创建 Topic（RocketMQ 5.x 通常自动创建）
make run               # 或 go run ./cmd/server
# 访问 http://localhost:8088
```


项目采用 `cmd/` + `internal/` 目录结构，详见 [AGENTS.md](AGENTS.md)。

---

以下是计划和正在进行的改动说明（按推荐顺序）：

### 第一阶段：高并发缓存体系 (Cache & Consistency)

1.  **基于 Redis BitMap 的布隆过滤器**
    * **问题**：恶意请求查询不存在的 ID，直接穿透缓存击穿数据库。
    * **方案**：在 Redis 中构建 BitMap 布隆过滤器，请求到达前先校验。相比内存版，支持分布式共享；相比直接查库，性能提升显著。

2.  **多级缓存架构 (L1 Local + L2 Redis)**
    * **问题**：秒杀场景下，热点 Key（Hot Key）瞬间流量过大，导致 Redis 单节点网卡被打满。
    * **方案**：引入进程内缓存 (`go-cache`)。
    * **机制**：QPS 计数器检测热点。一旦发现热点 Key，自动提升至本地缓存（TTL 5s）。请求优先命中本地内存，大幅降低 Redis 集群压力。

### 第二阶段：高可靠异步架构 (Reliability & Async)

3.  **秒杀削峰填谷 (RocketMQ 改造)** ✅
    * **已实现**：事务消息（半消息 → Lua 预减 → Commit/Rollback）→ 消费者异步写 MySQL → 立即返回「排队中」。
    * 重试与死信由 RocketMQ 自带。

4.  **服务熔断与限流** ✅
    * **已实现**：基于 `golang.org/x/time/rate`，秒杀接口 QPS 限流（默认 1000，可配 `SECKILL_RATE_LIMIT`/`SECKILL_RATE_BURST`），超限返回 429。规划：Sentinel-Go 可扩展更多能力。

5.  **订单超时处理 (Delay Message)** ✅
    * **已实现**：RocketMQ 延迟消息 (Level 16 / 30min)。下单后投递 → 30 分钟后消费者回查支付状态 → 未支付则关单 + 回滚 Redis/MySQL。
    * 环境变量 `ROCKETMQ_DELAY_TIME_LEVEL=4` 可改为 30 秒延迟，便于测试。

6.  **秒杀防护增强** ✅
    * **唯一索引**：`tb_voucher_order (user_id, voucher_id)` 唯一约束，分布式锁失效时数据库兜底。
    * **秒杀券布隆过滤器**：启动预热 `bf:seckill-voucher`，防恶意请求不存在的 voucherId 穿透。

### 第三阶段：搜索与智能化 (Search & AI)

7.  **Elasticsearch 地理位置搜索**
    * **痛点**：MySQL `LIKE` 无法高效处理全文检索，`Distance` 计算无法利用索引。
    * **方案**：引入 Elasticsearch。
    * **同步策略**：采用**应用层双写**策略（DB 事务提交后异步写入 ES），保证数据基本一致。利用 ES 的 `Geo-Distance` 实现高性能的“附近商户”查询。

8.  **AI 智能点评助手 (RAG 实现)**
    * **功能**：集成 LLM 大模型。
    * **流程**：用户提问 -> ES 检索 Top5 相关店铺 -> 组装 Prompt -> AI 生成推荐建议。
    * **体验**：通过 SSE (Server-Sent Events) 实现流式输出，让点评回复具有“真人打字”般的即时感。

### 第四阶段（可选/后置）：分布式架构与可观测性

9.  **多实例部署与可观测性**
    * **目标**：单机 → 可水平扩展的分布式集群。
    * **要点**：
        * 多实例无状态部署，认证使用 JWT（无状态），无需 Session 存储。
        * RocketMQ 消费者组自动协调多实例消费，无需手动维护实例标识。
        * 避免进程内有状态设计，确保实例可随时扩缩容。
    * **OpenTelemetry**：Trace 全链路追踪、Metrics 对接 Prometheus、Logs 与 TraceID 关联。

---
