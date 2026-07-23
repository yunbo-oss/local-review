# Implementation Plan: Recommend Agent with Correctable Memory

**Branch**: `002-recommend-agent-memory` | **Date**: 2026-07-23 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-recommend-agent-memory/spec.md`

**Note**: This plan covers source-plan Tasks 4–8 (Memory + Agent eval definition + bounded Agent + HTTP/SSE/OTel + eval/demo/docs). Prerequisite: `001-evaluable-hybrid-retrieval`（共享 `ShopSearchLogic` + Hybrid 默认 + `hybrid_prod` 基线）。

## Summary

在规格 `001` 已交付的 Hybrid 检索底座之上，构建**可纠正结构化记忆 + 有界三工具推荐 Agent + 确定性 Agent 评测**闭环：服务端持久化 session/profile 并在 filter 解析层补空；推荐入口 SSE 回传状态与最终回答；回合末抽取偏好补丁 CAS 合并；`cmd/eval-agent` 对 10～15 条场景跑 outcome / groundedness / trajectory / 多 trial，并与 **Hybrid RAG 生产口径**对照。**不做** Mem0/LangGraph/记忆工具/dense A/B。

技术路线对齐源计划 `docs/plans/2026-07-11-recommend-agent-eval.md` §1.2–1.4 / §1.7–1.8 / Tasks 4–8，验收以本规格为准。

## Technical Context

**Language/Version**: Go 1.24+

**Primary Dependencies**: Gin、go-redis/v9、现有 `internal/llm`（需扩展 tool calling）、现有 `ShopSearchLogic` / `RAGLogic`、OpenTelemetry（已有 HTTP `otelgin`；本规格补齐 agent/tool/search spans）、现有 `ShopRepo` / `BlogRepo`

**Storage**: Redis（session List + profile Hash；key 经 `pkg/utils/redisx`）；MySQL（店铺/点评只读）；向量检索仍走 Redis Stack（001 已建）

**Testing**: `go test` 表驱动（profile merge、extract 解析、tool executor、bounded loop fake LLM、graders、handler SSE）；`make eval-agent`；`make demo-agent`；与 `make eval-rag-prod` 对照

**Target Platform**: macOS / Linux 开发机 + Docker Compose（MySQL + Redis Stack 7.4.0-v8）

**Project Type**: 单体 Go Web 服务 + 独立 CLI（`cmd/server`、`cmd/eval-agent`）；无新微服务

**Performance Goals**: 单次推荐默认预算：maxSteps=3、maxToolCalls=5、toolTimeout=10s、runTimeout=45s；catalog 数十家；Agent 正式集 10～15 题 + 5 关键题×3 trial 约 1 小时内可跑完

**Constraints**:
- 记忆读写 MUST 服务端控制；MUST NOT 暴露为模型工具（FR-006）
- 最终回答引用 shop_id MUST ⊆ 本轮 observed set；校验失败不得当成功发出（FR-008）
- 相同 tool+canonical args 拒绝重复（FR-007）
- SSE 仅 status/message/done/error；禁止隐藏思维链与原始 tool result（FR-005）
- Agent eval 硬门禁：outcome / groundedness / trajectory；开放式措辞质量非门禁
- 对照叙事：**Agent vs Hybrid RAG**（001 `hybrid_prod`）；禁止 dense vs hybrid 数字
- 不引入 Mem0 / Eino / LangGraph / Milvus / 用户事实向量记忆

**Scale/Scope**: 10～15 Agent 场景；3 只读领域工具；Phase 1 结构化记忆；一份 Agent 报告 + 对照说明 + demo 脚本 + `doc/AGENT_AND_EVAL.md`

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*
*Source: `.specify/memory/constitution.md` v1.1.1 (local-review-go)*

- [x] **I. Interview-First**: 可讲清「记忆非工具 + 有界 loop + groundedness 先校验再输出 + Agent vs Hybrid RAG 评测」；面试卖点：① 可纠正 profile CAS ② 三工具 bounded Agent ③ 确定性 eval + 多 trial
- [x] **II. Layered Architecture**: `AgentHandler → RecommendAgentLogic → MemoryRepo / ShopSearchLogic / ShopRepo / BlogRepo / ToolChatClient`；`MemoryRepo` 接口在 `internal/repository/interface/`；Redis key 进 `pkg/utils/redisx/keys.go`；Handler 无业务逻辑
- [x] **III. Plan → Build → Test**: quickstart 定义 eval-agent / demo-agent / 单测矩阵；Task 5（eval 定义）先于 Task 6（Agent 核心）对齐源计划
- [x] **IV. Explain-as-You-Build**: 实现期按 AGENTS.md §5.8 解释；计划产出 quickstart + contracts 供口述
- [x] **V. Simplicity**: 自研 bounded loop，不用 LangGraph/Deep Agents；结构化 Hash 记忆，不上向量记忆 Phase 2；复杂度见 Complexity Tracking
- [x] **VI. Knowledge Capture**: 实现期新增/更新 `docs/solutions/interview/`（记忆非工具、有界 Agent、Agent eval、Agent vs Hybrid RAG）；blockers 按需

**Post-design re-check (Phase 1)**: 通过 — contracts 与 data-model 未引入额外框架或第二向量库；eval 与 demo 路径可测。

## Project Structure

### Documentation (this feature)

```text
specs/002-recommend-agent-memory/
├── plan.md              # This file
├── research.md          # Phase 0
├── data-model.md        # Phase 1
├── quickstart.md        # Phase 1
├── contracts/           # Phase 1
│   ├── agent-recommend-api.md
│   ├── memory-repo.md
│   ├── agent-tools.md
│   └── eval-agent-cli.md
└── tasks.md             # Phase 2 (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
cmd/
├── server/main.go                      # 注入 MemoryRepo + RecommendAgentLogic + AgentHandler
└── eval-agent/
    ├── main.go                         # Agent eval harness
    ├── types.go
    ├── graders.go
    ├── graders_test.go
    └── main_test.go

internal/
├── memory/
│   ├── types.go                        # Profile, Message, ProfilePatch
│   ├── profile.go                      # MergeProfile, MergeFilterWithProfile
│   ├── extract.go                      # LLM patch 解析（纯函数 + 注入）
│   ├── profile_test.go
│   └── extract_test.go
├── agent/
│   ├── types.go                        # RunConfig, ToolCall, AssistantTurn
│   ├── tools.go                        # 3 工具 schema + executor
│   ├── loop.go                         # bounded loop + dedupe + budgets
│   ├── trace.go                        # OTel span helpers
│   ├── tools_test.go
│   └── loop_test.go
├── logic/
│   ├── shop_search_logic.go            # 改：ResolveFilter 加 profile 补空
│   ├── recommend_agent_logic.go        # 门面：load → loop → groundedness → persist
│   └── recommend_agent_logic_test.go
├── llm/
│   └── client.go                       # 扩展 ToolChatClient / ChatWithTools
├── repository/
│   ├── interface/memory.go
│   └── memory_repo.go
├── handler/
│   ├── agent.go                        # POST /api/agent/recommend SSE
│   ├── agent_test.go
│   └── router.go                       # authGroup 注册
└── middleware/
    └── agent_rate_limit.go

pkg/utils/redisx/keys.go                # agent:sess:*, agent:profile:*

rag-evals/
├── golden/agent.v1.json                # 10～15 场景
└── reports/agent_latest.json           # gitignore 产物

script/agent-demo.sh
doc/AGENT_AND_EVAL.md
Makefile                                # eval-agent, demo-agent
```

**Structure Decision**: 延续单体分层；`internal/agent` 为纯编排与工具执行；`internal/memory` 为领域类型与 merge 纯函数；持久化经 `MemoryRepo`；评测 CLI 与 001 `eval-rag` 并列。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| 自研 bounded tool-loop（相对单次 RAG 一次检索+生成） | Spec US2/US3：多步 search→detail/blogs + 预算/去重/groundedness | 单次 Hybrid RAG 无法表达动态工具选择与轨迹评测；LangGraph 等框架叙事弱且 YAGNI |
| Profile CAS + 补丁语义（相对每次全量覆盖） | Spec US1/FR-002：可纠正、并发安全、删除优先 | 全量覆盖无法表达「忘掉预算」；无 version 会丢并发更新 |
| 先完整生成再校验 groundedness 再 SSE message（相对流式边生成边发） | FR-008：幻觉 shop_id 不可撤回 | 流式先发后校验无法撤销已发送 token |
| LLM tool calling 扩展（相对纯 prompt JSON 协议） | 模型原生 function call 更稳、可测 | 纯文本协议解析 fragile、难对齐 OpenAI 兼容 API |

## Implementation Phases (for /speckit-tasks)

对齐源计划 Tasks 4–8（细节以 tasks.md 为准）：

| Phase | Source Task | 交付物 | 依赖 |
|-------|-------------|--------|------|
| P3 Memory | Task 4 | `internal/memory/*`, `MemoryRepo`, `ShopSearchLogic` profile 补空, RAG 可选注入 | 001 ShopSearchLogic |
| P4 Eval 定义 | Task 5 | `agent.v1.json`, `cmd/eval-agent/graders*` | 无（可先测 grader） |
| P4 Agent 核心 | Task 6 | `internal/agent/*`, `RecommendAgentLogic`, LLM tool calling | P3 + P4 graders |
| P5 接口与观测 | Task 7 | `AgentHandler`, SSE, rate limit, OTel spans | P4 |
| P5 回归与文档 | Task 8 | `eval-agent` harness, demo, `doc/AGENT_AND_EVAL.md`, Makefile | P3–P7 + 001 hybrid_prod |

**推荐执行顺序**: Task 5（graders + golden）与 Task 4（memory）可并行；Task 6 依赖 4；Task 7–8 顺序收尾。

## Phase 0 / Phase 1 Artifacts

- [research.md](./research.md) — 记忆模型、Agent loop、groundedness、eval、OTel、LLM tools 决策
- [data-model.md](./data-model.md) — Profile / Session / AgentCase / AgentReport 等
- [contracts/](./contracts/) — HTTP SSE、MemoryRepo、三工具、eval CLI
- [quickstart.md](./quickstart.md) — 本地验证与对照步骤

## Definition of Done (maps to spec SC-*)

- [ ] `agent.v1.json` 10～15 条，关键类覆盖；SHA-256 写入 report
- [ ] `/api/agent/recommend`：登录、SSE、三工具、预算/去重/限流
- [ ] session/profile TTL；补丁 merge + CAS；RAG/Agent 共用 profile 补空
- [ ] groundedness 100% 于成功口径；断连不写 profile
- [ ] `make eval-agent` 报告含 outcome/groundedness/trajectory/trials/成本；5 关键题×≥3 trial
- [ ] Agent vs Hybrid RAG 对照说明（固定元数据）；无 dense 数字
- [ ] `make demo-agent` 三轮可复现；OTel spans 无隐私正文
- [ ] `go test ./... -count=1` + eval-agent + demo-agent 通过
- [ ] `docs/solutions/interview/` 至少更新 2 条 Agent 相关条目
