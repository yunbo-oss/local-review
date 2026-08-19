# Local Review Go

用 Go 实现的点评/电商后端，包含用户鉴权、店铺与博客、优惠券秒杀、缓存、消息队列、语义检索和推荐 Agent，并支持 Nginx 多实例部署。

**启动与测试**：详见 [doc/QUICKSTART.md](doc/QUICKSTART.md)。

---

## 一、分布式部署与基础设施

### 1.1 Nginx 负载均衡

- **配置**：`configs/nginx.conf`，upstream `go_backend` 指向 3 个 Go 实例
- **策略**：`least_conn` 最少连接
- **健康检查**：`/health` 端点，`max_fails=3`、`fail_timeout=30s` 被动健康检查，故障实例自动剔除
- **透传**：`X-Real-IP`、`X-Forwarded-For`、`Host` 等请求头透传

### 1.2 JWT 无状态认证

- **实现**：`internal/middleware/jwt.go`，`golang-jwt/jwt/v5`
- **Claims**：`CustomClaims` 含 `AuthUser`（id、nickName、icon）、`BufferTime`（缓冲期）、`RegisteredClaims`
- **Token 生命周期**：7 天有效，`TokenRefreshBuffer=30min` 内可刷新
- **多实例**：`JWT_SECRET_KEY` 通过 `env_file` 统一，保证各实例签发/校验一致
- **路由分组**：
  - `authGroup`：`middleware.AuthRequired()`，需登录
  - `publicGroup`：登录、验证码、热门博客等公开接口

---

## 二、高并发秒杀系统设计

### 2.1 整体流程

```
用户请求 POST /api/voucher-order/seckill/:id
    │
    ├─ 1. 令牌桶限流（中间件，超限 429）
    ├─ 2. 布隆过滤器校验 voucherId（不存在直接 404）
    ├─ 3. querySeckillVoucherById 查秒杀券
    ├─ 4. 校验秒杀时间（BeginTime/EndTime）
    ├─ 5. ensureSeckillStockInRedis（Redis 库存 key 存在则跳过，否则回填）
    ├─ 6. 发送 RocketMQ 事务消息（半消息）
    │       └─ ExecuteLocalTransaction：Lua 检查库存 + 防重复(SISMEMBER) + 预减
    │       └─ 成功 → Commit；失败 → Rollback
    ├─ 7. 立即返回「排队中」
    │
    └─ 消费者异步：lock:order:{userId} 分布式锁 → createVoucherOrder（HasPurchased + DecrStock + Create）
```

### 2.2 限流

- **秒杀接口**：`middleware.SeckillRateLimit()`，`golang.org/x/time/rate` 令牌桶，默认 1000 QPS、burst 2000，超限 429
- **登录/验证码**：按 IP 限流，`perIPRateLimit`，防暴力破解

### 2.3 Lua 脚本原子性

`script/voucher_script.lua` 在 Redis 内原子执行：

1. 检查 `seckill:stock:{voucherId}` 库存
2. 检查 `seckill:order:{voucherId}` 是否已含 userId（防重复）
3. `INCRBY stock -1`、`SADD order userId`

返回值：0 成功，1 库存不足/不存在，2 已购买。

**ensureSeckillStockInRedis 必要性**：Lua 要求 `seckill:stock:{voucherId}` 必须存在，否则直接返回 1 拒绝。key 可能因 Redis 重启、24h TTL 过期而缺失。该函数在 key 不存在时从 MySQL 回填，保证业务可恢复；使用分布式锁 `lock:rebuild:stock:{voucherId}` 防止多实例并发回填。

### 2.4 RocketMQ 事务消息

- **流程**：半消息 → `ExecuteLocalTransaction` 执行 Lua → 成功 Commit / 失败 Rollback
- **回查**：Producer 崩溃时，Broker 调用 `CheckLocalTransaction`，根据 `seckill:order:{voucherId}` 是否含 userId 判断 Commit/Rollback
- **Topic**：`seckill-orders`，消费者组 `seckill-consumer-group`

### 2.5 MySQL 乐观锁与唯一索引

- **DecrStock**：`UPDATE tb_seckill_voucher SET stock = stock - 1 WHERE voucher_id = ? AND stock > 0`，`stock > 0` 防止超卖
- **唯一索引**：`tb_voucher_order (user_id, voucher_id)` 兜底防重复下单
- **关单**：`UpdateStatus(id, NOTPAYED, CANCELED)` 条件更新，仅当状态为未支付时更新

### 2.6 订单超时处理

- **延迟消息**：下单时发送 `order-timeout` Topic，`DelayTimeLevel=16`（30 分钟）
- **消费者**：`HandleOrderTimeout` → `UpdateStatus` 关单 → MySQL `IncrStock` 回滚库存 → Lua 恢复 Redis 库存 + `SREM` 移除用户购买标记

### 2.7 压测结果

k6 压测，1 Nginx + 3 Go 实例，151 用户 × 25 秒杀券：总 QPS ~1160，无超卖少卖。详见 [doc/LOAD_TEST.md](doc/LOAD_TEST.md)。

---

## 三、缓存架构与高可用保障

### 3.1 Redis Key 设计

`pkg/utils/redisx/keys.go` 集中管理：

| Key 模式 | 用途 |
|----------|------|
| `cache:shop:{id}` | 店铺详情缓存 |
| `seckill:stock:{voucherId}` | 秒杀库存 |
| `seckill:order:{voucherId}` | 用户购买标记（Set） |
| `cache:seckill:voucher:{id}` | 秒杀券缓存 |
| `shop:lock:{id}` | 店铺缓存重建锁 |
| `lock:order:{userId}` | 秒杀订单创建锁（消费者防同一用户并发） |
| `lock:rebuild:stock:{voucherId}` | 秒杀库存回填锁（防多实例并发回填） |
| `bf:shop`、`bf:seckill-voucher` | 布隆过滤器 |
| `uv:{date}` | UV 统计（HyperLogLog） |
| `vec:shop:{id}` | RAG 店铺向量 Hash |

### 3.2 布隆过滤器

- **实现**：`pkg/utils/BloomFilter.go`，基于 Redis BitMap
- **预热**：启动时异步从 DB 加载店铺 ID、秒杀券 ID，批量 `AddBatch` 写入
- **使用**：店铺详情 `QueryShopByIdWithCacheNull`、秒杀 `SeckillVoucher` 前先 `Contains`，不存在直接返回

### 3.3 店铺缓存策略

- **当前使用**：`QueryShopByIdPassThrough`，Cache Aside + 布隆过滤器防穿透 + 分布式锁防击穿（缓存 miss 时仅一个请求查 DB 重建）
- **逻辑过期**：`QueryShopByIdWithLogicExpire` 已实现，缓存存 `RedisData{Data, ExpireTime}`，无物理 TTL；过期时抢锁，抢到则投递 `redisDataQueue` 异步重建，抢不到返回旧数据

### 3.4 秒杀券缓存

- **Key**：`cache:seckill:voucher:{id}`，TTL 5 分钟
- **回填**：`querySeckillVoucherById` 未命中时查 DB 并写入
- **库存回填**：`seckill:stock` key 不存在时，`singleflight` 防并发回填，从 MySQL 读取并 `SET`

### 3.5 分布式锁与 Watchdog

- **实现**：`pkg/utils/distributed_lock.go`
- **加锁**：`SET key token NX EX ttl`，成功则启动 Watchdog 协程
- **Watchdog**：每 `ttl/2` 执行 Lua 续期 `EXPIRE`，业务超时也不误删锁
- **解锁**：Lua 校验 token 后 `DEL`，保证仅持有者可释放

### 3.6 缓存一致性：MQ 异步删缓存

- **写路径**：`UpdateShopWithCache` → DB 更新 → 发 MQ `shop-update`（不同步删缓存）
- **消费者**：`shop-update-cache-consumer-group` 异步 `DEL cache:shop:{id}`
- **兜底**：MQ 发送失败时同步删缓存

### 3.7 店铺更新 MQ 双消费者

`shop-update` Topic 两个消费者组：

| 消费者组 | 职责 |
|----------|------|
| `shop-update-cache-consumer-group` | 异步删店铺缓存 |
| `shop-update-rag-consumer-group` | 异步 Embedding + 更新 Redis 向量 |

---

## 四、AI 语义检索引擎 (RAG) 与推荐 Agent

> 暂未实现前端。这里的 ReAct 指 Reason/Act 的有界 Agent 运行时，不是 React.js。RAG 演示：`make demo-rag`；Agent 记忆演示：`make demo-agent`。说明见 [doc/AGENT_AND_EVAL.md](doc/AGENT_AND_EVAL.md)。

### 4.0 推荐入口

| 接口 | 说明 |
|------|------|
| `POST /api/rag/chat` | Hybrid RAG oneshot（SSE） |
| `POST /api/agent/recommend` | 有界多步 Agent（需登录；`force_route` 可选） |
| `POST /api/recommend` | 统一入口：`RecommendRouter` → RAG / Agent |

当前架构的端到端评测从生产 Router 入口开始，不强制指定 Agent 路由，也不运行对照组：

```bash
LLM_API_KEY='your-key' make docker-agent-e2e-v61
```

该命令实际执行 `Query Understanding -> Router -> Clarify / Hybrid RAG / Parallel ReAct -> Answer Verifier`，报告输出到 `rag-evals/reports/agent_e2e_v61.json`。

### 4.1 背景

传统关键词匹配无法理解「适合情侣的浪漫餐厅」等语义，引入向量检索 + LLM 实现智能推荐。

### 4.2 技术方案

- **向量存储**：Redis Stack RediSearch，HNSW 索引，`idx:shop:vector`
- **Schema**：`internal/config/redis/vector.go`，Hash 前缀 `vec:shop:`，字段含 `name`、`type_name`、`area`、`text_content`、`avg_price`、`score`、`comments`、`sold`、`embedding`（VECTOR HNSW COSINE）
- **Embedding**：默认 `local-feature-hash-zh-v2`（384 维确定性本地 embedding，便于离线复现）；也保留 OpenAI-compatible embedding provider 配置

### 4.3 检索流程

```
用户提问
    │
    ├─ 1. LLM 意图解析：提取 area、typeName、maxPrice、minScore 等 JSON
    ├─ 2. 确定性本地 embedding：问题转为 384 维向量
    ├─ 3. ShopSearchLogic：RediSearch TEXT + KNN，RRF 融合
    │       └─ 预过滤：TAG(area, type_name) + NUMERIC 范围
    │       └─ 语义阈值：MaxDistance 过滤 COSINE 距离过大的结果
    ├─ 4. 组装上下文：店铺信息 + 探店笔记（BlogRepo）
    └─ 5. LLM Chat：生成推荐，SSE 流式输出
```

### 4.4 数据同步

- **实时**：店铺创建/更新发 MQ → RAG 消费者 `NewShopUpdateRAGHandler` 异步 Embedding + `StoreShop`
- **离线**：`make seed-vector` 批量导入

### 4.5 当前 Agent 架构

```mermaid
flowchart TD
    A["POST /api/recommend"] --> B["鉴权、输入与状态预检"]
    B --> C["LLM Query Understanding<br/>意图、改写、硬条件、软偏好、证据需求、置信度"]
    C --> D{"Adaptive Router"}
    D -->|"低置信度、缺少指代"| E["Clarify"]
    D -->|"简单且自包含"| F["Hybrid RAG one-shot"]
    D -->|"比较、核验、追问、记忆"| G["选择性 Memory Context"]
    G --> H["Bounded Parallel ReAct Runtime"]
    H --> I["Controller 输出 act / finish / clarify"]
    I -->|"act"| J["校验结构化动作与依赖 DAG"]
    J --> K{"已有合法候选 ID？"}
    K -->|"否"| L["search_shops 建立候选注册表"]
    K -->|"是"| M["并行 get_shop / list_shop_blogs"]
    L --> N["更新 Candidate、Evidence Ledger、Evidence Gaps"]
    M --> N
    N -->|"证据不足：续查评论或改写重搜"| H
    I -->|"finish"| O["生成结构化 Claim Answer"]
    O --> P["字段一致性、引用合法性、语义蕴含校验"]
    P -->|"通过"| Q["输出带证据回答"]
    P -->|"失败"| R["确定性证据回退或安全无结果"]
    Q --> S["写入会话；按策略更新长期画像"]
    R --> S
```

当前 V2 不是“Planner 一次生成完整计划，再由 Executor 从头执行”的静态工作流。Controller 每轮只输出下一批有类型约束的动作；Executor 返回观察后，运行时重新计算候选、证据缺口与剩余预算，再决定继续检索、翻页、改写重搜、澄清或结束。这种逐轮决策就是当前实现中的 replan。

| 层次 | 当前实现 |
|------|----------|
| Query Understanding | 一次 LLM 结构化解析生成统一 `IntentSpec`，包含 intent、route、hard filters、soft preferences、evidence requirements 和 1～3 条 rewrite；原问题始终保留为检索分支，避免改写丢失硬条件 |
| Adaptive Router | 在 `rag_oneshot`、`agent_multistep`、`agent_memory`、`clarify` 间分流；模型异常时回退确定性 Router，低置信度或缺失历史指代时优先澄清 |
| ReAct Controller | 只输出 `act / finish / clarify` 和短 `reason_code`，不暴露或持久化思维链；非法 JSON 或状态机决策只允许一次有界修复 |
| Parallel Executor | 根据 `depends_on` 计算 DAG 就绪前沿，通过 `errgroup` 限流并行；支持单工具超时、瞬时错误重试、参数去重、失败依赖跳过和总调用预算 |
| Candidate / Evidence | `search_shops` 后才注册候选；详情与评价工具只能访问已注册 ID。评价通过服务端 cursor 增量读取，默认每店最多两页 |
| Rerank | Hybrid 检索与语义相关性形成第一阶段排序；工具执行后再按已验证证据覆盖率确定性重排 |
| Answer Verifier | 最终回答先转为 claim JSON，每条事实绑定 `shop:{id}.{field}` 或 `blog:{id}`；再做字段值、跨店引用、主观评价蕴含与硬条件校验，失败时 fail closed |
| Runtime Safety | 默认限制 4 轮、10 次工具调用、12 次尝试、2 次搜索和每店 2 页评价；无新证据、重复调用、超时和客户端取消都有显式终止状态 |

### 4.6 Agent 层面的主要改动

| 早期/兼容实现 | 当前 V2 |
|---------------|---------|
| Router、RAG 过滤提取和 Agent 各自理解请求，容易出现意图不一致 | 统一 `IntentSpec` 同时服务路由、检索、证据收集和记忆策略 |
| 静态 Planner 或顺序 tool loop | 有界 Parallel ReAct，每轮基于最新 observation 和 evidence gaps 重新决策 |
| 模型可直接给详情/评价工具传任意店铺 ID | Candidate Registry 设置搜索屏障，未检索到的 ID 无法进入后续工具 |
| 一次读取评论后直接生成答案 | 评论证据不足时沿服务端 cursor 继续读取；无支持证据则明确返回无结果 |
| 只检查答案中的店铺 ID 是否出现过 | Claim-level Evidence Ledger + 字段一致性 + LLM entailment，主观结论必须引用真实评价 |
| 将历史和画像直接拼入 Prompt | 分层 Memory Context，并以 `none / read_only / write_after_success` 控制读取与提交 |
| 请求日志难以还原一次运行 | Redis checkpoint + MySQL run/tool audit + OpenTelemetry 分层 span |

这里的 `agent_multistep` 与 `agent_memory` 只是兼容路由标签，不是两个 Agent。二者进入同一个 V2 runtime，区别是是否读取会话/画像以及是否允许在回答验证成功后更新长期记忆。`AGENT_RUNTIME_VERSION=v1_plan` 仅用于回放旧 Planner 基线，默认运行 `v2_react`。

### 4.7 并行执行、记忆与可观测性边界

- **为什么不能一开始并行查详情和评价**：`get_shop`、`list_shop_blogs` 的 `shop_id` 必须来自本轮 `search_shops`。第一轮先搜索；候选注册后，同一批候选的详情和评价才形成可并行的 DAG 就绪前沿。
- **如何继续检查评价**：每次评论工具返回服务端签发的 `next_cursor`。Evidence Gap 仍未满足且 cursor 非空时，下一轮 Controller 才能继续翻页；客户端或模型不能伪造偏移量。
- **Memory 是否需要单独 Agent**：当前没有 Memory Agent。应用层先从 MySQL 加载版本化结构化画像、从 Redis 加载有界会话与摘要，再只注入相关事实；用户明确表达的纯偏好更新可在预检阶段确定性提交，模型推断出的偏好则只有在 `COMPLETED + AnswerVerified` 的 `write_after_success` 路径才能 CAS 更新长期画像。
- **运行恢复**：完整 `AgentState` 按 `run_id + revision` 写入 Redis checkpoint，使用事务 CAS 防止旧 revision 覆盖新状态，TTL 为 30 分钟；它与长期用户记忆分离。
- **执行追踪**：HTTP 上下文中的 W3C Trace ID 贯穿 `agent.run -> agent.controller / agent.action -> tool.execute`；SSE 的 `run_started`、最终响应和 MySQL `agent_runs` 使用同一 `trace_id`。`agent_tool_calls` 仅保存参数摘要/哈希、状态、错误码、耗时和结果数，不保存原始问句或评价正文。

关键实现：

- Query Understanding / Rewrite：`internal/agent/understanding.go`
- Adaptive Router：`internal/logic/adaptive_recommend_router.go`
- ReAct 状态机与预算：`internal/agent/react_runtime.go`、`internal/agent/react_types.go`
- DAG 并行执行：`internal/agent/react_executor.go`
- Evidence Gap / Claim Verifier：`internal/agent/evidence_gap.go`、`internal/agent/claims.go`、`internal/agent/claim_entailment.go`
- Memory 与运行审计编排：`internal/logic/recommend_agent_logic.go`

### 4.8 评测结果

评测使用固定种子生成的 200 家店和 1000 条评价，并连接真实的 DeepSeek API、MySQL 与 Redis。v4 是旧 tool-loop 的历史冻结结果；当前 Parallel ReAct + Adaptive Router 必须看 v6/v6.1 报告，不能沿用 v4 数字冒充新架构效果。

| 数据集 | 结果 |
|--------|------|
| Retrieval v2（60 题） | HitRate@5 100%，Recall@5 81.63%，Precision@5 83.21%，MRR 0.9732，NDCG@5 0.9802，过滤准确率 100%，基础设施错误率 0% |
| Retrieval v3 challenge（120 题） | HitRate@5 70.54%，task success 64.17%，no-result accuracy 12.50% |
| Agent v4（历史旧 runtime）与 one-shot Hybrid RAG 同任务对照 | scenario-macro task success 96.43% vs 46.43%；该数字不代表当前 V2 Agent |
| Router E2E v6.1 首轮诊断（修复前） | trial-micro task success 60.71%，scenario-macro 56.67%，成功任务 groundedness 97.06%，P50/P95 3.741s/13.849s，infra error 0% |
| Router E2E v6.1 修复后（当前 V2） | trial-micro task/composite 92.86%，scenario-macro 90.00%，成功任务 groundedness 100%，P50/P95 8.455s/23.141s，56 trials、infra error 0% |
| Router v2 challenge（52 题） | Accuracy 和 macro-F1 均为 100%；旧策略为 55.77% |

v6.1 通过真实生产入口执行 `Query Understanding -> Router -> Clarify / RAG / Parallel ReAct`，不跑对照组。当前报告是共同机制修复后重新完整实测的结果；仍有 4/56 失败集中在 `agent_memory` 的开放式语义候选选择，不能把 92.86% 外推为线上准确率。详细口径见 [Agent 与评测说明](doc/AGENT_AND_EVAL.md)。

---

## 五、其他功能模块

### 5.1 UV 统计

- **实现**：`middleware/uv.go`，`UVStatisticsMiddleware`
- **存储**：Redis `PFADD uv:{yyyyMMdd} {visitor}`，HyperLogLog 去重
- **标识**：已登录用 userId，未登录用 `IP|UserAgent`

### 5.2 博客与关注

- **博客**：发布、点赞（`blog:like:{id}` Set）、关注流分页
- **关注**：`follow:{userId}` Set，共同关注用 `SINTER`



---

## 六、目录结构

```
local-review-go/
├── cmd/
│   ├── server/                   # 服务入口
│   ├── eval-rag/                 # Retrieval 评测
│   ├── eval-agent/               # Agent / 同任务 Hybrid 评测
│   └── generate-*/               # 固定种子数据与 golden 生成器
├── internal/
│   ├── agent/                    # 有界循环、工具、证据账本、事实校验
│   ├── config/                   # MySQL、Redis、RocketMQ、OTel、env
│   ├── handler/                  # HTTP 与 SSE 接口
│   ├── logic/                    # 业务逻辑层
│   ├── memory/                   # 结构化画像与会话记忆
│   ├── rag/                      # Hybrid 检索与向量存储
│   ├── repository/               # 数据访问（含 interface/）
│   ├── model/                    # GORM 实体
│   ├── middleware/               # JWT、UV、限流
│   ├── mq/                       # RocketMQ 生产者/消费者（秒杀、订单超时、店铺更新）
│   └── llm/                      # Embedding、Chat 客户端
├── pkg/
│   ├── httpx/                    # Result[T]、Ok/Fail、BindJSON
│   └── utils/                    # BloomFilter、DistributedLock、redisx
├── configs/nginx.conf            # Nginx 负载均衡
├── rag-evals/                     # 版本化数据集、golden 与正式报告
├── script/                        # Lua、seed、Demo、冒烟和压测脚本
└── doc/                           # 使用说明与技术文档
```

压测方式与报告见 [doc/LOAD_TEST.md](doc/LOAD_TEST.md)。
