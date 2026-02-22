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

- **分布式优先**：多实例无状态、Session 存 Redis、消费者带实例标识
- **可观测性**：OpenTelemetry（Trace、Metrics、Logs）
- **依赖注入**：cmd/server/main.go 中创建 Repo → 注入 Logic → 创建 Handler
- **统一响应**：`httpx.Result[T]`、`OkWithData`、`Fail`
- **Redis key**：集中在 `pkg/utils/redisx/keys.go`
- **布隆过滤器**：店铺 ID 预热，防缓存穿透
- **秒杀（当前）**：Redis 预扣减 + Stream 异步消费
- **秒杀（规划）**：RocketMQ 削峰 + Redis Lua 预减 + Sentinel 限流 + 延迟消息超时关单

## 开发流程

Plan → Build → Test

## 协作约定（面向学习阶段开发者）

维护者处于学习阶段。**每次做代码改动时**，需解释清楚：

- **改了什么**：修改的文件和改动点
- **为什么这样改**：设计思路或问题背景
- **代码逻辑**：关键逻辑、调用关系、数据流（通俗语言，必要时分步说明）
- **如何理解**：易混淆概念（接口与实现、依赖注入、缓存策略等）的简要说明

以「第一次接触这段代码的人」的视角说明，帮助理解而非仅完成修改。
