# 模糊问句要不要上 Query 改写

- **日期**: 2026-07-21
- **类型**: interview
- **标签**: `interview` `rag` `query-rewrite` `hyde` `hybrid` `yagni`
- **关联代码**: `internal/logic/rag_logic.go`, `internal/logic/shop_search_logic.go`（001）, Agent `search_shops`（002）
- **关联功能/PR**: `001-evaluable-hybrid-retrieval`；改写不进 001 默认范围

## 一句话结论（面试口述版）

> 模糊问句先靠 **Hybrid + 硬 filter +（002）多轮澄清**，不要默认挂 HyDE/多路改写。改写是「评测证明召回瓶颈后再加」的可选层：哪怕线上目标约 **200 家店**（当前 seed 少只是测试），盲目改写仍易漂意图、加延迟；应用 **vague 子集指标** 决定开不开，而不是按「库变大了就默认开」。

## 场景或症状

- 用户问「附近有什么好吃的」「帮我推荐一下」等过短/过泛问题，纯向量 TopK 可能飘。
- 产品目标 catalog：**真实生活约 200 家店**；当前 seed 仅数十家是为了可核验评测，不代表终态规模。
- 直觉：加 Query 改写 / HyDE 拉齐「问句 ↔ 文档」语义鸿沟。

## 详细说明

### 业界共识（选型，非默认全开）

来源：[HyDE 原论文 (ACL 2023)](https://aclanthology.org/2023.acl-long.99.pdf)；生产侧综述如 [HyDE vs Multi-Query vs Step-Back](https://www.bestaiweb.ai/when-to-use-hyde-vs-multi-query-vs-step-back-prompting-choosing-the-right-query-transformation-for-your-rag/)、[Query rewrite layer 实践](https://tianpan.co/blog/2026-04-16-rag-query-rewrite-layer)：

| 手法 | 擅长 | 风险 |
|------|------|------|
| **HyDE** | 短问 ↔ 长文档语义鸿沟 | 幻觉细节污染检索；额外 1 次 LLM；延迟↑ |
| **Multi-query** | 歧义、多种合理解读 | N× 检索成本；需融合/rerank |
| **同义扩展** | 词面覆盖 | 扩歪意图、Precision↓ |
| **选择性路由** | 简单问句走快路径 | 需要门槛或评测驱动 |

共识：**按失败模式选工具，且常对「简单问句跳过改写」**；先有 baseline 指标再加层。

### 对本仓库：200 店也不等于「该默认改写」

约 200 家仍属**中小领域库**（远小于百万级文档 RAG），但比「测试用几十家」更吃语义与词面覆盖——**Hybrid + filter 的收益会更明显**，改写的必要性仍要用数据说：

1. **先修检索主路径**：共享入口、Hybrid、硬槽位、正确评测（001）。
2. **过泛问句优先产品层**：002 澄清 / profile，避免静默改写「猜错用户要啥」。
3. **触发条件 = 检索效果不好（且归因清楚）**，例如：
   - `hybrid_prod` 上 **vague 标签子集** HitRate/Recall 明显低于整体；或
   - 线上抽检：短问/口语问句系统性 miss 相关店，而 filter 已正确。
4. **不要**仅因为「catalog 从 30→200」就默认打开改写流水线。

### 若评测证明后再做

1. 在 `retrieval.v1`（及扩店后的 v2）标 **vague** 题，先录无改写的 Hybrid 基线。
2. 仅 vague 子集差时试：
   - **轻量**：改写主要服务 TEXT 路 / 轻量扩写；或
   - **Agent 内**：`search_shops(query=…)` 由模型改参数（可观测、有步数预算）。
3. HyDE 仍慎用：易编造不存在的氛围/菜品。
4. 报告必须带：是否改写、模型、延迟；与无改写对照（百分点写法同评测协议）。

## 常见追问

1. **Q**: 是不是检索不好再加改写？  
   **A**: **对。** 更精确：先有 Hybrid 基线 → 看 vague/失败 case → 归因不是 filter/索引问题 → 再加改写并对照评测。不是「感觉模糊就加」。

2. **Q**: 目标 200 家店了，还坚持后置？  
   **A**: 规模变大提高 Hybrid/改写的**潜在**收益，但不改变「评测门禁」；200 店仍可用全量人工核验题集，先把主路径做稳更划算。

3. **Q**: Agent 算不算已经在做改写？  
   **A**: 模型可在 tool 的 `query` 里换说法，但是 **按步、可观测、有预算** 的，不是检索前强制流水线。

## 如何避免再发 / 复习提示

- 面试口径：「目标约 200 店；改写是评测驱动的可选层，默认靠 Hybrid + 硬过滤 + Agent 澄清。」

## 参考

- 内部：`docs/solutions/interview/2026-07-21-rag-vs-standard-and-intent-scope.md`；`docs/plans/2026-07-11-recommend-agent-eval.md` §0.3
- 外部：HyDE ACL 2023；上文 Linked 实践文
