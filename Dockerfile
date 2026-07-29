FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server \
    && CGO_ENABLED=0 go build -o /out/seed-vector ./cmd/seed-vector \
    && CGO_ENABLED=0 go build -o /out/eval-rag ./cmd/eval-rag \
    && CGO_ENABLED=0 go build -o /out/eval-agent ./cmd/eval-agent \
    && CGO_ENABLED=0 go build -o /out/generate-eval-data ./cmd/generate-eval-data

FROM alpine:latest
RUN apk --no-cache add bash ca-certificates curl jq python3 redis tzdata
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=builder /out/ ./
COPY --from=builder /app/script ./script
COPY --from=builder /app/rag-evals ./rag-evals
COPY --from=builder /app/front-end ./front-end
RUN chmod +x /app/script/docker-entrypoint.sh
EXPOSE 8088
ENTRYPOINT ["/app/script/docker-entrypoint.sh"]
CMD ["./server"]
