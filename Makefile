.PHONY: run build test tidy clean air test-api seed seed-redis seed-load-test seed-reset-load-test seed-vector load-test-seckill \
	generate-eval-data generate-challenge-data eval-router eval-router-challenge eval-rag eval-rag-smoke eval-rag-oracle eval-rag-prod eval-rag-prod-baseline \
	eval-hybrid-task eval-agent eval-agent-fake demo-agent docker-reset docker-up docker-verify docker-eval docker-challenge docker-demo \
	docker-challenge-v4 docker-challenge-v5 docker-challenge-v6 docker-agent-e2e-v61

run:
	go run ./cmd/server

air:
	air

build:
	go build -o bin/local-review-go ./cmd/server

test:
	go test ./... -count=1

tidy:
	go mod tidy

clean:
	rm -rf bin/ tmp/

# 接口功能测试（需先启动服务，建议 make seed && make seed-redis）
test-api:
	chmod +x script/api-test.sh && ./script/api-test.sh

# RAG 智能点评：展示（3 问题流式）
demo-rag:
	chmod +x script/rag.sh && ./script/rag.sh --demo

# RAG 一键初始化：seed + seed-redis + seed-vector + 后台启动服务
init-rag:
	chmod +x script/rag.sh && ./script/rag.sh --init

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

# RAG 智能点评：确定性本地向量导入（需 Redis Stack + MySQL seed）
seed-vector:
	go run ./cmd/seed-vector --reset --expected-count=200

# 固定种子生成 175 家新增店、955 条新增评论和 v2 goldens
generate-eval-data:
	go run ./cmd/generate-eval-data
	go run ./cmd/generate-eval-data --check

# 生成冻结 v3、可检查 v3.1/v6.1 regression，以及 v4/v5/v6 challenge；均不调用 LLM
generate-challenge-data:
	go run ./cmd/generate-challenge-data
	go run ./cmd/generate-challenge-data --check
	go run ./cmd/generate-challenge-data --suite=v31
	go run ./cmd/generate-challenge-data --suite=v31 --check
	go run ./cmd/generate-challenge-data --suite=v4
	go run ./cmd/generate-challenge-data --suite=v4 --check
	go run ./cmd/generate-challenge-data --suite=v5
	go run ./cmd/generate-challenge-data --suite=v5 --check
	go run ./cmd/generate-challenge-data --suite=v6
	go run ./cmd/generate-challenge-data --suite=v6 --check
	go run ./cmd/generate-challenge-data --suite=v61
	go run ./cmd/generate-challenge-data --suite=v61 --check

# 生产规则 Router 的确定性分类评测（不调用 LLM）
eval-router:
	go run ./cmd/eval-router --split=test --out=rag-evals/reports/router_v1.json

# 冻结 Router v2 challenge（不调用 LLM）
eval-router-challenge:
	go run ./cmd/eval-router --test-set=rag-evals/challenge/router.v2.json --split=challenge \
		--out=rag-evals/reports/router_challenge_v2.json

# RAG 检索评估（需 seed + seed-vector + LLM_API_KEY）
# script/rag-eval.json = SMOKE ONLY，非正式 baseline；正式集见 rag-evals/golden/
eval-rag:
	go run ./cmd/eval-rag

# Smoke：旧格式 ≤7 题，filter-mode=none，禁止写 baseline
eval-rag-smoke:
	go run ./cmd/eval-rag --test-set=script/rag-eval.json --filter-mode=none --retriever=hybrid

# Oracle filter + hybrid（正式 golden）
eval-rag-oracle:
	go run ./cmd/eval-rag --filter-mode=oracle --retriever=hybrid --split=test

# 生产口径：llm filter + hybrid（正式 golden）
eval-rag-prod:
	go run ./cmd/eval-rag --filter-mode=llm --retriever=hybrid --split=test

# 正式 Hybrid Retrieval 基线写入
eval-rag-prod-baseline:
	go run ./cmd/eval-rag --filter-mode=llm --retriever=hybrid --split=test --write-baseline \
		--baseline=rag-evals/baseline/hybrid_prod_v2.json \
		--out=rag-evals/reports/retrieval_prod_v2.json

# 在 Agent 的同一批任务/相同 trial 数上运行 Hybrid RAG
eval-hybrid-task:
	go run ./cmd/eval-agent --mode=inprocess --system=hybrid_rag --split=test \
		--test-set=rag-evals/golden/agent.v2.json \
		--out=rag-evals/baseline/hybrid_task_v2.json

# Agent 正式评测（需 LLM_API_KEY + Redis Stack + MySQL + seed-vector）
eval-agent:
	go run ./cmd/eval-agent --mode=inprocess --system=agent --split=test \
		--test-set=rag-evals/golden/agent.v2.json \
		--out=rag-evals/baseline/agent_prod_v2.json \
		--compare-baseline=rag-evals/baseline/hybrid_task_v2.json \
		--force-route=agent_multistep

# Agent harness 冒烟（不调 LLM；验证报告非 stub / trial 隔离）
eval-agent-fake:
	go run ./cmd/eval-agent --mode=fake --split=test --trials=3 \
		--test-set=rag-evals/golden/agent.v2.json \
		--out=rag-evals/reports/agent_latest.json \
		--force-route=agent_multistep

# 三轮记忆演示（需服务已启动 + seed-redis 验证码）
demo-agent:
	chmod +x script/agent-demo.sh && ./script/agent-demo.sh

# RAG 索引 schema 变更后：删除旧索引，再 make seed-vector 重新导入
drop-vector-index:
	chmod +x script/rag.sh && ./script/rag.sh --drop-index

# 测试 LLM API 是否可用（Embedding + Chat，仅需 LLM_API_KEY）
test-llm:
	go run ./cmd/test-llm

# --- Docker 可复现评测闭环 ---
# LLM_API_KEY 只从当前 shell 注入；不要写进 .env 或仓库。
docker-reset:
	docker compose --profile verify --profile eval --profile challenge --profile challenge-v4 --profile challenge-v5 --profile challenge-v6 --profile agent-e2e-v61 --profile demo down -v --remove-orphans

docker-up:
	docker compose up -d --build

docker-verify:
	docker compose --profile verify run --rm data-check
	docker compose --profile verify run --rm router-eval
	docker compose --profile verify run --rm router-challenge-eval
	docker compose --profile verify run --rm api-smoke

docker-eval:
	@test -n "$$LLM_API_KEY" || (echo "LLM_API_KEY is required" >&2; exit 1)
	docker compose --profile eval run --rm rag-eval
	docker compose --profile eval run --rm hybrid-task-eval
	docker compose --profile eval run --rm --no-deps agent-eval

# 代码与数据冻结后只运行一次；失败结果保留，不据此继续调 v3。
docker-challenge:
	@test -n "$$LLM_API_KEY" || (echo "LLM_API_KEY is required" >&2; exit 1)
	docker compose --profile challenge run --rm challenge-rag-eval
	docker compose --profile challenge run --rm challenge-hybrid-task-eval
	docker compose --profile challenge run --rm --no-deps challenge-agent-eval

# 修复只在 v3.1 回归集完成；冻结后 v4 只正式运行一次，不按逐题结果继续调参。
docker-challenge-v4:
	@test -n "$$LLM_API_KEY" || (echo "LLM_API_KEY is required" >&2; exit 1)
	docker compose --profile challenge-v4 run --rm challenge-v4-hybrid-task-eval
	docker compose --profile challenge-v4 run --rm --no-deps challenge-v4-agent-eval

# v5：统一 Query Understanding + Plan/Execute/Replan + claim grounding + layered memory。
docker-challenge-v5:
	@test -n "$$LLM_API_KEY" || (echo "LLM_API_KEY is required" >&2; exit 1)
	docker compose --profile challenge-v5 run --rm challenge-v5-hybrid-task-eval
	docker compose --profile challenge-v5 run --rm --no-deps challenge-v5-agent-eval

# v6：有界 Parallel ReAct + 评论游标续查 + Redis checkpoint + claim entailment。
docker-challenge-v6:
	@test -n "$$LLM_API_KEY" || (echo "LLM_API_KEY is required" >&2; exit 1)
	docker compose --profile challenge-v6 run --rm challenge-v6-hybrid-task-eval
	docker compose --profile challenge-v6 run --rm --no-deps challenge-v6-agent-eval

# v6.1：生产 Router -> Clarify / Hybrid RAG / Parallel ReAct 的完整入口评测；不跑对照组。
docker-agent-e2e-v61:
	@test -n "$$LLM_API_KEY" || (echo "LLM_API_KEY is required" >&2; exit 1)
	docker compose --profile agent-e2e-v61 run --rm agent-e2e-v61-eval

docker-demo:
	@test -n "$$LLM_API_KEY" || (echo "LLM_API_KEY is required" >&2; exit 1)
	docker compose --profile demo run --rm --no-deps redis-seed
	docker compose --profile demo run --rm --no-deps memory-demo-reset
	docker compose --profile demo run --rm --no-deps memory-demo

# 秒杀压测（多用户+多券，8G 内存推荐限流 50 QPS/实例）
load-test-seckill:
	k6 run -e BASE_URL=http://localhost:80 script/load-test-seckill.js

# 秒杀压测-全速（不设 sleep，测机器上限，需 docker-compose 中 SECKILL_RATE_LIMIT 调高）
load-test-seckill-max:
	k6 run -e BASE_URL=http://localhost:80 -e NO_SLEEP=1 script/load-test-seckill.js
