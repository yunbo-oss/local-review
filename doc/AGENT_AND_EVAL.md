# Agent 推荐与评测说明

**Feature**: 002 / 003 | **更新日期**: 2026-07-26

## 1. 两条路径

| 路径 | 入口 | 适用 |
|------|------|------|
| Hybrid RAG oneshot | `POST /api/rag/chat` 或 `POST /api/recommend`（路由=`rag_oneshot`） | 单轮清晰推荐 |
| Recommend Agent | `POST /api/agent/recommend` 或统一入口路由到 `agent_*` | 多步工具、记忆纠偏好 |

生产分流见 `RecommendRouter`（规则 + 可选 `force_route`）。**不是**通用 NLU。

## 2. Agent 运行时要点

- 三工具：`search_shops` / `get_shop` / `list_shop_blogs`（只读）
- 记忆：**不**暴露为模型工具；服务端注入 profile 摘要 + 成功路径 merge
- EvidenceLedger + Verify：引用 ⊆ 本轮可引用集；空评价不得洗白 ID
- 有界：max steps / tool attempts / per-turn cap
- SSE：`status` → `message`（仅校验通过）→ `done{trace_id,route,...}`；失败仅 `error` 公共文案

### Function calling 不可用

若网关/模型不支持 tools，`cmd/server` 不会注册 Agent Handler；`eval-agent --mode=inprocess` 会 fatal：

```text
ToolChatClient 不可用：当前模型/网关可能不支持 function calling
```

处理：更换支持 function calling 的 chat 模型，或先用 `--mode=fake` 验证 harness/报告形状（**不可**当正式分数）。

## 3. 评测（eval-agent）

```bash
# harness 冒烟（不调 LLM，报告 version=agent-eval.v1，无 stub）
go run ./cmd/eval-agent --mode=fake --split=test --trials=3 \
  --out=rag-evals/reports/agent_latest.json \
  --compare-baseline=rag-evals/baseline/hybrid_prod_v1.json \
  --force-route=agent_multistep

# 正式（需 LLM_API_KEY + Redis Stack + MySQL + seed-vector）
make eval-agent
# 等价：
# go run ./cmd/eval-agent --mode=inprocess --split=test \
#   --out=rag-evals/reports/agent_latest.json \
#   --compare-baseline=rag-evals/baseline/hybrid_prod_v1.json
```

报告主数字：

- 分 tag `tag_outcome_rates` / `comparison.per_tag_outcome_delta`
- `summary.pass_at_k_rate`（≥3 trial 全过占比）
- `p50_latency_ms` / `avg_tokens` / `avg_tool_calls`
- 配角：`groundedness_rate`、`trajectory_pass_rate`
- `n_infra_error` 单列，不进质量分母

对照：**仅 Agent vs Hybrid RAG**。禁止把 dense vs hybrid 写进 Agent 简历主叙事。

### Hybrid 基线

```bash
make eval-rag-prod
# 写入正式基线：
go run ./cmd/eval-rag --filter-mode=llm --retriever=hybrid --split=test --write-baseline \
  --baseline=rag-evals/baseline/hybrid_prod_v1.json
```

仓库内 placeholder 基线的 `hit_rate_at_k=0` **不得**写进简历；替换后再引用 Δ。

## 4. 演示（demo-agent）

```bash
make seed && make seed-redis   # 验证码 13800138000 / 123456（TTL 短，临近执行）
make run                      # 或分布式 80 端口：BASE_URL=http://localhost:80
make demo-agent
```

三轮：写偏好 → 同 session 推荐 → 忘掉预算。抽检关注 `trace_id` / `route`，勿贴完整评价正文。

## 5. 为何不做的事

| 不做 | 原因 |
|------|------|
| Mem0 / 记忆工具给模型 | 偏好需服务端确定性 merge；见面试笔记 memory-not-as-tools |
| dense vs hybrid 作为 Agent 价值证明 | Agent 价值在多步+记忆+groundedness，检索仍共用 Hybrid |
| stub 报告当正式结果 | `version` 含 stub 会被拒绝；fake 模式仅测 harness |

## 6. Trace 笔记（成功 / 失败）

- **成功**：`done` 含 `trace_id`；MySQL `agent_runs.status=COMPLETED`；`grounding_status=ok`
- **Grounding 失败**：SSE 仅 `error`，无 `message`；run=`FAILED`；不写 profile
- **断连**：`CANCELLED`；不写偏好/会话
