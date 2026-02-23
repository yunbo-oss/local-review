.PHONY: run build test tidy clean air test-api

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

# 接口冒烟测试（需先启动服务）
test-api:
	./script/api-test.sh

# 创建 RocketMQ 秒杀 Topic（首次启动 RocketMQ 后执行）
rocketmq-topic:
	./script/rocketmq-init-topic.sh
