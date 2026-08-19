# 启动与测试

## 一、本地开发（单实例）

```bash
# 1. 创建 .env 并安装依赖
cp .env.example .env
go mod tidy

# 2. 启动 PostgreSQL/pgvector、Redis、RocketMQ、迁移、种子和服务
docker compose up -d --build

# 3. 若只启动依赖并在宿主机调试服务
make run

# 访问 http://localhost:8088
```

## 二、分布式部署（1 Nginx + 3 Go 实例）

```bash
# 1. 创建 .env（保证 JWT_SECRET_KEY 等各实例一致）
cp .env.example .env

# 2. 启动分布式（1 Nginx + 3 Go + Jaeger）
docker compose -f docker-compose.yml -f docker-compose.distributed.yml up -d --build

# 3. 可选：预创建 RocketMQ Topic
./script/rocketmq-init-topic.sh

# 4. 可选：种子数据（压测需 seed + seed-load-test + seed-redis）
make seed
make seed-load-test
make seed-redis
# 若服务已启动再执行 seed，需重启 Go 实例以刷新布隆过滤器

# 访问 http://localhost:80（Nginx）| http://localhost:16686（Jaeger UI）
```

## 三、功能测试

```bash
# 接口冒烟测试（需服务已启动）
make test-api

# 指定 BASE_URL（分布式用 80）
./script/api-test.sh http://localhost:80
```

## 四、压测

```bash
# 压测前准备（需 make seed && make seed-load-test && make seed-redis）
make seed-reset-load-test   # 重置订单和库存

# 标准压测（sleep 0.4，约 112 QPS）
make load-test-seckill

# 全速压测（不设 sleep，测机器上限）
make load-test-seckill-max
```

压测方式与报告见 [doc/LOAD_TEST.md](LOAD_TEST.md)。

## 五、RAG 智能点评

- **检索存储**：PostgreSQL 17 + pgvector；全文 GIN 与向量 HNSW 索引由迁移任务创建
- **向量导入**：`make seed-vector`（正式评测应先生成 v2 数据并保证 PostgreSQL 共 200 家店）
- **模型**：本地向量化不需要 API key；生成回答和 Agent 需要 `LLM_API_KEY`
- **接口**：`POST /api/rag/chat` 需登录，支持 SSE 流式输出
- **展示**：`make demo-rag`（3 问题流式）

## 六、从空环境复现正式评测

不要把 API key 写入 `.env`。下面的命令会清空本项目 Compose volumes，自动完成迁移、固定数据、Redis、200 条 PostgreSQL 向量和 RocketMQ topic 初始化，再执行 API 校验、正式评测和三轮偏好 Demo。

```bash
make docker-reset
LLM_API_KEY='你的密钥' make docker-up
LLM_API_KEY='你的密钥' make docker-verify
LLM_API_KEY='你的密钥' make docker-eval
LLM_API_KEY='你的密钥' make docker-demo

# 冻结 v4 同任务 Agent / Hybrid 对照
LLM_API_KEY='你的密钥' make docker-challenge-v4
```

正式指标、报告口径和失败分析见 [AGENT_AND_EVAL.md](AGENT_AND_EVAL.md)；实践问题见 [EVAL_PRACTICE_LOG.md](EVAL_PRACTICE_LOG.md)。

## 七、常用 Make 命令

| 命令 | 说明 |
|------|------|
| `make run` | 启动服务 |
| `make build` | 构建二进制 |
| `make test` | 运行单元测试 |
| `make test-api` | 接口冒烟测试 |
| `make seed` | 插入 PostgreSQL 种子数据 |
| `make seed-load-test` | 151 用户 + 25 秒杀券 |
| `make seed-redis` | 初始化 Redis 秒杀库存 + 验证码 |
| `make seed-reset-load-test` | 重置订单和库存 |
| `make seed-vector` | 生成并持久化 PostgreSQL 检索文档/向量 |
| `make generate-eval-data` | 固定种子生成/校验 v2 数据和 golden |
| `make eval-router-challenge` | 运行冻结 Router v2 challenge（不调用模型） |
| `make docker-verify` | 校验数据、登录、API、RAG/Agent SSE 和引用 |
| `make docker-eval` | 运行三份正式 Retrieval/Hybrid/Agent 报告 |
| `make docker-challenge-v4` | 运行冻结 v4 的 Agent / 同任务 Hybrid challenge |
| `make docker-demo` | 运行三轮长期偏好 Demo |
| `make init-rag` | RAG 一键初始化 |
| `make demo-rag` | RAG 展示（流式） |
| `make load-test-seckill` | 秒杀压测 |
| `make load-test-seckill-max` | 秒杀压测（全速） |
