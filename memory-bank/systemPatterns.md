# 系统架构 (systemPatterns)

## 分层架构

```
Handler → Logic → Repository（接口）→ Repository（实现）→ DB
```

- **Handler**：参数解析、校验、调用 logic、返回 `httpx.Result[T]`
- **Logic**：业务逻辑，依赖 Repository 接口，不直接操作 DB
- **Repository**：数据访问，接口在 `internal/repository/interface/`，实现在 `internal/repository/`
- **Model**：GORM 实体，仅定义，无 DB 操作

## 关键模式

- **分布式**：1 Nginx + 3 Go 实例（Docker），Nginx `least_conn` 负载均衡，多实例无状态、JWT 无状态认证、RocketMQ 消费者组自动协调
- **可观测性**：OpenTelemetry（Trace、Metrics、Logs）
- **依赖注入**：cmd/server/main.go 中创建 Repo → 注入 Logic → 创建 Handler
- **统一响应**：`httpx.Result[T]`、`OkWithData`、`Fail`
- **Redis key**：集中在 `pkg/utils/redisx/keys.go`
- **布隆过滤器**：店铺 ID、秒杀券 ID 预热，防缓存穿透
- **秒杀（当前）**：Redis Lua 预减 + RocketMQ 事务消息 + 异步消费 + 限流 + 订单超时延迟消息 + 唯一索引兜底

## 开发流程

Plan → Build → Test

## 协作约定（面向学习阶段开发者）

维护者处于学习阶段。**每次做代码改动时**，需解释清楚：

- **改了什么**：修改的文件和改动点
- **为什么这样改**：设计思路或问题背景
- **代码逻辑**：关键逻辑、调用关系、数据流（通俗语言，必要时分步说明）
- **如何理解**：易混淆概念（接口与实现、依赖注入、缓存策略等）的简要说明

以「第一次接触这段代码的人」的视角说明，帮助理解而非仅完成修改。
