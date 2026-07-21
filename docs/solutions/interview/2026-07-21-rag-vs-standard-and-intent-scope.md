# 本仓库 RAG：只有硬槽位抽取，不是完整意图解析

- **日期**: 2026-07-21
- **类型**: interview
- **标签**: `interview` `rag` `filter` `intent` `filtered-vector-search`
- **关联代码**: `internal/logic/rag_logic.go`, `internal/handler/rag.go`, `internal/repository/vector_repo.go`
- **关联功能/PR**: 规格 `001-evaluable-hybrid-retrieval`（共享入口 / Hybrid）；Agent 多步意图在 `002`

## 一句话结论（面试口述版）

> 我们线上是 **Naive Filtered Vector RAG**：在标准「Embed → 检索 → 生成」前多一步 **结构化硬过滤槽位抽取**（区域/类型/价格等），没有做意图分类、对话动作识别或完整 NLU。软语义（浪漫、约会）进 embedding，硬约束进 RediSearch 预过滤。

## 场景或症状

- 背景：面试常问「你们 RAG 和教科书 RAG 有什么不一样？有没有意图识别？」
- 易混点：Prompt 里写了「意图解析助手」，容易被理解成完整意图系统。

## 详细说明

### 本仓库做了什么「意图」相关工作

| 做了 | 没做 |
|------|------|
| LLM 从问句抽硬槽位 → `VectorSearchFilter` | 意图分类（推荐 / 闲聊 / 比价 / 投诉…） |
| 客户端可显式传 `filter` | 多轮指代消解、槽位继承 |
| 抽失败则不过滤（降级） | 工具路由、Agent 规划（属 002） |
| | Query rewrite / HyDE / 多路 query 扩展 |

### 与标准 RAG 流程对比

**教科书 / 常见基线 RAG**：

```text
Question → Embed → Vector TopK → 拼 context → LLM 生成
```

**本仓库当前（Naive Filtered Vector RAG）**：

```text
Question
  → [可选] 显式 filter 或 LLM 抽硬槽位
  → Embed(原问题)
  → Redis：结构化预过滤 + KNN Top5
  → 拼店铺向量文本 + Blog 点评
  → LLM 流式生成（SSE）
```

**001 规划中的增量（相对当前）**：共享 `ShopSearchLogic` + 默认 Hybrid（TEXT+向量 RRF）；仍不是完整意图引擎。

**002 规划**：多步 Agent + 记忆。下列能力**部分**由 Agent 覆盖，但多数不是单独 NLU 模块：

| 能力 | 002 Agent 里怎么做 | 是否单独模块 |
|------|-------------------|--------------|
| 工具路由 | 模型在 bounded loop 里选 `search_shops` / `get_shop` / `list_shop_blogs` | 否，靠 tool-calling |
| 多轮 / 指代 | 服务端注入 session 历史 + profile；模型在上下文里消解「那家」「还是老预算」 | 否，无独立指代解析器 |
| 硬条件补全 | ResolveFilter：显式 > LLM 抽槽 > profile 补空 | 沿用/扩展 001 抽槽，非新意图分类器 |
| 意图分类 | **不做**独立分类器（推荐 vs 闲聊…） | 产品面基本是推荐任务 |
| Query 改写 | **不作为默认流水线**；模糊问句优先 Hybrid + filter +（002）澄清；评测证明后再考虑轻量改写/Agent 内改 query | 见 `2026-07-21-query-rewrite-when-needed.md` |
| Memory 读写 | **不做**模型侧 memory tool；服务端 Load/Persist/MergeProfile | 应用状态，非领域工具 |

### 方案与取舍

- **为何加 filter 而不做完整意图**：点评检索里区域/价格是硬约束，进预过滤比全靠向量更稳、更可解释；完整 NLU 成本高，当前 catalog 小、产品形态是单轮问答。
- **为何软语义不进 filter**：索引没有「浪漫」TAG；氛围靠点评摘要 embedding 承载。

## 常见追问（建议至少 2 条）

1. **Q**: 这算不算 Query Understanding？  
   **A**: 算很窄的一层——**structured constraint extraction / slot filling**，不是完整 QU 流水线。

2. **Q**: 和「带 rerank 的标准 RAG」比缺什么？  
   **A**: 无独立 reranker、无 query rewrite、无分块文档语料（我们是店级向量 + 点评拼接）；强项是领域硬过滤 +（001）Hybrid。

3. **Q**: 评测为什么要分 oracle / llm filter？  
   **A**: 隔离「抽槽噪声」和「检索质量」，避免把 filter 抽错当成向量召回差。

4. **Q**: 那意图分类、指代、改写是不是都放到 Agent 里？  
   **A**: 工具路由和多轮上下文会；但不是再挂一套 NLU。无独立意图分类/指代器/query rewriter；记忆由服务端管，不交给模型随意读写。

## 如何避免再犯 / 复习提示

- 对外说「意图解析」时补半句：**只抽检索硬条件**。
- 画图时把 filter 画在 Embed 之前、与语义检索正交。

## 参考

- 内部：`specs/001-evaluable-hybrid-retrieval/spec.md`；`docs/plans/2026-07-11-recommend-agent-eval.md` §1.1
- 外部：Filtered vector search / metadata filtering 常见于向量库文档（Redis / Milvus 等预过滤模式）
