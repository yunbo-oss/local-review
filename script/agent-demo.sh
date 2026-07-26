#!/usr/bin/env bash
# demo-agent：三轮记忆可复现演示（登录 → 写偏好 → 同 session 推荐 → 纠正预算）
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8088}"
PHONE="${DEMO_PHONE:-13800138000}"
CODE="${DEMO_CODE:-123456}"
SESSION_ID="${DEMO_SESSION_ID:-demo-$(date +%s)}"

redact() {
  sed -E 's/"content_snippet":"[^"]*"/"content_snippet":"[redacted]"/g; s/Bearer [A-Za-z0-9._-]+/Bearer [redacted]/g'
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

recommend() {
  local q="$1"
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
    -d "$body" | tee /tmp/agent-demo-sse.txt | redact
  echo ""
  if grep -q 'event:error' /tmp/agent-demo-sse.txt; then
    echo "    !! 出现 error 事件" >&2
  fi
  TRACE=$(grep -o '"trace_id":"[^"]*"' /tmp/agent-demo-sse.txt | head -1 || true)
  ROUTE=$(grep -o '"route":"[^"]*"' /tmp/agent-demo-sse.txt | head -1 || true)
  echo "    $TRACE $ROUTE"
}

# 1) 写偏好
recommend "我常在海淀活动，人均预算大概100，喜欢安静咖啡店"
# 2) 同 session 追问（应补全区域/预算）
recommend "推荐两家安静的咖啡店"
# 3) 纠正：忘掉预算
recommend "忘掉预算，再帮我看看朝阳区适合聚餐的餐厅"

echo ""
echo "==> demo 完成。请确认：偏好写入、同 session 补全、预算清空；SSE 无完整评价/密钥。"
echo "    session_id=$SESSION_ID"
echo "    脱敏 profile 请用 Redis HGETALL agent:profile:{userId} 或 MySQL user_agent_profiles（勿贴完整密钥）"
