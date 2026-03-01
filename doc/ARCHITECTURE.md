# 架构图 (Mermaid)

## 核心组件

| 层级 | 组件 | 说明 |
|------|------|------|
| **流量入口** | Nginx | 反向代理与负载均衡，upstream `go_backend` 指向 3 个 Go 实例，`least_conn` 策略 |
| **应用服务** | Go Cluster | 基于 Gin 框架，Handler → Logic → Repository 分层 |
| | Handler | HTTP 请求处理、参数校验、返回 `httpx.Result` |
| | Logic | 业务逻辑（秒杀资格、RAG 检索等） |
| | Repository | 接口与实现，操作 Redis / MySQL |
| **数据存储** | MySQL | 用户、店铺、优惠券、博客等关系型数据 |
| | Redis Stack | 缓存（店铺详情、分布式锁）、向量索引 `vec:shop`（RAG） |
| **消息队列** | RocketMQ | `seckill-orders` 削峰异步落库；`shop-update` 异步删缓存/更新向量；`order-timeout` 延迟关单 |
| **外部服务** | LLM API | DeepSeek/智谱等，RAG 意图解析与内容生成 |

## 关键数据流向

| 场景 | 链路 |
|------|------|
| **常规 REST** | Client → Nginx → Handler → Logic → Repository → Redis（缓存）/ MySQL |
| **秒杀** | Client → Nginx → Handler → Redis（Lua 预减+防重）→ RocketMQ → Consumer → MySQL |
| **RAG** | Client → Handler → LLM（意图解析）→ Redis（向量检索）→ LLM（生成）→ Client |

---

## 1. 系统部署架构

```mermaid
flowchart TB
    subgraph Client["客户端"]
        User[用户/前端]
    end

    subgraph Gateway["网关层"]
        Nginx[Nginx<br/>least_conn 负载均衡<br/>/health 健康检查]
    end

    subgraph App["应用层"]
        Go1[Go 实例 1]
        Go2[Go 实例 2]
        Go3[Go 实例 3]
    end

    subgraph Data["数据层"]
        MySQL[(MySQL)]
        Redis[(Redis<br/>+ Redis Stack)]
        MQ[RocketMQ]
    end

    User -->|:80| Nginx
    Nginx --> Go1
    Nginx --> Go2
    Nginx --> Go3
    Go1 & Go2 & Go3 --> MySQL
    Go1 & Go2 & Go3 --> Redis
    Go1 & Go2 & Go3 --> MQ
```

## 2. 应用分层架构

```mermaid
flowchart LR
    subgraph HTTP["HTTP 层"]
        Handler[Handler<br/>参数解析/校验]
    end

    subgraph Logic["业务逻辑层"]
        LogicLayer[Logic<br/>业务逻辑]
    end

    subgraph Data["数据访问层"]
        Repo[Repository<br/>接口+实现]
    end

    subgraph Storage["存储"]
        DB[(MySQL)]
        Cache[(Redis)]
    end

    Handler --> LogicLayer
    LogicLayer --> Repo
    Repo --> DB
    Repo --> Cache
```

## 3. 秒杀流程时序图

```mermaid
sequenceDiagram
    participant U as 用户
    participant N as Nginx
    participant G as Go 实例
    participant R as Redis
    participant MQ as RocketMQ
    participant DB as MySQL

    U->>N: POST /api/voucher-order/seckill/:id
    N->>G: 负载均衡
    G->>G: 1. 令牌桶限流
    alt 超限
        G-->>U: 429
    end
    G->>G: 2. 布隆过滤器校验
    alt 不存在
        G-->>U: 404
    end
    G->>R: 3. querySeckillVoucherById
    G->>G: 4. 校验秒杀时间
    G->>R: 5. ensureSeckillStockInRedis
    G->>MQ: 6. 发送事务消息(半消息)
    MQ->>R: ExecuteLocalTransaction: Lua 预减+防重复
    alt Lua 成功
        MQ->>MQ: Commit
    else Lua 失败
        MQ->>MQ: Rollback
    end
    G-->>U: 7. 排队中
    MQ->>G: 消费者拉取消息
    G->>R: lock:order:{userId}
    G->>DB: createVoucherOrder(HasPurchased+DecrStock+Create)
    G->>MQ: 发送 order-timeout 延迟消息
```

## 4. 店铺更新与缓存一致性

```mermaid
flowchart LR
    subgraph Write["写路径"]
        A[UpdateShop] --> B[DB 更新]
        B --> C[发 MQ shop-update]
    end

    subgraph MQ["RocketMQ"]
        Topic[shop-update Topic]
    end

    subgraph Consumer1["消费者组 1"]
        C1[shop-update-cache-consumer-group]
        C1 --> D1[DEL cache:shop:{id}]
    end

    subgraph Consumer2["消费者组 2"]
        C2[shop-update-rag-consumer-group]
        C2 --> D2[Embedding + StoreShop 向量]
    end

    C --> Topic
    Topic --> C1
    Topic --> C2
```

## 5. RAG 智能点评流程

```mermaid
flowchart TB
    subgraph Input["输入"]
        Q[用户提问]
    end

    subgraph RAG["RAG 流程"]
        L1[LLM 意图解析<br/>提取 area/typeName/maxPrice 等]
        L2[Embedding API<br/>问题转向量]
        L3[Redis Vector<br/>FT.SEARCH 预过滤+KNN]
        L4[组装上下文<br/>店铺+探店笔记]
        L5[LLM Chat<br/>SSE 流式输出]
    end

    subgraph Storage["存储"]
        Redis[(Redis Stack<br/>HNSW 向量索引)]
        Blog[BlogRepo]
    end

    Q --> L1
    L1 --> L2
    L2 --> L3
    L3 --> Redis
    L3 --> L4
    L4 --> Blog
    L4 --> L5
    L5 --> R[推荐回答]
```

## 6. 订单超时关单流程

```mermaid
sequenceDiagram
    participant P as 秒杀消费者
    participant MQ as RocketMQ
    participant DB as MySQL
    participant R as Redis

    P->>MQ: 下单成功后发送 order-timeout(30min)
    Note over MQ: 延迟消息 Level 16
    MQ->>P: 30 分钟后投递
    P->>DB: UpdateStatus(NOTPAYED→CANCELED)
    P->>DB: IncrStock 回滚库存
    P->>R: Lua: INCRBY stock +1, SREM order userId
```
