FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /app/server ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=builder /app/server .
COPY --from=builder /app/script ./script
COPY front-end ./front-end
RUN chmod +x /app/script/docker-entrypoint.sh
EXPOSE 8088
ENTRYPOINT ["/app/script/docker-entrypoint.sh"]
CMD ["./server"]
