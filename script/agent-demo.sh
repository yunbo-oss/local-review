#!/usr/bin/env bash
# demo-agent：三轮记忆可复现演示（登录 → 写偏好 → 同 session 推荐 → 纠正预算）
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8088}"
PHONE="${DEMO_PHONE:-13800138000}"
CODE="${DEMO_CODE:-123456}"
SESSION_ID="${DEMO_SESSION_ID:-demo-$(date +%s)}"
REPORT_PATH="${DEMO_REPORT_PATH:-rag-evals/reports/memory_demo_latest.json}"
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

redact() {
  sed -E 's/"content_snippet":"[^"]*"/"content_snippet":"[redacted]"/g; s/Bearer [A-Za-z0-9._-]+/Bearer [redacted]/g'
}

sse_message_text() {
  awk '
    /^event:/ { event_name=$0; sub(/^event:[[:space:]]*/, "", event_name); next }
    /^data:/ && event_name == "message" {
      line=$0
      sub(/^data:[[:space:]]?/, "", line)
      printf "%s", line
    }
  ' "$1"
}

echo "==> BASE_URL=$BASE_URL phone=$PHONE session=$SESSION_ID"
echo "==> 提示：请先 make seed && make seed-redis，并确保服务已启动且 LLM_API_KEY 可用"

LOGIN=$(curl -sS -X POST "$BASE_URL/api/user/login" \
  -H 'Content-Type: application/json' \
  -d "{\"phone\":\"$PHONE\",\"code\":\"$CODE\"}")
TOKEN=$(echo "$LOGIN" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("data") or "")' 2>/dev/null || true)
if [[ -z "$TOKEN" || "$TOKEN" == "None" ]]; then
  echo "登录失败（请确认 seed-redis 验证码仍有效），响应：" >&2
  echo "$LOGIN" | redact >&2
  exit 1
fi
echo "==> 已登录（token redacted）"

ME=$(curl -fsS "$BASE_URL/api/user/me" -H "Authorization: Bearer $TOKEN")
USER_ID=$(echo "$ME" | python3 -c 'import json,sys; d=json.load(sys.stdin); v=d.get("data") or {}; print(v.get("id") or v.get("Id") or "")')
[[ -n "$USER_ID" ]] || { echo "无法读取 demo user id" >&2; exit 1; }

# Compose 已清理该 demo 用户的 MySQL profile；这里同步清理 Redis cache、
# 固定 session 和限流窗口，使重复执行仍从同一初始状态开始。
redis-cli -h "${REDIS_ADDR:-redis}" -p "${REDIS_PORT:-6379}" \
  -a "${REDIS_PASSWORD:-8888.216}" --no-auth-warning DEL \
  "agent:profile:${USER_ID}" \
  "agent:profile:cache:${USER_ID}" \
  "agent:sess:${USER_ID}:${SESSION_ID}" \
  "agent:rl:${USER_ID}" >/dev/null

recommend() {
  local q="$1"
  local round="$2"
  local require_citation="$3"
  local sse_file="$TMP_DIR/round-${round}.sse"
  echo ""
  echo "==> recommend: $q"
  local body
  body=$(QUESTION="$q" SESSION_ID="$SESSION_ID" python3 - <<'PY'
import json, os
print(json.dumps({"question": os.environ["QUESTION"], "session_id": os.environ["SESSION_ID"]}))
PY
)
  curl -sS -N -X POST "$BASE_URL/api/agent/recommend" \
    -H "Authorization: Bearer $TOKEN" \
    -H 'Content-Type: application/json' \
    -d "$body" | tee "$sse_file" | redact
  echo ""
  if grep -q '^event:error' "$sse_file"; then
    echo "    !! round $round 出现 error 事件" >&2
    exit 1
  fi
  grep -q '^event:message' "$sse_file" || { echo "round $round 缺少 message 事件" >&2; exit 1; }
  grep -q '^event:done' "$sse_file" || { echo "round $round 缺少 done 事件" >&2; exit 1; }
  local message_text
  message_text=$(sse_message_text "$sse_file")
  if [[ "$require_citation" == "yes" ]]; then
    grep -Eq '\[shop:[0-9]+\]' <<<"$message_text" || {
      echo "round $round 推荐回答缺少 [shop:id] 引用" >&2
      exit 1
    }
  fi
  local trace route cites
  trace=$(grep -o '"trace_id":"[^"]*"' "$sse_file" | head -1 | cut -d'"' -f4 || true)
  route=$(grep -o '"route":"[^"]*"' "$sse_file" | head -1 | cut -d'"' -f4 || true)
  cites=$({ grep -Eo '\[shop:[0-9]+\]' <<<"$message_text" || true; } | sort -u | wc -l | tr -d ' ')
  [[ -n "$trace" && -n "$route" ]] || { echo "round $round 缺少 trace_id/route" >&2; exit 1; }
  eval "ROUND_${round}_TRACE=\$trace"
  eval "ROUND_${round}_ROUTE=\$route"
  eval "ROUND_${round}_CITES=\$cites"
  echo "    trace_id=$trace route=$route citations=$cites"
}

# 1) 写偏好（与正式 memory golden 使用同一可满足约束）
recommend "我以后优先海淀区、预算80元以内" 1 no
# 2) 同 session 追问（应自动补全区域/预算）
recommend "按我的偏好推荐一家适合学生的店" 2 yes
# 3) 纠正：忘掉预算并切换区域
recommend "忘掉预算，改为丰台区，再推荐一家美食类、适合家庭聚餐的店" 3 yes

PROFILE_JSON=$(redis-cli -h "${REDIS_ADDR:-redis}" -p "${REDIS_PORT:-6379}" \
  -a "${REDIS_PASSWORD:-8888.216}" --no-auth-warning GET "agent:profile:cache:${USER_ID}")
PROFILE_JSON="$PROFILE_JSON" python3 - <<'PY'
import json, os
p = json.loads(os.environ["PROFILE_JSON"])
if p.get("budget_max") not in (None, 0):
    raise SystemExit(f"三轮后 budget_max 未清空: {p.get('budget_max')}")
areas = p.get("preferred_areas") or []
if "丰台区" not in areas:
    raise SystemExit(f"三轮后未记录丰台区偏好: {areas}")
PY

mkdir -p "$(dirname "$REPORT_PATH")"
REPORT_PATH="$REPORT_PATH" SESSION_ID="$SESSION_ID" USER_ID="$USER_ID" PROFILE_JSON="$PROFILE_JSON" \
ROUND_1_TRACE="$ROUND_1_TRACE" ROUND_1_ROUTE="$ROUND_1_ROUTE" ROUND_1_CITES="$ROUND_1_CITES" \
ROUND_2_TRACE="$ROUND_2_TRACE" ROUND_2_ROUTE="$ROUND_2_ROUTE" ROUND_2_CITES="$ROUND_2_CITES" \
ROUND_3_TRACE="$ROUND_3_TRACE" ROUND_3_ROUTE="$ROUND_3_ROUTE" ROUND_3_CITES="$ROUND_3_CITES" \
python3 - <<'PY'
import json, os
rounds = []
for i in range(1, 4):
    rounds.append({
        "round": i,
        "trace_id": os.environ[f"ROUND_{i}_TRACE"],
        "route": os.environ[f"ROUND_{i}_ROUTE"],
        "citation_count": int(os.environ[f"ROUND_{i}_CITES"]),
        "sse_success": True,
    })
p = json.loads(os.environ["PROFILE_JSON"])
report = {
    "version": "memory-demo.v2",
    "session_id": os.environ["SESSION_ID"],
    "user_id": int(os.environ["USER_ID"]),
    "rounds": rounds,
    "final_profile": {
        "preferred_areas": p.get("preferred_areas") or [],
        "preferred_types": p.get("preferred_types") or [],
        "budget_max": p.get("budget_max"),
        "version": p.get("version"),
    },
    "checks": {
        "all_sse_success": True,
        "recommendation_citations_present": all(r["citation_count"] > 0 for r in rounds[1:]),
        "budget_cleared": p.get("budget_max") in (None, 0),
        "area_correction_recorded": "丰台区" in (p.get("preferred_areas") or []),
    },
}
with open(os.environ["REPORT_PATH"], "w", encoding="utf-8") as f:
    json.dump(report, f, ensure_ascii=False, indent=2)
    f.write("\n")
PY

echo ""
echo "==> demo 完成：3/3 SSE 成功、推荐轮含引用、预算已清空、丰台区偏好已记录。"
echo "    session_id=$SESSION_ID"
echo "    report=$REPORT_PATH"
