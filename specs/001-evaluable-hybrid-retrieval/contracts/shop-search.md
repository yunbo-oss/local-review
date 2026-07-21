# Contract: ShopSearchLogic（共享检索入口）

**Consumers**: `RAGLogic`（`/api/rag/chat`）、`cmd/eval-rag`、后续 `002` Agent `search_shops` tool  
**Date**: 2026-07-21

## Purpose

保证「解析过滤条件 → 按策略检索」在线上与评测为同一决策路径（Spec FR-005 / SC-003）。

## Go interface (target)

```go
type RetrieverStrategy string

const (
    RetrieverHybrid RetrieverStrategy = "hybrid" // default
    RetrieverDense  RetrieverStrategy = "dense"  // diagnostic only
)

type ShopSearchLogic interface {
    // Search 在已解析的 filter 上检索；filter==nil 表示不过滤。
    // strategy 默认 hybrid；文本子检索失败时返回 error（不得静默 dense 成功）。
    Search(ctx context.Context, query string, filter *interfaces.VectorSearchFilter, strategy RetrieverStrategy, topK int) ([]interfaces.ShopSearchResult, error)
}

// Filter 解析与 Search 分离（可同包或可注入接口）
type FilterExtractor interface {
    Extract(ctx context.Context, question string) (*interfaces.VectorSearchFilter, error)
}

// ResolveFilter：explicit 非空字段覆盖 extractor 结果；001 无 profile。
func ResolveFilter(explicit *interfaces.VectorSearchFilter, extracted *interfaces.VectorSearchFilter) *interfaces.VectorSearchFilter
```

## Behavioral contract

| 输入 | 行为 |
|------|------|
| `strategy=hybrid` | Embed → 并行 KNN TopN + TEXT TopN → FuseRRF → TopK；TEXT 失败 → error |
| `strategy=dense` | Embed → KNN TopK（仅诊断） |
| `filter` 非 nil | RediSearch 预过滤（area/type/price/score/comments）应用于两路（或至少 KNN 路与 TEXT 路一致策略，实现期统一） |
| 维度不一致 | Embed 失败向上返回，不写索引 |

## Result ordering

返回列表下标 0 为最佳；同一逻辑输入下顺序稳定（RRF 平局按 `shop_id` 升序）。

## Out of scope

- Profile 补全、会话记忆、生成回答、SSE 协议细节
