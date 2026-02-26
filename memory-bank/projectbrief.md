# 项目简介 (projectbrief)

## local-review-go

用 Go 重写的点评类项目，从单机架构升级为可水平扩展的分布式架构。

### 主要功能

- 店铺管理
- 优惠券 / 秒杀
- 博客
- 关注
- UV 统计

### 当前技术栈

- Go 1.24+、Gin、GORM、MySQL、Redis、JWT、RocketMQ
- 布隆过滤器（店铺、秒杀券防穿透）
- 限流（`golang.org/x/time/rate`，秒杀接口）

### 规划中技术栈

- OpenTelemetry（Trace、Metrics、Logs 可观测性）
- Sentinel-Go（可扩展限流熔断能力）
- Elasticsearch（地理位置搜索）
- LLM + RAG（AI 智能点评）
