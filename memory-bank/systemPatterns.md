# 系统架构 (systemPatterns)

## 分层架构

```
Handler → Logic → Repository（接口）→ Repository（实现）→ DB
```

- **Handler**：参数解析、校验、调用 logic、返回 `httpx.Result[T]`
- **Logic**：业务逻辑，依赖 Repository 接口，不直接操作 DB
- **Repository**：数据访问，接口在 `repository/interface/`，实现在 `repository/`
- **Model**：GORM 实体，仅定义，无 DB 操作

## 关键模式

- **依赖注入**：main.go 中创建 Repo → 注入 Logic → 创建 Handler
- **统一响应**：`httpx.Result[T]`、`OkWithData`、`Fail`
- **Redis key**：集中在 `utils/redisx/keys.go`
- **布隆过滤器**：店铺 ID 预热，防缓存穿透
- **秒杀**：Redis 预扣减 + Stream 异步消费

## 开发流程

Plan → Build → Test
