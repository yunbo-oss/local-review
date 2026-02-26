# 当前正在做什么 (activeContext)

> 本文档记录当前开发进度，有重大进展时请更新。

## 开发计划（来自 README，按推荐顺序）

### 第一阶段：分布式架构与可观测性（优先）

> **核心痛点**：单机 → 可水平扩展的分布式集群。

1. **Nginx + 多实例部署** ✅
   - 已实现：1 Nginx + 3 Go 实例，`least_conn` 负载均衡
   - 健康检查：`/health` 端点 + Nginx `max_fails=3 fail_timeout=30s` 被动健康检查
   - 日志：JSON 格式 + `instance_id`，便于集中收集
   - 可观测性：OpenTelemetry Trace（Jaeger OTLP），未配置 endpoint 时自动 noop
   - 配置一致性：`env_file` 统一 JWT_SECRET_KEY 等
   - 连接池：`MYSQL_MAX_OPEN_CONNS=30` 每实例，避免 3×100 超限
   - 可选：`docker-compose.observability.yml` 接入 Loki + Promtail 集中日志

### 第二阶段：高并发缓存体系 (Cache & Consistency)

2. **基于 Redis BitMap 的布隆过滤器** ✅
   - 店铺、秒杀券 ID 预热，防恶意请求穿透

### 第三阶段：高可靠异步架构 (Reliability & Async)

3. **秒杀削峰填谷 (RocketMQ 改造)** ✅
4. **服务熔断与限流** ✅
5. **订单超时处理 (Delay Message)** ✅
6. **秒杀防护增强** ✅（唯一索引、秒杀券布隆过滤器）

### 第四阶段：搜索与智能化 (Search & AI)

7. **AI 智能点评助手 (RAG)** 🔲
   - 流程：用户提问 → **Redis Vector** 检索 Top5 相关店铺 → 组装 Prompt → AI 生成推荐
   - 体验：SSE 流式输出
   - 注：已砍掉 Elasticsearch，改用 Redis Vector

---

## 近期完成：秒杀防护增强 ✅

- **唯一索引**：`tb_voucher_order (user_id, voucher_id)` 唯一约束
- **限流**：秒杀接口 QPS 限流（`golang.org/x/time/rate`），默认 1000 QPS，超限 429
- **秒杀券布隆过滤器**：`bf:seckill-voucher` 启动预热，AddSeckillVoucher 时同步加入
- **订单超时延迟消息**：30 分钟未支付自动关单 + 回滚 Redis/MySQL

---

## 近期完成：黑马点评前端适配 ✅

- **API 前缀**：所有接口统一挂载到 `/api`
- **静态文件**：Gin 托管 `front-end/`，访问 http://localhost:8088
- **新增接口**：`GET /user/:id`、`GET /blog/of/user?id=&current=`
- **Logout**：登出接口
- **上传路径**：`front-end/imgs`，删除时兼容 `/imgs` 前缀

---

## 已砍掉 / 不再规划

- **多级缓存 (L1 Local + L2 Redis)**：已砍掉
- **Elasticsearch**：已砍掉，RAG 改用 Redis Vector

---

*最后更新：请在有重大进展时更新此文件。*
