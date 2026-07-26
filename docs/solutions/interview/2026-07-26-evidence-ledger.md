# EvidenceLedger：为何不能只靠 observed ID 集合

- **日期**: 2026-07-26
- **类型**: interview
- **标签**: [agent, groundedness, evidence, 幻觉防护]
- **关联代码**: `internal/agent/evidence.go`, `internal/agent/verifier.go`, `internal/agent/tools.go`, `internal/agent/loop.go`
- **关联功能/PR**: specs/003-agent-hardening US1

## 一句话结论（面试口述版）

> 仅用「出现过的 shop_id」不够：空评价或对任意 ID 调工具会污染观测集。EvidenceLedger 区分 discovered/verified/字段来源；终答先校验引用与强事实，再发成功 SSE。

## 场景或症状

- 旧实现：`list_shop_blogs` / `get_shop` 成功或空结果都会把 id 写入 Observed。
- 攻击面：对未检索到的 ID 调空 blogs → 模型引用该 ID → 旧 groundedness 仍可能放行。

## 详细说明

### 原理 / 根因

工具结果是**证据**，不是「说过就算」。可引用身份只应来自本轮成功检索（discovered）；详情核实写 verified 字段；空 blogs 不得单独造身份。

### 方案与取舍

| 方案 | 结论 |
|------|------|
| 仅收紧 Observed 写入 | 不够校验人均/地址冲突 |
| LLM-as-judge | 非确定，不宜硬门禁 |
| EvidenceLedger + VerifyAnswer | 采用 |

### 落地要点

- search → Discover；get/list 须已 discovered
- 空 blogs → 不授予 citeable
- 有候选却无 `[shop:id]` → `grounding_no_citation`
- 人均与证据冲突 → `grounding_fact_conflict`

## 常见追问

1. **Q**: 和 RAG 防幻觉有何不同？  
   **A**: RAG 靠上下文约束；Agent 多工具动态证据，必须账本 + 发出前校验。

2. **Q**: 无结果怎么办？  
   **A**: 本轮无 citeable 且无引用 → 允许说明无合适店铺，不算成功硬荐。

## 参考

- `specs/003-agent-hardening/contracts/evidence-ledger.md`
