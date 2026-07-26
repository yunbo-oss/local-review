# docs/solutions — 面试知识与开发卡点

本目录是宪章 **原则 VI** 要求的个人知识库，服务秋招复习：把「能讲的」和「踩过的」从聊天里沉淀成可检索文档。

## 目录

| 路径 | 用途 |
|------|------|
| `interview/` | 面试有价值的知识（原理、选型、追问答法） |
| `blockers/` | 开发卡点（现象 → 根因 → 修复 → 防再发） |
| `TEMPLATE.md` | 新建条目时复制此模板 |
| `README.md` | 本说明 + 索引 |

## 何时必须写

- 学到可迁移的后端/分布式/Go/中间件/RAG 要点，且面试可能被问到
- 排查耗时明显、或结论对以后有用的 bug / 环境问题
- 完成一个有设计故事的用户故事或重要修复后（实现收尾核对）
- **与 AI 讨论知识点、方案对比、面试答法时**：形成可复用结论后，当场或当次回合更新（宪章原则 VI）；无新结论可跳过但应说明原因

## 怎么写

1. 复制 `TEMPLATE.md` 到对应子目录，文件名建议：`YYYY-MM-DD-简短英文或拼音.md`
2. 填完「一句话结论」——这应是你面试时能直接开口的那句
3. 关联具体代码路径（便于演示）
4. 在下方 **索引** 里加一行

禁止写入真实密钥、生产密码、不可公开的个人隐私。

**注意**：本目录不是项目进度板。进度看 `memory-bank/activeContext.md`；工程规范看 `AGENTS.md`；原则看宪章。

## 索引

| 日期 | 类型 | 标题 | 文件 |
|------|------|------|------|
| 2026-07-26 | interview | Agent vs Hybrid：评测叙事怎么讲 | [2026-07-26-agent-vs-hybrid-eval.md](./interview/2026-07-26-agent-vs-hybrid-eval.md) |
| 2026-07-26 | interview | RecommendRouter：简单→RAG / 复杂→Agent | [2026-07-26-recommend-router.md](./interview/2026-07-26-recommend-router.md) |
| 2026-07-26 | interview | EvidenceLedger：为何不能只靠 observed ID | [2026-07-26-evidence-ledger.md](./interview/2026-07-26-evidence-ledger.md) |
| 2026-07-23 | interview | 记忆为何不暴露为模型工具 | [2026-07-23-memory-not-as-tools.md](./interview/2026-07-23-memory-not-as-tools.md) |
| 2026-07-23 | interview | 有界 Agent：预算与 Groundedness | [2026-07-23-bounded-agent-groundedness.md](./interview/2026-07-23-bounded-agent-groundedness.md) |
| 2026-07-22 | interview | RAG 检索链路拆解：ShopSearch/RAG、Hybrid/RRF、指标与 Golden | [2026-07-22-rag-retrieval-eval-walkthrough.md](./interview/2026-07-22-rag-retrieval-eval-walkthrough.md) |
| 2026-07-21 | interview | 评测与线上路径对齐（ShopSearchLogic） | [2026-07-21-shop-search-path-alignment.md](./interview/2026-07-21-shop-search-path-alignment.md) |
| 2026-07-21 | interview | HitRate vs Recall vs Precision | [2026-07-21-hitrate-vs-recall-precision.md](./interview/2026-07-21-hitrate-vs-recall-precision.md) |
| 2026-07-21 | interview | 模糊问句要不要上 Query 改写 | [2026-07-21-query-rewrite-when-needed.md](./interview/2026-07-21-query-rewrite-when-needed.md) |
| 2026-07-21 | interview | 本仓库 RAG vs 标准 RAG；意图范围只有硬槽位 | [2026-07-21-rag-vs-standard-and-intent-scope.md](./interview/2026-07-21-rag-vs-standard-and-intent-scope.md) |
| 2026-07-21 | interview | Hybrid 默认上线，跳过 dense 对照 | [hybrid-default-skip-dense-ab.md](./interview/hybrid-default-skip-dense-ab.md) |
