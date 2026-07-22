# 当前正在做什么 (activeContext)

> 本文档记录当前开发进度，有重大进展时请更新。

## 工具 / 环境（最近）

- **Spec Kit（specify CLI）**：已安装；本仓库 `cursor-agent` 集成（`.specify/` + `.cursor/skills/speckit-*`）
- **Constitution v1.1.1**：秋招面试导向；原则 VI 知识落盘；`memory-bank/` **仅保留** `activeContext.md`

## 001 Evaluable Hybrid Retrieval（进行中）

**状态**：代码已落地（US2/US4/US1/US3）；待本地 `docker compose` + `seed-vector` 后跑 `make eval-rag-smoke` / `eval-rag-prod` 录 `hybrid_prod` 基线。

已完成要点：
- 维度契约 fail-fast（删 1536 fallback；Embed 校验 `LLM_EMBEDDING_DIM`）
- 删 `IngestShop`；Redis 镜像 pin `7.4.0-v8`
- `ShopSearchLogic` 共享入口；默认 Hybrid（TEXT∥KNN + FuseRRF）；`RAGLogic` / `cmd/eval-rag` 共用
- 正式 golden `rag-evals/golden/retrieval.v1.json`（30 题）；正确 HitRate≠Recall 指标 bundle
- Makefile：`eval-rag-smoke` / `oracle` / `prod`

下一步：起栈 → seed → seed-vector → `make eval-rag-prod --write-baseline`；知识落盘 `docs/solutions/interview/`。

## 开发计划（来自 README，按推荐顺序）

### 第一阶段：分布式架构与可观测性（优先）

> **核心痛点**：单机 → 可水平扩展的分布式集群。

1. **Nginx + 多实例部署** ✅
   - 已实现：1 Nginx + 3 Go 实例，`least_conn` 负载均衡
   - 健康检查：`/health` 端点 + Nginx `max_fails=3 fail_timeout=30s` 被动健康检查
   - 日志：JSON 格式 + `instance_id`，便于集中收集
   - 可观测性：OpenTelemetry Trace（Jaeger OTLP），未配置 endpoint 时自动 noop
   - 配置一致性：`env_file` 统一 JWT_SECRET_KEY 等
   - 连接池：`MYSQL_MAX_OPEN_CONNS=30` 每实例，避免 3×100 超限

### 第二阶段：高并发缓存体系 (Cache & Consistency)

2. **基于 Redis BitMap 的布隆过滤器** ✅
   - 店铺、秒杀券 ID 预热，防恶意请求穿透

### 第三阶段：高可靠异步架构 (Reliability & Async)

3. **秒杀削峰填谷 (RocketMQ 改造)** ✅
4. **服务熔断与限流** ✅
5. **订单超时处理 (Delay Message)** ✅
6. **秒杀防护增强** ✅（唯一索引、秒杀券布隆过滤器）

### 第四阶段：搜索与智能化 (Search & AI)

7. **AI 智能点评助手 (RAG)** ✅（Naive 基线已完成）
   - 流程：用户提问 → LLM 提取 filter → Embedding API 转向量 → Redis Stack KNN 检索 Top5 店铺 → LLM 生成推荐 → SSE 流式输出
   - 已实现：Redis Stack 向量索引、Embedding/Chat 客户端、VectorRepo、RAG Logic、Chat Handler、seed-vector 离线导入
   - **数据同步**：店铺创建/更新时发 MQ（shop-update），RAG 消费者异步更新向量
   - **Filtered Vector Search**：预过滤（area、type_name、avg_price、score、comments）+ 语义阈值 MaxDistance；`POST /api/rag/chat` 支持 `filter` 参数
   - **Embedding 语义优化**：embedding 文本为「店铺名 + 用户点评摘要」，与 filter 覆盖字段分离，承载「浪漫」「适合约会」等 filter 无法表达的语义；`internal/rag/text.go` 提供 `BuildShopTextForEmbedding(shop, blogs)`

8. **可评测智能搜索 + 带记忆推荐 Agent** 📋 001 Tasks 已生成 → 待 `/speckit-implement`
   - 源计划：`docs/plans/2026-07-11-recommend-agent-eval.md`
   - **本规格**：`specs/001-evaluable-hybrid-retrieval/` — Hybrid 默认 + 共享检索 + 可信评测（**不做 dense vs hybrid 对照交付**，2026-07-21）
   - **Plan 产物**（2026-07-21）：`plan.md` / `research.md` / `data-model.md` / `contracts/` / `quickstart.md`
   - **Tasks**：`specs/001-evaluable-hybrid-retrieval/tasks.md`（T001–T042；顺序 US2→US4→US1→US3）
   - 实现顺序：契约 → 共享入口 → 评测基线 → Hybrid RRF + `hybrid_prod` 基线
- **RAG eval skill**：`.cursor/skills/rag-eval/`（实现期需同步去掉过时的 dense 对照表述）
- **评测环境**：Docker Compose 本地为主；AidLux 犀牛派不作 baseline 环境
   - **后续**：`002` Recommend Agent + 结构化记忆 + Agent eval（计划 Task 4–8）；**对外数字优先 Agent vs Hybrid RAG**
   - 明确不做：Mem0 / Eino / LangGraph / Milvus；dense↔hybrid 简历叙事；模型侧 memory tool；事实向量记忆为 Phase 2
   - 2026-07-12：HitRate≠Recall；小高质量集；整包 Spec 拆成 001/002
   - 2026-07-21：跳过 dense A/B；Hybrid 直接默认；精力给 Agent

### 店铺更新 MQ 异步化 ✅

- **缓存**：UpdateShop 不再同步删缓存，改为发 MQ → 消费者异步 Del
- **RAG 向量**：SaveShop/UpdateShop 发 MQ → RAG 消费者异步 Embedding + 更新 Redis 向量
- **Topic**：`shop-update`，两消费者组：`shop-update-cache-consumer-group`、`shop-update-rag-consumer-group`

---

## 近期完成：秒杀防护增强 ✅

- **唯一索引**：`tb_voucher_order (user_id, voucher_id)` 唯一约束
- **限流**：秒杀接口 QPS 限流（`golang.org/x/time/rate`），默认 1000 QPS，超限 429
- **秒杀券布隆过滤器**：`bf:seckill-voucher` 启动预热，AddSeckillVoucher 时同步加入
- **订单超时延迟消息**：30 分钟未支付自动关单 + 回滚 Redis/MySQL

---

## 近期完成：黑马点评前端适配 ✅

- **API 前缀**：所有接口统一挂载到 `/api`
- **静态文件**：Gin 托管 `front-end/`，访问 http://localhost:8088
- **新增接口**：`GET /user/:id`、`GET /blog/of/user?id=&current=`
- **Logout**：登出接口
- **上传路径**：`front-end/imgs`，删除时兼容 `/imgs` 前缀

---

## 已砍掉 / 不再规划

- **多级缓存 (L1 Local + L2 Redis)**：已砍掉
- **Elasticsearch**：已砍掉，RAG 改用 Redis Vector

---

---

## 工具链

- **Skill 编写**：Anthropic 改进版 `skill-creator` 已安装至 `~/.cursor/skills/skill-creator/`（eval / benchmark / description 优化）。`.cursorrules` 已要求创建或改进 Skill 时优先遵循该 skill。

## 文档索引

- **README**：项目概览、架构设计、启动说明
- **AGENTS.md**：工程规范、分层、启动与业务约定
- **.specify/memory/constitution.md**：秋招导向宪章
- **.cursorrules**：会话级规则（读 AGENTS、activeContext、docs/solutions）
- **memory-bank/activeContext.md**：本文件 —— 仅当前进度（勿再扩展其它 memory-bank 文件）
- **docs/solutions/**：面试知识与开发卡点

*最后更新：2026-07-21 — 001 `/speckit-tasks` 完成（tasks.md T001–T042）。*
