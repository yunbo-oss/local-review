#!/usr/bin/env bash
# API 接口功能测试脚本
# 用法: ./script/api-test.sh [BASE_URL]
# 示例: ./script/api-test.sh http://localhost:8088
#       ./script/api-test.sh http://localhost:80   # 分布式 Nginx
#
# 前置: make seed && make seed-redis（测试用户 13800138000、验证码 123456、秒杀券 6/7/8）

set -euo pipefail
BASE_URL="${1:-http://localhost:8088}"
API="${BASE_URL}/api"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass() { echo -e "${GREEN}✓ $1${NC}"; }
fail() { echo -e "${RED}✗ $1${NC}"; exit 1; }
info() { echo -e "${YELLOW}→ $1${NC}"; }
sse_message_text() {
  awk '
    /^event:/ { event_name=$0; sub(/^event:[[:space:]]*/, "", event_name); next }
    /^data:/ && event_name == "message" {
      line=$0
      sub(/^data:[[:space:]]?/, "", line)
      printf "%s", line
    }
  ' "${1:--}"
}

# 检查服务是否运行
info "检查服务: $BASE_URL"
if curl -sf "${BASE_URL}/ping" > /dev/null 2>&1; then
  pass "GET /ping"
elif curl -sf "${BASE_URL}/health" > /dev/null 2>&1; then
  pass "GET /health（分布式可用）"
else
  fail "服务未启动，请先运行: make run 或 docker compose up -d"
fi

echo ""
info "========== 1. 基础检查 =========="

# 健康检查（分布式部署有）
if curl -sf "${BASE_URL}/health" > /dev/null 2>&1; then
  resp=$(curl -sf "${BASE_URL}/health")
  [[ "$resp" == *"postgres"* ]] && [[ "$resp" == *"redis"* ]] && pass "GET /health" || fail "GET /health"
fi

echo ""
info "========== 2. 公开接口（无需登录）=========="

# 店铺类型列表
resp=$(curl -sf "${API}/shop-type/list")
[[ "$resp" == *"success"* ]] && pass "GET /api/shop-type/list" || fail "GET /api/shop-type/list"

# 热门博客
resp=$(curl -sf "${API}/blog/hot")
[[ "$resp" == *"success"* ]] && pass "GET /api/blog/hot" || fail "GET /api/blog/hot"

# UV 统计
DATE=$(date +%Y%m%d)
resp=$(curl -sf "${API}/statistics/uv?date=${DATE}")
[[ "$resp" == *"success"* ]] && pass "GET /api/statistics/uv" || fail "GET /api/statistics/uv"

# 当前 UV
resp=$(curl -sf "${API}/statistics/uv/current")
[[ "$resp" == *"success"* ]] && pass "GET /api/statistics/uv/current" || fail "GET /api/statistics/uv/current"

# 发送验证码（会生成新验证码并写入 Redis）
resp=$(curl -sf -X POST "${API}/user/code?phone=13800138000")
[[ "$resp" == *"success"* ]] && pass "POST /api/user/code" || fail "POST /api/user/code"

echo ""
info "========== 3. 登录流程 =========="

# 获取 Redis 中的验证码（优先用 /code 刚生成的；若无则用 seed-redis 的 123456）
CODE=""
if command -v redis-cli &> /dev/null; then
  REDIS_HOST="${REDIS_ADDR:-127.0.0.1}"
  REDIS_PORT_VALUE="${REDIS_PORT:-6379}"
  REDIS_PASS="${REDIS_PASSWORD:-8888.216}"
  CODE=$(redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT_VALUE" -a "$REDIS_PASS" \
    GET "login:code:13800138000" 2>/dev/null | tr -d '"')
elif docker compose ps --status running redis >/dev/null 2>&1; then
  CODE=$(docker compose exec -T redis redis-cli -a "${REDIS_PASSWORD:-8888.216}" \
    GET "login:code:13800138000" 2>/dev/null | tr -d '"')
fi

# 若未设置验证码，使用 seed-redis 中的默认值 123456
if [[ -z "$CODE" ]]; then
  CODE="123456"
  info "使用默认验证码 123456（若失败请执行 make seed-redis）"
fi

if [[ -n "$CODE" ]]; then
  resp=$(curl -sf -X POST "${API}/user/login" \
    -H "Content-Type: application/json" \
    -d "{\"phone\":\"13800138000\",\"code\":\"$CODE\"}")

  if [[ "$resp" == *"success"* ]] && [[ "$resp" == *"data"* ]]; then
    pass "POST /api/user/login"
    TOKEN=$(echo "$resp" | grep -o '"data":"[^"]*"' | cut -d'"' -f4)

    if [[ -n "$TOKEN" ]]; then
      echo ""
      info "========== 4. 需登录接口 =========="

      # 获取当前用户
      resp=$(curl -sf "${API}/user/me" -H "authorization: $TOKEN")
      [[ "$resp" == *"success"* ]] && pass "GET /api/user/me" || fail "GET /api/user/me"

      # 店铺列表（按类型）
      resp=$(curl -sf "${API}/shop/of/type?typeId=1&current=1" -H "authorization: $TOKEN")
      [[ "$resp" == *"success"* ]] && pass "GET /api/shop/of/type" || fail "GET /api/shop/of/type"

      # 店铺详情（布隆过滤器、缓存）
      resp=$(curl -sf "${API}/shop/1" -H "authorization: $TOKEN")
      [[ "$resp" == *"success"* ]] && pass "GET /api/shop/:id" || fail "GET /api/shop/:id"

      # 优惠券列表
      resp=$(curl -sf "${API}/voucher/list/1" -H "authorization: $TOKEN")
      [[ "$resp" == *"success"* ]] && pass "GET /api/voucher/list/:shopId" || fail "GET /api/voucher/list/:shopId"

      # 秒杀可重复冒烟：首次可能排队成功，后续可能已抢购/库存不足，限流时为 429。
      # 不使用 curl -f，否则 set -e 会在读取预期的 4xx 业务响应前提前退出。
      seckill_response=$(curl -sS -w $'\n%{http_code}' -X POST \
        "${API}/voucher-order/seckill/6" -H "authorization: $TOKEN")
      http_code="${seckill_response##*$'\n'}"
      resp="${seckill_response%$'\n'*}"
      if [[ "$http_code" == "200" ]] || [[ "$http_code" == "409" ]] \
        || [[ "$http_code" == "429" ]] \
        || [[ "$resp" == *"排队中"* ]] || [[ "$resp" == *"限流"* ]] \
        || [[ "$resp" == *"已抢购"* ]] || [[ "$resp" == *"库存"* ]]; then
        pass "POST /api/voucher-order/seckill/:id (HTTP $http_code)"
      else
        fail "POST /api/voucher-order/seckill/:id (HTTP $http_code)"
      fi

      # 我的博客
      resp=$(curl -sf "${API}/blog/of/me?current=1" -H "authorization: $TOKEN")
      [[ "$resp" == *"success"* ]] && pass "GET /api/blog/of/me" || fail "GET /api/blog/of/me"

      # 关注状态
      resp=$(curl -sf "${API}/follow/or/not/1" -H "authorization: $TOKEN")
      [[ "$resp" == *"success"* ]] && pass "GET /api/follow/or/not/:id" || fail "GET /api/follow/or/not/:id"

      # Router clarify 路径不调用模型；缺少历史的指代应直接要求补充信息。
      clarify_sse=$(curl -sfS -N -X POST "${API}/recommend" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d '{"question":"还是上次那种","session_id":"api-smoke-missing-history"}')
      [[ "$clarify_sse" == *'"route":"clarify"'* ]] \
        && [[ "$clarify_sse" == *"event:message"* ]] \
        && [[ "$clarify_sse" == *"event:done"* ]] \
        && [[ "$clarify_sse" != *"event:error"* ]] \
        && pass "POST /api/recommend (Router clarify)" || fail "POST /api/recommend 未执行 clarify"

      if [[ -n "${LLM_API_KEY:-}" ]]; then
        echo ""
        info "========== 5. RAG / Agent / 引用 =========="

        rag_sse=$(curl -sfS -N -X POST "${API}/rag/chat" \
          -H "Authorization: Bearer $TOKEN" \
          -H "Content-Type: application/json" \
          -d '{"question":"找「静巷咖啡·国贸店」，说明价格和安静办公依据"}')
        [[ "$rag_sse" == *"event:done"* ]] && [[ "$rag_sse" != *"event:error"* ]] \
          && pass "POST /api/rag/chat (SSE)" || fail "POST /api/rag/chat (SSE)"
        rag_text=$(printf '%s\n' "$rag_sse" | sse_message_text)
        [[ "$rag_text" =~ \[shop:[0-9]+\] ]] \
          && pass "RAG [shop:id] 引用" || fail "RAG 回答缺少 [shop:id] 引用"

        agent_sse=$(curl -sfS -N -X POST "${API}/agent/recommend" \
          -H "Authorization: Bearer $TOKEN" \
          -H "Content-Type: application/json" \
          -d '{"question":"只查《静巷咖啡·国贸店》，总结安静办公的评价依据","session_id":"api-smoke-agent"}')
        [[ "$agent_sse" == *"event:message"* ]] && [[ "$agent_sse" == *"event:done"* ]] \
          && [[ "$agent_sse" != *"event:error"* ]] \
          && pass "POST /api/agent/recommend (SSE)" || fail "POST /api/agent/recommend (SSE)"
        agent_text=$(printf '%s\n' "$agent_sse" | sse_message_text)
        [[ "$agent_text" =~ \[shop:[0-9]+\] ]] \
          && pass "Agent grounded citation" || fail "Agent 回答缺少 grounded citation"
      else
        info "LLM_API_KEY 未注入，跳过 RAG/Agent 实调"
      fi

      # 登出
      resp=$(curl -sf -X POST "${API}/user/logout" -H "authorization: $TOKEN")
      [[ "$resp" == *"success"* ]] && pass "POST /api/user/logout" || fail "POST /api/user/logout"
    fi
  else
    info "登录失败（验证码可能已过期），跳过需登录接口测试"
    info "提示: 执行 make seed-redis 重新设置验证码"
  fi
else
  info "无法获取验证码，跳过登录及需登录接口测试"
  info "提示: 执行 make seed-redis 或访问前端手动登录验证"
fi

echo ""
echo -e "${GREEN}========== 功能测试完成 ==========${NC}"
