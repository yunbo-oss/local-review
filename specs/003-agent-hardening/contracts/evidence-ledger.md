# Contract: EvidenceLedger & Answer Verification

**Phase**: A | **Consumers**: ToolExecutor, RunLoop/Harness, graders（Phase B）

## Purpose

本轮工具结果写入证据账本；终答在发出成功推荐前通过 Grounding + Fact 校验。

## Mutations

| 工具结果 | Ledger 效果 |
|----------|-------------|
| `search_shops` 非空命中 | 各 shop → discovered；登记基础字段（若有） |
| `get_shop` 命中 | verified=true；登记权威字段 |
| `get_shop` not_found | 不授予可引用 |
| `list_shop_blogs` 非空且 shop 已 discovered | 登记 blog_ids；不单独因空列表发现店铺 |
| `list_shop_blogs` 空 | **不得**将 shop 标为可引用 |

默认：详情/评价工具仅允许访问本轮 discovered 的 shop_id（防任意 ID 探测洗白）。

## Verify（成功 message 前）

1. 若非 no-result outcome：答案 MUST 含 ≥1 个 `[shop:{id}]`。
2. 每个引用 id MUST ∈ 可引用集（至少 discovered；陈述强事实时建议 verified）。
3. 答案中可解析的强事实（均价/地址/营业时间/评分）MUST 与 ledger.fields 一致，否则删事实或整答失败。
4. 失败 → 不发成功 `message`；run grounding_status=FAILED。

## Errors（稳定类别）

`grounding_no_citation` | `grounding_unknown_shop` | `grounding_fact_conflict` | `grounding_empty_blogs_wash`
