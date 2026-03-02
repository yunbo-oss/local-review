# 启动与测试

## 一、本地开发（单实例）

```bash
# 1. 启动依赖（MySQL、Redis、RocketMQ）
docker compose up -d

# 2. 创建 .env 并安装依赖
cp .env.example .env
go mod tidy

# 3. 可选：预创建 RocketMQ Topic
./script/rocketmq-init-topic.sh

# 4. 可选：种子数据（功能测试/压测需执行）
make seed
make seed-redis

# 5. 启动服务
make run
# 或 go run ./cmd/server

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

- **依赖**：Redis Stack（docker-compose 已替换为 `redis-stack-server`）、`LLM_API_KEY`
- **向量导入**：`make seed-vector`（需先 `make seed`）
- **接口**：`POST /api/rag/chat` 需登录，支持 SSE 流式输出
- **展示**：`make demo-rag`（3 问题流式）

## 六、常用 Make 命令

| 命令 | 说明 |
|------|------|
| `make run` | 启动服务 |
| `make build` | 构建二进制 |
| `make test` | 运行单元测试 |
| `make test-api` | 接口冒烟测试 |
| `make seed` | 插入 MySQL 种子数据 |
| `make seed-load-test` | 151 用户 + 25 秒杀券 |
| `make seed-redis` | 初始化 Redis 秒杀库存 + 验证码 |
| `make seed-reset-load-test` | 重置订单和库存 |
| `make seed-vector` | RAG 店铺向量导入 |
| `make init-rag` | RAG 一键初始化 |
| `make demo-rag` | RAG 展示（流式） |
| `make load-test-seckill` | 秒杀压测 |
| `make load-test-seckill-max` | 秒杀压测（全速） |
