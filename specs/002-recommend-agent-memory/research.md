# Research: Recommend Agent with Correctable Memory

**Feature**: `002-recommend-agent-memory`  
**Date**: 2026-07-23  
**Status**: All Technical Context unknowns resolved (no remaining NEEDS CLARIFICATION)

## 1. Structured memory: session + profile (Phase 1 Hash only)

**Decision**:
- **Session** `agent:sess:{userId}:{sessionId}` — Redis List，元素 JSON `{role, content, ts}`；pipeline `RPUSH → LTRIM(0,19) → EXPIRE 7d`；加载后按**字符/ token 预算**二次裁剪（不只靠条数）。
- **Profile** `agent:profile:{userId}` — Redis Hash：`preferred_areas`, `preferred_types`, `budget_max`, `dislikes`, `summary`, `version`, `updated_at`；TTL 90d，更新后刷新。
- **ProfilePatch** 增量语义：`_*_add` / `_*_remove`；同值 add+remove → remove 胜出；`budget_max` 三态（nil=不变，0=清空，正数=覆盖）。
- **MergeProfile** 纯函数 + **MemoryRepo.MergeProfile** 用 `WATCH version` CAS，冲突最多重试 3 次。
- **ExtractProfilePatch**：输入**仅用户本轮原话** + 旧 profile；assistant 输出不得作事实来源；JSON 解析/范围校验失败 → Warn，不覆盖旧 profile。

**Rationale**: Spec FR-001–004 / US1；源计划 §1.4。Hash 足够表达硬偏好；向量记忆 Phase 2 仅当真实失败案例证明不够（YAGNI）。

**Alternatives considered**:
- Mem0 / 模型 `read_memory`/`update_memory` 工具 → 噪声、安全、难测；规格明确禁止
- 每次全量 profile 覆盖 → 无法「忘掉预算」与并发合并
- assistant 输出写回 profile → 幻觉污染长期记忆

**Filter 注入**: `MergeFilterWithProfile(explicit, extracted, profile)` — 显式 > extracted > profile 仅补空字段；001 的 `ResolveFilter` 扩展此层。

---

## 2. Memory not exposed as model tools

**Decision**: 推荐回合开始 `LoadSession` + `LoadProfile`；system prompt 注入 profile 摘要；filter 解析层补空；回合结束服务端 `ExtractProfilePatch` → `MergeProfile`。模型**仅**见 3 个只读领域工具。

**Rationale**: Spec FR-006 / 面试叙事；服务端控制读写可校验、限权、单测 merge/extract，不依赖模型自律。

**Alternatives considered**:
- 暴露 memory read/write tools → 调用噪声、难纠正、安全面扩大

---

## 3. Bounded Agent loop (self-hosted, no orchestration framework)

**Decision**:
- 默认预算：`maxSteps=3`, `maxToolCalls=5`, `toolTimeout=10s`, `runTimeout=45s`, `maxToolResultChars=6000`（env 可配，验收按默认）。
- 循环：`ChatWithTools` → 若有 tool calls 则执行（经 dedupe key=`toolName+canonicalJSON(args)`）→ 追加 tool result → 直至无 tool call 或触达预算。
- 相同 tool+args 第二次请求 → 拒绝执行，返回结构化「duplicate」给模型，计 trajectory。
- Context cancel（SSE 断连）→ 立即停止；**未完成成功回合不写 profile**（session 亦仅在成功路径追加，与源计划一致）。

**Rationale**: Spec FR-007 / US2；Anthropic effective agents：从简单可测 loop 开始；源计划 §1.2–1.3。

**Alternatives considered**:
- LangGraph / Deep Agents / Eino → 依赖叙事弱、YAGNI
- 无步数上限 → 成本与空转不可控

---

## 4. Three read-only domain tools

**Decision**:

| Tool | 职责 | 后端 |
|------|------|------|
| `search_shops` | 发现候选 | `ShopSearchLogic.Search`（默认 hybrid）+ 本轮 filter（含 profile 补空） |
| `get_shop` | 权威详情 | `ShopRepo.GetByID` |
| `list_shop_blogs` | 真实评价 | `BlogRepo` 按 shop_id 限条 |

参数 JSON Schema + Go 本地校验（shop_id>0, limit 上限等）；结果超长服务端截断。

**Rationale**: Spec FR-006；源计划 §1.2 — 搜索与详情分离，blogs 按需加载体现动态工具选择。

**Alternatives considered**:
- 把所有字段塞进 search 结果 → 上下文膨胀、难按需加载

---

## 5. LLM tool calling extension

**Decision**:
- 新增 `ToolChatClient` 抽象（`internal/llm`）：`ChatWithTools(ctx, messages, tools) (AssistantTurn, error)`；`ChatComplete` 保留给最终回答或无 tools 路径。
- `AssistantTurn` 含 assistant content、tool calls、usage；Agent 核心依赖接口，不直接绑 `go-openai` 类型。
- 启动时对默认 chat 模型做一次 function calling smoke（不兼容则启动失败，不静默降级）。

**Rationale**: 现有 client 仅有 Embed/ChatStream/ChatComplete；Task 6 前置；可 mock 单测 loop。

**Alternatives considered**:
- ReAct 纯文本 `<tool>` 标签 → 解析 fragile
- 仅 ChatComplete + 手工 JSON → 与 OpenAI 兼容 API 工具协议不一致

---

## 6. Groundedness: validate before SSE answer

**Decision**:
- 工具执行期间维护 **observedShopIDs** 集合（search 结果 + get_shop + blogs 关联 shop）。
- 最终回答完整生成后，解析引用格式 `[shop:{id}]`（与源计划一致）；凡引用 id ∉ observed → **grounding error**，走 error 事件或降级说明，**不**发 `message` 成功流。
- Phase 1 **不**流式边生成边校验（token 已发无法撤回）。

**Rationale**: Spec FR-008 / US3 / SC-002；源计划 §1.3 幻觉约束。

**Alternatives considered**:
- 流式 message 后再校验 → 无法撤回幻觉内容
- 不强制引用格式 → eval groundedness grader 无法确定性判分

---

## 7. SSE protocol for `/api/agent/recommend`

**Decision**:
- `POST /api/agent/recommend`（`AuthRequired`）；body `{question, session_id}` 均 required。
- 事件：`status`（searching | reading_shop | reading_blogs）、`message`（最终完整回答，单条或分块但已校验）、`done`（trace_id + usage 摘要）、`error`。
- **禁止**：raw tool JSON、profile 全文、system prompt、chain-of-thought。

**Rationale**: Spec FR-005 / US2；对齐现有 `rag.go` SSE 模式并扩展 status。

**Alternatives considered**:
- WebSocket → 现有项目 SSE 先例足够
- 非流式 JSON 一次返回 → 长推理 UX 差，且与 RAG 流式不一致

---

## 8. Agent evaluation: deterministic graders + multi-trial

**Decision**:
- 场景集 `rag-evals/golden/agent.v1.json`：10～15 条；字段见 data-model。
- **Graders（纯函数，先单测）**:
  - `GradeOutcome` — filter/allowed shops/profile_after/max_steps/max_tool_calls
  - `GradeGroundedness` — 引用 ⊆ observed
  - `GradeTrajectory` — 步数/工具数/重复调用/预算
  - `AggregateTrials` — 关键 case ≥3 trial 成功率
- Harness：每 trial 独立 `session_id`；setup profile 预写；捕获 transcript、tool calls、observed IDs、final profile、latency/tokens。
- Infra 失败单列，不进质量分母；正式命令 infra>0 非零退出（对齐 001 协议）。
- **对照**：同条件 subset 上对比 Agent vs `make eval-rag-prod`（Hybrid RAG 单次问答 outcome 代理指标或人工对齐题集）；叙事固定 **Agent vs Hybrid RAG**，非 dense。

**Rationale**: Spec FR-010–013 / US4；源计划 §1.7–1.8；Anthropic/OpenAI agent eval 指南：小高质量集 + 多 trial + 代码 grader。

**Alternatives considered**:
- LLM-as-judge 作 PR 硬门禁 → 抖动大；Phase 1 仅可选抽检开放式措辞
- 仅 outcome 不看 trajectory → 无法证明 bounded loop 价值

---

## 9. Observability and rate limiting

**Decision**:
- Spans（`internal/agent/trace.go`）：`agent.run`, `llm.tool_turn`, `tool.execute`, `rag.embed`, `rag.search`, `llm.generate`。
- 属性：model、tool_name、status、steps、tokens、latency、candidate_count；**不**记录 question 全文、profile 全文、blog 正文。
- `AgentRateLimit`：按 JWT userID 滑动窗口；超限 429，**不**调用 LLM。

**Rationale**: Spec FR-014–015 / US5；constitution 分层 + 面试可演示 trace。

---

## 10. Execution order (Task 5 before Task 6)

**Decision**: 先落地 `agent.v1.json` + graders 单测（Task 5），与 MemoryRepo（Task 4）可并行；Agent loop（Task 6）实现时已有验收标准。

**Rationale**: 源计划明确「先定义 eval」；Spec US4 Independent Test 允许助手实现前测 grader。

**Alternatives considered**:
- 先写 Agent 再补 eval → 易漏测边界（循环、groundedness）

---

## 11. Prerequisite from 001

**Decision**: 复用 `ShopSearchLogic`（hybrid 默认）、`hybrid_prod` 基线、现有 auth middleware；001 未完成 baseline 录制时，002 对照章节可先用本地 report 路径，但 DoD 要求固定 fingerprint。

**Rationale**: Spec prerequisite；避免重复建设检索层。
