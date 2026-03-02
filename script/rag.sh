#!/usr/bin/env bash
# RAG 智能点评：初始化 / 展示 / 删除索引
# 用法: ./script/rag.sh [--init|--drop-index|--demo] [BASE_URL]
#   --init       一键初始化（seed + seed-redis + seed-vector + 后台启动服务）
#   --drop-index 删除向量索引（schema 变更后，再 make seed-vector 重建）
#   --demo       展示（3 个问题，流式输出），默认
# 示例: ./script/rag.sh
#       ./script/rag.sh --init
#       ./script/rag.sh http://localhost:80
#
# 前置（非 --init）: make seed && make seed-redis && make seed-vector && make run

set -e
MODE="--demo"
BASE_URL="http://localhost:8088"
[[ "$1" == "--init" ]] && MODE="--init" && shift
[[ "$1" == "--drop-index" ]] && MODE="--drop-index" && shift
[[ "$1" == "--demo" ]] && shift
[[ -n "$1" ]] && [[ "$1" != --* ]] && BASE_URL="$1"
API="${BASE_URL}/api"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

pass() { echo -e "${GREEN}✓ $1${NC}"; }
fail() { echo -e "${RED}✗ $1${NC}"; exit 1; }
info() { echo -e "${YELLOW}→ $1${NC}"; }
question() { echo -e "${CYAN}  Q: $1${NC}"; }

# ---------- --drop-index ----------
do_drop_index() {
  REDIS_CMD="${REDIS_CMD:-docker exec local-review-redis redis-cli -a 8888.216}"
  $REDIS_CMD FT.DROPINDEX idx:shop:vector DD 2>/dev/null || true
  echo "向量索引已删除，重启服务或执行 make seed-vector 将自动重建"
  exit 0
}

# ---------- --init ----------
do_init() {
  SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
  PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

  echo ""
  echo -e "${YELLOW}========== RAG 环境初始化 ==========${NC}"
  echo ""

  echo -e "${YELLOW}[1] 检查 Docker 容器${NC}"
  docker ps --format '{{.Names}}' 2>/dev/null | grep -q local-review-mysql || fail "MySQL 未运行，请先: docker compose up -d"
  docker ps --format '{{.Names}}' 2>/dev/null | grep -q local-review-redis || fail "Redis 未运行，请先: docker compose up -d"
  pass "Docker 容器已就绪"
  echo ""

  echo -e "${YELLOW}[2] 导入 MySQL 种子数据${NC}"
  cd "$PROJECT_ROOT" && make seed
  pass "种子数据已导入"
  echo ""

  echo -e "${YELLOW}[3] 初始化 Redis${NC}"
  make seed-redis
  pass "Redis 已初始化"
  echo ""

  echo -e "${YELLOW}[4] 导入店铺向量${NC}"
  make seed-vector || fail "向量导入失败，请检查 .env 中 LLM_API_KEY"
  pass "向量已导入"
  echo ""

  echo -e "${YELLOW}[5] 启动 Go 服务（后台）${NC}"
  cd "$PROJECT_ROOT"
  nohup make run > /tmp/local-review-go.log 2>&1 &
  SERVER_PID=$!
  echo "服务 PID: $SERVER_PID，日志: /tmp/local-review-go.log"

  echo -n "等待服务启动"
  for i in $(seq 1 30); do
    if curl -sf http://localhost:8088/ping > /dev/null 2>&1 || curl -sf http://localhost:8088/health > /dev/null 2>&1; then
      echo ""
      pass "服务已就绪 (http://localhost:8088)"
      break
    fi
    echo -n "."
    sleep 1
    [[ $i -eq 30 ]] && { echo ""; fail "服务启动超时，请查看 /tmp/local-review-go.log"; }
  done
  echo ""

  echo -e "${GREEN}========== 初始化完成 ==========${NC}"
  echo ""
  echo "面试时执行: make demo-rag"
  echo "停止服务: kill $SERVER_PID"
  echo ""
  exit 0
}

# ---------- 提前退出 ----------
[[ "$MODE" == "--drop-index" ]] && do_drop_index
[[ "$MODE" == "--init" ]] && do_init

# ---------- 登录获取 Token ----------
do_login() {
  curl -sf -X POST "${API}/user/code?phone=13800138000" > /dev/null 2>&1 || true
  sleep 1
  CODE="123456"
  if docker ps --format '{{.Names}}' 2>/dev/null | grep -q local-review-redis; then
    c=$(docker exec local-review-redis redis-cli -a 8888.216 GET "login:code:13800138000" 2>/dev/null | tr -d '"')
    [[ -n "$c" ]] && CODE="$c"
  fi
  login=$(curl -sf -X POST "${API}/user/login" -H "Content-Type: application/json" \
    -d "{\"phone\":\"13800138000\",\"code\":\"$CODE\"}")
  echo "$login" | grep -o '"data":"[^"]*"' | cut -d'"' -f4
}

parse_sse() {
  while IFS= read -r line; do
    [[ "$line" != data:* ]] && continue
    chunk="${line#data:}"
    echo -n "${chunk# }"
  done
}

stream_rag() {
  local token="$1" q="$2" filter="${3:-}" body q_esc
  q_esc=$(printf '%s' "$q" | sed 's/\\/\\\\/g; s/"/\\"/g')
  [[ -n "$filter" ]] && body="{\"question\":\"$q_esc\",\"filter\":$filter}" || body="{\"question\":\"$q_esc\"}"
  curl -s -N -X POST "${API}/rag/chat" \
    -H "Content-Type: application/json" -H "authorization: $token" -d "$body" | parse_sse
}

# ========== main (--demo) ==========
echo ""
info "========== RAG 智能点评 =========="
echo "BASE_URL: $BASE_URL"
echo ""

curl -sf "${BASE_URL}/ping" > /dev/null 2>&1 || curl -sf "${BASE_URL}/health" > /dev/null 2>&1 || fail "服务未启动，请先 make run"

info "登录获取 Token"
TOKEN=$(do_login)
[[ -z "$TOKEN" ]] && fail "登录失败，请执行 make seed-redis"
pass "登录成功"
echo ""

info "[1] 朝阳区适合情侣的浪漫餐厅"
question "朝阳区适合情侣的浪漫餐厅"
echo ""
stream_rag "$TOKEN" "朝阳区适合情侣的浪漫餐厅"
echo -e "\n"

info "[2] 带预过滤：朝阳区 + 美食"
question "有什么好吃的（filter: 朝阳区, 美食）"
echo ""
stream_rag "$TOKEN" "有什么好吃的" '{"area":"朝阳区","typeName":"美食"}'
echo -e "\n"

info "[3] 人均100以内的火锅"
question "人均100以内的火锅"
echo ""
stream_rag "$TOKEN" "人均100以内的火锅"
echo ""
pass "测试完成"
