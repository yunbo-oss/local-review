#!/usr/bin/env bash
# API 接口冒烟测试脚本
# 用法: ./script/api-test.sh [BASE_URL]
# 示例: ./script/api-test.sh http://localhost:8088

set -e
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

# 检查服务是否运行
info "检查服务: $BASE_URL"
if ! curl -sf "${BASE_URL}/ping" > /dev/null; then
  fail "服务未启动，请先运行: make run 或 make air"
fi
pass "服务已启动"

echo ""
info "========== 公开接口测试（无需登录）=========="

# 1. Ping
resp=$(curl -sf "${BASE_URL}/ping")
[[ "$resp" == *"pong"* ]] && pass "GET /ping" || fail "GET /ping"

# 2. 店铺类型列表
resp=$(curl -sf "${API}/shop-type/list")
[[ "$resp" == *"success"* ]] && pass "GET /api/shop-type/list" || fail "GET /api/shop-type/list"

# 3. 热门博客
resp=$(curl -sf "${API}/blog/hot")
[[ "$resp" == *"success"* ]] && pass "GET /api/blog/hot" || fail "GET /api/blog/hot"

# 4. UV 统计（需传 date 参数，格式 YYYYMMDD）
DATE=$(date +%Y%m%d)
resp=$(curl -sf "${API}/statistics/uv?date=${DATE}")
[[ "$resp" == *"success"* ]] && pass "GET /api/statistics/uv" || fail "GET /api/statistics/uv"

# 5. 当前 UV
resp=$(curl -sf "${API}/statistics/uv/current")
[[ "$resp" == *"success"* ]] && pass "GET /api/statistics/uv/current" || fail "GET /api/statistics/uv/current"

# 6. 发送验证码
resp=$(curl -sf -X POST "${API}/user/code?phone=13800138000")
[[ "$resp" == *"success"* ]] && pass "POST /api/user/code" || fail "POST /api/user/code"

echo ""
info "========== 登录流程测试 =========="

# 获取 Redis 中的验证码（需要 Redis 可访问）
CODE=""
if command -v redis-cli &> /dev/null; then
  CODE=$(redis-cli -a 8888.216 GET "login:code:13800138000" 2>/dev/null | tr -d '"')
elif docker ps --format '{{.Names}}' 2>/dev/null | grep -q local-review-redis; then
  CODE=$(docker exec local-review-redis redis-cli -a 8888.216 GET "login:code:13800138000" 2>/dev/null | tr -d '"')
fi

if [[ -n "$CODE" ]]; then
  # 7. 登录
  resp=$(curl -sf -X POST "${API}/user/login" \
    -H "Content-Type: application/json" \
    -d "{\"phone\":\"13800138000\",\"code\":\"$CODE\"}")
  
  if [[ "$resp" == *"success"* ]] && [[ "$resp" == *"data"* ]]; then
    pass "POST /api/user/login"
    # 提取 token（简单方式，兼容无 jq）
    TOKEN=$(echo "$resp" | grep -o '"data":"[^"]*"' | cut -d'"' -f4)
    
    if [[ -n "$TOKEN" ]]; then
      echo ""
      info "========== 需登录接口测试 =========="
      
      # 8. 获取当前用户
      resp=$(curl -sf "${API}/user/me" -H "authorization: $TOKEN")
      [[ "$resp" == *"success"* ]] && pass "GET /api/user/me" || fail "GET /api/user/me"
      
      # 9. 店铺类型分页（需要 typeId，用 1 测试）
      resp=$(curl -sf "${API}/shop/of/type?typeId=1&current=1" -H "authorization: $TOKEN")
      [[ "$resp" == *"success"* ]] && pass "GET /api/shop/of/type" || fail "GET /api/shop/of/type"
      
      # 10. 登出
      resp=$(curl -sf -X POST "${API}/user/logout" -H "authorization: $TOKEN")
      [[ "$resp" == *"success"* ]] && pass "POST /api/user/logout" || fail "POST /api/user/logout"
    fi
  else
    info "登录失败（可能验证码已过期），跳过需登录接口测试"
  fi
else
  info "无法获取验证码（需 redis-cli 或 Docker），跳过登录及需登录接口测试"
  info "提示: 可访问 http://localhost:8088 用前端手动登录验证"
fi

echo ""
echo -e "${GREEN}========== 冒烟测试完成 ==========${NC}"
