# AGENTS.md — local-review-go

本文档为 AI 编码助手提供项目上下文与开发规范，遵循 [AGENTS.md 开放格式](https://github.com/agentsmd/agents.md)。

---

## 1. 项目概述

**local-review-go** 是用 Go 重写的点评类项目，从单机架构升级为可水平扩展的分布式架构。主要功能包括：店铺管理、优惠券/秒杀、博客、关注、UV 统计等。

---

## 2. 技术栈

| 类别 | 技术 |
|------|------|
| 语言 | Go 1.24+ |
| Web 框架 | Gin |
| ORM | GORM |
| 数据库 | MySQL |
| 缓存 | Redis (go-redis/v9) |
| 认证 | JWT (golang-jwt/jwt/v5) |
| 日志 | logrus |
| 工具 | validator/v10, uuid, mapstructure, singleflight |

---

## 3. 目录结构

```
local-review-go/
├── main.go                 # 入口：初始化配置、依赖注入、路由、布隆过滤器预热
├── go.mod / go.sum
├── src/
│   ├── config/             # 配置层
│   │   ├── init.go         # 统一初始化入口
│   │   ├── env.go          # 环境变量读取 (GetEnv)
│   │   ├── mysql/init.go   # MySQL 连接池
│   │   └── redis/init.go   # Redis 连接
│   ├── handler/            # HTTP 层：参数解析、调用 logic、统一响应
│   │   ├── router.go       # 路由配置、中间件、Handlers 聚合
│   │   ├── shop.go, user.go, blog.go, voucher.go, ...
│   │   └── ...
│   ├── logic/              # 业务逻辑层：接口 + 实现
│   │   ├── shop_logic.go   # 店铺逻辑（含缓存、布隆过滤器）
│   │   ├── voucher_logic.go, voucher_order_logic.go, ...
│   │   └── ...
│   ├── repository/         # 数据访问层
│   │   ├── interface/      # Repository 接口定义（package interfaces）
│   │   │   ├── shop.go, user.go, blog.go, voucher.go, ...
│   │   │   └── ...
│   │   ├── shop_repo.go, user_repo.go, blog_repo.go, ...
│   │   └── ...
│   ├── model/              # 数据模型（仅实体定义）
│   │   ├── Shop.go, User.go, Blog.go, Voucher.go, ...
│   │   └── 含 TableName()、GORM 标签，无 DB 操作
│   ├── middleware/         # 中间件
│   │   ├── jwt.go          # JWT 解析、AuthRequired
│   │   ├── uv.go           # UV 统计
│   │   └── ...
│   ├── httpx/              # HTTP 通用工具
│   │   └── result.go       # Result[T]、Ok/Fail、BindJSON
│   └── utils/              # 通用工具
│       ├── BloomFilter.go  # Redis 分布式布隆过滤器
│       ├── distributed_lock.go
│       ├── Const.go
│       └── redisx/         # Redis key 常量、辅助函数
│           ├── keys.go
│           ├── regex.go, random.go, worker.go, ...
│           └── RedisData.go
```

---

## 4. 开发环境与启动

### 环境变量（优先使用，否则用默认值）

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `MYSQL_USER` | MySQL 用户 | root |
| `MYSQL_PASSWORD` | MySQL 密码 | 8888.216 |
| `MYSQL_ADDR` | MySQL 地址 | 127.0.0.1 |
| `MYSQL_PORT` | MySQL 端口 | 3306 |
| `MYSQL_DATABASE` | 数据库名 | local_review_go |
| `REDIS_ADDR` | Redis 地址 | 127.0.0.1 |
| `REDIS_PORT` | Redis 端口 | 6379 |
| `REDIS_PASSWORD` | Redis 密码 | 8888.216 |
| `JWT_SECRET_KEY` | JWT 密钥 | local-review-key-change-in-production（生产必须修改） |

### 启动命令

```bash
# 安装依赖
go mod tidy

# 启动服务（默认 :8088）
go run .
```

### 构建

```bash
go build -o local-review-go .
```

---

## 5. 代码风格与规范

### 5.1 分层与职责

调用链：`Handler → Logic → Repository（接口）→ Repository（实现）→ DB`

- **Handler**：只做参数解析、校验、调用 logic、返回 `httpx.Result[T]`，不写业务逻辑。
- **Logic**：定义 `XxxLogic` 接口，实现类为 `xxxLogic`；通过依赖注入获取 Repository、Redis、BloomFilter 等；不直接操作 DB。
- **Repository**：定义 `XxxRepo` 接口（在 `repository/interface/`），实现类在 `repository/`；封装所有数据访问，Logic 仅依赖接口。
- **Model**：GORM 实体 + `TableName()` + 表名常量；仅保留结构定义，不含任何 DB 操作。

### 5.2 统一响应格式

使用 `src/httpx/result.go` 中的泛型结构：

```go
// 成功
c.JSON(http.StatusOK, httpx.OkWithData(data))
c.JSON(http.StatusOK, httpx.OkWithList(list, total))
c.JSON(http.StatusOK, httpx.Ok[string]())

// 失败
c.JSON(http.StatusBadRequest, httpx.Fail[string]("错误信息"))
```

### 5.3 错误处理

- Handler 中：根据 `err` 类型或内容选择 HTTP 状态码（400/404/500）。
- Logic 中：使用 `fmt.Errorf("...: %w", err)` 包装错误，便于追踪。
- 敏感错误（如 DB 内部错误）对用户只返回通用提示，详细错误打日志。

### 5.4 命名约定

- 包名：小写单数，如 `handler`、`logic`、`model`、`repository`。
- 接口：`XxxLogic`、`XxxRepo`；实现：`xxxLogic`、`xxxRepo`。
- Handler 方法：动词开头，如 `QueryShopById`、`SaveShop`。
- Repository 接口：放在 `repository/interface/`，package 名为 `interfaces`（`interface` 为 Go 关键字）。
- Redis key：在 `utils/redisx/keys.go` 中集中定义常量。

### 5.5 依赖注入模式

```go
// main.go 中：先创建 Repo，再注入 Logic
shopRepo := repository.NewShopRepo(mysql.GetMysqlDB())
shopLogic := logic.NewShopLogic(logic.ShopLogicDeps{ShopRepo: shopRepo})
shopHandler := handler.NewShopHandler(shopLogic)
```

- **Repo**：在 main 中通过 `repository.NewXxxRepo(mysql.GetMysqlDB())` 创建。
- **Logic**：通过 `XxxLogicDeps` 注入 Repo；Deps 中某字段为 nil 时，构造函数内使用全局实例创建默认 Repo。
- **特殊依赖**：如 BloomFilter 通过 `SetBloomFilter` 等方法后置注入。

### 5.6 上下文传递

- Handler 从 `c.Request.Context()` 获取 `ctx`，并传递给 logic。
- 数据库操作、Redis 操作、外部调用均应使用 `ctx`，便于超时与链路追踪。

---

## 6. 测试

```bash
# 运行全部测试
go test ./...

# 运行指定包
go test ./src/utils/...
go test ./src/logic/...
```

- 新功能需补充单元测试。
- 使用 `*_test.go` 与包内测试，必要时使用 `testify` 等库。

---

## 7. 关键业务约定

### 7.1 布隆过滤器

- 店铺 ID 在启动时异步预热到 Redis 布隆过滤器。
- 查询店铺详情前先校验布隆过滤器，若判定不存在则直接返回 404，避免缓存穿透。

### 7.2 秒杀与 Redis Stream

- 秒杀库存使用 Redis 预扣减，订单通过 Redis Stream 异步消费。
- 消费者名称需带实例标识（如 UUID），以支持多实例部署。

### 7.3 认证与路由

- 需登录接口挂在 `authGroup` 下，使用 `middleware.AuthRequired()`。
- 公开接口（登录、验证码、热门博客等）挂在 `publicGroup` 下。

---

## 8. 常见注意点

- **不要**在 Handler 中直接操作 DB 或 Redis，应通过 logic 层。
- **不要**在 Logic 中直接操作 DB，应通过 Repository 层；Logic 依赖 `repoInterfaces.XxxRepo` 接口。
- **不要**在 Model 中编写 DB 操作，Model 仅保留实体定义和 `TableName()`。
- **不要**硬编码 Redis key，使用 `redisx` 包中的常量。
- **不要**在生产环境使用默认 `JWT_SECRET_KEY`，务必通过环境变量配置。
- **新增 Handler** 时，需在 `router.go` 的 `Handlers` 和 `ConfigRouter` 中注册，并在 `main.go` 中完成依赖注入。
- **新增业务领域** 时，按顺序：Model 实体 → `repository/interface/` 接口 → `repository/` 实现 → Logic 注入 Repo → main 中创建并注入。
- 修改 model 后，GORM `AutoMigrate` 会更新表结构，但复杂迁移需单独处理。

---

## 9. 参考资源

- [AGENTS.md 规范](https://github.com/agentsmd/agents.md)
- [Go 微服务 AGENTS.md 示例](https://agentsmd.net/agents-md-examples/go-microservices-backend-development-guide/)
