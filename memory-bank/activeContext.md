# 当前正在做什么 (activeContext)

> 本文档记录当前开发进度，有重大进展时请更新。

## 开发计划（来自 README）

### 第零阶段：分布式架构与可观测性 (Distributed & Observability)

> **核心痛点**：支持多实例水平扩展、观测分布式链路。

1. **分布式架构改造** 🔲
   - 目标：单机 → 可水平扩展的分布式集群
   - 要点：多实例无状态、Session 存 Redis、Redis Stream 消费者带 `c1-{UUID}` 实例标识、避免进程内有状态

2. **OpenTelemetry 可观测性** 🔲
   - 痛点：分布式下请求跨多实例，难以定位问题与性能瓶颈
   - 方案：集成 OpenTelemetry（Trace、Metrics、Logs）
   - 能力：Trace 全链路追踪、Metrics 对接 Prometheus、Logs 与 TraceID 关联

### 第一阶段：高并发缓存体系 (Cache & Consistency)

> **核心痛点**：高并发下数据库不被击穿，且数据保持一致。

1. **基于 Redis BitMap 的布隆过滤器** ✅
   - 恶意请求查询不存在的 ID 会穿透缓存击穿数据库
   - 方案：Redis BitMap 布隆过滤器，请求到达前先校验；支持分布式共享，性能优于直接查库

2. **多级缓存架构 (L1 Local + L2 Redis)** 🔲
   - 秒杀场景下 Hot Key 瞬间流量过大，Redis 单节点网卡被打满
   - 方案：引入 `go-cache` 进程内缓存
   - 机制：QPS 计数器检测热点 → 热点 Key 自动提升至本地缓存（TTL 5s）→ 请求优先命中本地，降低 Redis 压力

### 第二阶段：高可靠异步架构 (Reliability & Async)

> **核心痛点**：秒杀高流量处理、订单超时处理。

3. **秒杀削峰填谷 (RocketMQ 改造)** 🔲
   - 原方案：同步下单，数据库并发写入压力大
   - 新方案：RocketMQ 异步
   - 流程：Redis Lua 预减库存 → 校验通过发 MQ，立即返回「排队中」→ 消费者可控速率写 MySQL

4. **服务熔断与限流 (Sentinel)** 🔲
   - 痛点：秒杀瞬间流量超限，CPU 飙升甚至崩溃
   - 方案：集成 Sentinel-Go，秒杀接口 QPS 限流（如 1000），超限返回「系统繁忙」

5. **订单超时处理 (Delay Message)** 🔲
   - 原方案：Cron 每分钟轮询全表，性能差且有延迟
   - 新方案：RocketMQ 延迟消息 (Level 16 / 30min)
   - 流程：下单后投递延迟消息 → 30 分钟后消费者回查支付状态 → 未支付则关单 + 回滚库存

### 第三阶段：搜索与智能化 (Search & AI)

> **核心痛点**：复杂查询需求与用户体验升级。

6. **Elasticsearch 地理位置搜索** 🔲
   - 痛点：MySQL `LIKE` 全文检索低效，`Distance` 无法利用索引
   - 方案：引入 ES，应用层双写（DB 事务提交后异步写 ES）
   - 能力：`Geo-Distance` 实现高性能「附近商户」查询

7. **AI 智能点评助手 (RAG)** 🔲
   - 功能：集成 LLM 大模型
   - 流程：用户提问 → ES 检索 Top5 相关店铺 → 组装 Prompt → AI 生成推荐
   - 体验：SSE 流式输出，「真人打字」般即时感

---

## 近期完成：黑马点评前端适配 ✅

- **API 前缀**：所有接口统一挂载到 `/api`，前端 baseURL 为 `/api`
- **静态文件**：Gin 直接托管 `front-end/`，访问 `http://localhost:8088` 即可使用前端
- **新增接口**：`GET /user/:id`（UserBrief）、`GET /blog/of/user?id=&current=`（other-info 用）
- **Logout**：实现登出接口，返回成功
- **上传路径**：`UPLOADPATH` 改为 `front-end/imgs`，删除时兼容前端传入的 `/imgs` 前缀
- **前端修复**：common.js 每次请求从 sessionStorage 读取 token；shop-detail 秒杀前检查 token
- **静态资源**：需从黑马点评原项目复制 `imgs/` 下的 add.png、bd.png、thumbup.png、icons/default-icon.png 等

---

*最后更新：请在有重大进展时更新此文件。*
