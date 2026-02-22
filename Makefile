.PHONY: run build test tidy clean

run:
	go run ./cmd/server

build:
	go build -o bin/local-review-go ./cmd/server

test:
	go test ./...

tidy:
	go mod tidy

clean:
	rm -rf bin/ tmp/
