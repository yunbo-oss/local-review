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

### 分布式部署（Docker）

```bash
# 1 个 Nginx + 3 个 Go 实例，Nginx 负载均衡
docker-compose -f docker-compose.yml -f docker-compose.distributed.yml up -d
# 访问 http://localhost:80（经 Nginx 转发）
```

项目采用 `cmd/` + `internal/` 目录结构，详见 [AGENTS.md](AGENTS.md)。

---

以下是计划和正在进行的改动说明（按推荐顺序）：

### 第一阶段：分布式架构与可观测性（优先）

1.  **Nginx + 多实例部署** 🔲
    * **目标**：1 个 Nginx + 3 个 Go 实例在 Docker 中启动，Nginx 做负载均衡。
    * **要点**：多实例无状态、JWT 无状态认证、RocketMQ 消费者组自动协调、避免进程内有状态。
    * **可观测性**：OpenTelemetry（Trace、Metrics、Logs）。

### 第二阶段：高并发缓存体系 (Cache & Consistency)

2.  **基于 Redis BitMap 的布隆过滤器** ✅
    * **已实现**：店铺、秒杀券 ID 预热，防恶意请求穿透缓存击穿数据库。

### 第三阶段：高可靠异步架构 (Reliability & Async)

3.  **秒杀削峰填谷 (RocketMQ 改造)** ✅
4.  **服务熔断与限流** ✅
5.  **订单超时处理 (Delay Message)** ✅
6.  **秒杀防护增强** ✅（唯一索引、秒杀券布隆过滤器）

### 第四阶段：搜索与智能化 (Search & AI)

7.  **AI 智能点评助手 (RAG 实现)** 🔲
    * **功能**：集成 LLM 大模型。
    * **流程**：用户提问 → **Redis Vector** 检索 Top5 相关店铺 → 组装 Prompt → AI 生成推荐建议。
    * **体验**：通过 SSE (Server-Sent Events) 实现流式输出，让点评回复具有「真人打字」般的即时感。

---
