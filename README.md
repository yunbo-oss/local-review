# Local Review Go

用Go重写了黑马点评基本功能

我在cursor的帮助下用go重构并优化了这个点评项目。

### 快速启动

```bash
# 依赖 MySQL、Redis、RocketMQ（可用 docker-compose up -d 启动）
cp .env.example .env   # 按需修改
make run               # 或 go run ./cmd/server
# 访问 http://localhost:8088
```

秒杀功能需 RocketMQ，首次启动后可执行 `make rocketmq-topic` 创建 Topic（部分环境会自动创建）。

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
    * **已实现**：Redis Lua 预减 → 发送 RocketMQ → 立即返回「排队中」→ 消费者异步写 MySQL。
    * 重试与死信由 RocketMQ 自带。

4.  **服务熔断与限流 (Sentinel)**
    * **痛点**：秒杀瞬间流量超过 Go 服务端处理上限，导致 CPU 飙升甚至服务崩溃。
    * **方案**：集成 Sentinel-Go。
    * **策略**：针对秒杀接口配置 QPS 限流（如限制 1000 QPS）。超出阈值的请求直接返回 "系统繁忙"，保护下游服务不被压垮。

5.  **订单超时处理 (Delay Message)**
    * **原方案**：Cron 定时任务每分钟轮询全表，性能差且有延迟。
    * **新方案**：利用 RocketMQ 的 **延迟消息** (Level 16 / 30min)。
    * **流程**：下单后投递延迟消息。30分钟后消费者收到消息，回查支付状态。若未支付，则执行“关单+回滚库存”，实现精准且低负载的超时控制。

### 第三阶段：搜索与智能化 (Search & AI)

6.  **Elasticsearch 地理位置搜索**
    * **痛点**：MySQL `LIKE` 无法高效处理全文检索，`Distance` 计算无法利用索引。
    * **方案**：引入 Elasticsearch。
    * **同步策略**：采用**应用层双写**策略（DB 事务提交后异步写入 ES），保证数据基本一致。利用 ES 的 `Geo-Distance` 实现高性能的“附近商户”查询。

7.  **AI 智能点评助手 (RAG 实现)**
    * **功能**：集成 LLM 大模型。
    * **流程**：用户提问 -> ES 检索 Top5 相关店铺 -> 组装 Prompt -> AI 生成推荐建议。
    * **体验**：通过 SSE (Server-Sent Events) 实现流式输出，让点评回复具有“真人打字”般的即时感。

### 第四阶段（可选/后置）：分布式架构与可观测性

8.  **多实例部署与可观测性**
    * **目标**：单机 → 可水平扩展的分布式集群。
    * **要点**：
        * 多实例无状态部署，认证使用 JWT（无状态），无需 Session 存储。
        * RocketMQ 消费者组自动协调多实例消费，无需手动维护实例标识。
        * 避免进程内有状态设计，确保实例可随时扩缩容。
    * **OpenTelemetry**：Trace 全链路追踪、Metrics 对接 Prometheus、Logs 与 TraceID 关联。

---
