# Contract: Agent Domain Tools

**Package**: `internal/agent`  
**Executor**: `ToolExecutor`（注入 ShopSearchLogic, ShopRepo, BlogRepo）  
**Consumer**: bounded loop via `ChatWithTools`  
**Date**: 2026-07-23

## Purpose

三类只读领域能力；参数 JSON Schema + Go 校验；结果计入 observed shop IDs（Spec FR-006 / FR-007）。

## Tool definitions

### 1. `search_shops`

发现候选店铺。

**Parameters**:

| 名 | 类型 | 必填 | 说明 |
|----|------|------|------|
| `query` | string | yes | 语义检索问句 |
| `area` | string | no | 硬过滤区域 |
| `type_name` | string | no | 硬过滤类型 |
| `max_price` | integer | no | 人均上限 |

**Behavior**:
- 合并工具参数与本轮已解析 filter（显式 > 抽取 > profile 补空已在 logic 层）
- 调用 `ShopSearchLogic.Search(..., RetrieverHybrid, topK=5)`（默认）
- 返回精简 JSON 列表：`[{shop_id, name, area, type_name, avg_price, score}]`
- 所有返回 shop_id 加入 observed set

**Errors**: 参数非法 → 结构化 tool error（不 panic）；检索 infra 失败 → tool error 文本

---

### 2. `get_shop`

读取单店权威详情。

**Parameters**:

| 名 | 类型 | 必填 |
|----|------|------|
| `shop_id` | integer | yes |

**Behavior**: `ShopRepo.GetByID`；字段含 name, area, type, prices, score, address, hours 等（实现期对齐 model）；shop_id 加入 observed set。

**Errors**: shop_id ≤ 0 → 校验 error；不存在 → `{ "error": "not_found" }`

---

### 3. `list_shop_blogs`

列出店铺真实评价（按需）。

**Parameters**:

| 名 | 类型 | 必填 | 默认 |
|----|------|------|------|
| `shop_id` | integer | yes | — |
| `limit` | integer | no | 5（max 10） |

**Behavior**: 返回 `[{blog_id, title, content_snippet, score}]`；content 截断；shop_id 加入 observed set。

## Cross-tool rules

| 规则 | 值 |
|------|-----|
| Dedupe | 同 run 内 `toolName+canonicalJSON(args)` 重复 → 拒绝，返回 duplicate 标记 |
| Timeout | 单工具 ctx 10s（可配置） |
| Result cap | 6000 字符，超出截断 |
| Observed set | search 结果 IDs ∪ get_shop ID ∪ list_shop_blogs 的 shop_id |

## JSON Schema (OpenAI-compatible tools array)

实现期导出 `[]llm.ToolDefinition`；name/description/parameters 与上表一致；`additionalProperties: false`。

## Out of scope

- `read_memory` / `update_memory`
- 写操作（下单、收藏等）
- 任意 SQL / 自定义 query
