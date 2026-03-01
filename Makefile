.PHONY: run build test tidy clean air test-api seed seed-redis seed-load-test seed-reset-load-test seed-vector load-test-seckill

run:
	go run ./cmd/server

air:
	air

build:
	go build -o bin/local-review-go ./cmd/server

test:
	go test ./...

tidy:
	go mod tidy

clean:
	rm -rf bin/ tmp/

# 接口功能测试（需先启动服务，建议 make seed && make seed-redis）
test-api:
	chmod +x script/api-test.sh && ./script/api-test.sh

# 创建 RocketMQ 秒杀 Topic（首次启动 RocketMQ 后执行）
rocketmq-topic:
	./script/rocketmq-init-topic.sh

# 压测前：插入 MySQL 种子数据（需 Docker 中 MySQL 运行）
seed:
	docker exec -i local-review-mysql mysql -uroot -p8888.216 local_review_go < script/seed.sql

# 压测前：多用户 + 多秒杀券（需先 make seed）
seed-load-test:
	docker exec -i local-review-mysql mysql -uroot -p8888.216 local_review_go < script/seed-load-test.sql

# 压测前：重置订单和库存（清空 tb_voucher_order，恢复 MySQL/Redis 库存）
seed-reset-load-test:
	docker exec -i local-review-mysql mysql -uroot -p8888.216 local_review_go < script/seed-reset-load-test.sql
	$(MAKE) seed-redis

# 压测前：初始化 Redis 秒杀库存 + 测试用户验证码（需 Docker 中 Redis 运行）
seed-redis:
	chmod +x script/seed-redis.sh && ./script/seed-redis.sh

# RAG 智能点评：店铺向量化导入（需 Redis Stack + LLM_API_KEY + make seed）
seed-vector:
	go run ./cmd/seed-vector

# 测试 LLM API 是否可用（Embedding + Chat，仅需 LLM_API_KEY）
test-llm:
	go run ./cmd/test-llm

# 秒杀压测（多用户+多券，8G 内存推荐限流 50 QPS/实例）
load-test-seckill:
	k6 run -e BASE_URL=http://localhost:80 script/load-test-seckill.js

# 秒杀压测-全速（不设 sleep，测机器上限，需 docker-compose 中 SECKILL_RATE_LIMIT 调高）
load-test-seckill-max:
	k6 run -e BASE_URL=http://localhost:80 -e NO_SLEEP=1 script/load-test-seckill.js
