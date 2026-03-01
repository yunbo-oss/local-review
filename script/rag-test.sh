#!/usr/bin/env bash
# RAG 智能点评助手功能测试脚本
# 用法: ./script/rag-test.sh [BASE_URL]
# 示例: ./script/rag-test.sh http://localhost:8088
#       ./script/rag-test.sh http://localhost:80   # 分布式 Nginx
#
# 前置条件:
#   1. 服务已启动 (make run 或 docker compose up -d)
#   2. make seed && make seed-redis（测试用户 13800138000、验证码 123456）
#   3. make seed-vector（需 LLM_API_KEY + Redis Stack，导入店铺向量）
#   4. 环境变量 LLM_API_KEY 已配置（服务启动时需有，否则 RAG 不可用）

set -e
BASE_URL="${1:-http://localhost:8088}"
API="${BASE_URL}/api"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

pass() { echo -e "${GREEN}✓ $1${NC}"; }
fail() { echo -e "${RED}✗ $1${NC}"; exit 1; }
info() { echo -e "${YELLOW}→ $1${NC}"; }
question() { echo -e "${CYAN}  Q: $1${NC}"; }

# 解析 SSE 流式响应，收集 message 事件内容，检测 error 事件
# 返回: 0 成功，1 有 error 事件
parse_rag_response() {
  local full=""
  local has_error=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" == data:* ]]; then
      local chunk="${line#data: }"
      # 检查上一个 event 类型（简化：data 前一行通常是 event）
      # 实际 SSE: event: message \n data: xxx，我们按顺序处理
      # 若遇到 error 的 data，则标记
      if [[ "$chunk" == *"error"* ]] && [[ "$prev_event" == "error" ]]; then
        has_error=1
      fi
      full+="$chunk"
    elif [[ "$line" == event:* ]]; then
      prev_event="${line#event: }"
      prev_event="${prev_event// /}"
      if [[ "$prev_event" == "error" ]]; then
        has_error=1
      fi
    fi
  done
  echo "$full"
  return $has_error
}

# 调用 RAG Chat 并返回完整回复（通过临时文件传递）
call_rag_chat() {
  local token="$1"
  local question="$2"
  local filter_json="${3:-}"
  local tmpfile=$(mktemp)
  local http_code
  local has_error=0
  local body

  # 构建 JSON body（转义 question 中的 " 和 \）
  local q_escaped
  q_escaped=$(printf '%s' "$question" | sed 's/\\/\\\\/g; s/"/\\"/g')
  if [[ -n "$filter_json" ]]; then
    body="{\"question\":\"$q_escaped\",\"filter\":$filter_json}"
  else
    body="{\"question\":\"$q_escaped\"}"
  fi

  http_code=$(curl -s -o "$tmpfile" -w "%{http_code}" -N -X POST "${API}/rag/chat" \
    -H "Content-Type: application/json" \
    -H "authorization: $token" \
    -d "$body" \
    --max-time 60)

  if [[ "$http_code" != "200" ]]; then
    echo "HTTP $http_code"
    cat "$tmpfile" | head -20
    rm -f "$tmpfile"
    return 1
  fi

  # 解析 SSE：收集所有 data 行（message 事件的内容）
  local full_response=""
  local prev_event=""
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" == event:* ]]; then
      prev_event="${line#event: }"
      prev_event="${prev_event// /}"
    elif [[ "$line" == data:* ]]; then
      # 兼容 "data: xxx" 与 "data:xxx" 两种格式，去除 data: 前缀
      local chunk="${line#data:}"
      chunk="${chunk# }"  # 去除可能的首空格
      if [[ "$prev_event" == "error" ]]; then
        has_error=1
      else
        full_response+="$chunk"
      fi
    fi
  done < "$tmpfile"
  rm -f "$tmpfile"

  echo "$full_response"
  [[ $has_error -eq 1 ]] && return 1 || return 0
}

echo ""
info "========== RAG 智能点评助手测试 =========="
echo "BASE_URL: $BASE_URL"
echo ""

# 1. 检查服务
info "1. 检查服务是否运行"
if curl -sf "${BASE_URL}/ping" > /dev/null 2>&1; then
  pass "GET /ping"
elif curl -sf "${BASE_URL}/health" > /dev/null 2>&1; then
  pass "GET /health"
else
  fail "服务未启动，请先运行: make run 或 docker compose up -d"
fi

# 2. 获取验证码并登录
info "2. 登录获取 Token"
CODE="123456"
if command -v redis-cli &> /dev/null; then
  c=$(redis-cli -a 8888.216 GET "login:code:13800138000" 2>/dev/null | tr -d '"')
  [[ -n "$c" ]] && CODE="$c"
elif docker ps --format '{{.Names}}' 2>/dev/null | grep -q local-review-redis; then
  c=$(docker exec local-review-redis redis-cli -a 8888.216 GET "login:code:13800138000" 2>/dev/null | tr -d '"')
  [[ -n "$c" ]] && CODE="$c"
fi

# 发送验证码（确保有效）
curl -sf -X POST "${API}/user/code?phone=13800138000" > /dev/null 2>&1 || true
sleep 1
if docker ps --format '{{.Names}}' 2>/dev/null | grep -q local-review-redis; then
  CODE=$(docker exec local-review-redis redis-cli -a 8888.216 GET "login:code:13800138000" 2>/dev/null | tr -d '"')
  [[ -z "$CODE" ]] && CODE="123456"
fi

login_resp=$(curl -sf -X POST "${API}/user/login" \
  -H "Content-Type: application/json" \
  -d "{\"phone\":\"13800138000\",\"code\":\"$CODE\"}" 2>/dev/null || echo "")

if [[ "$login_resp" != *"success"* ]] || [[ "$login_resp" != *"data"* ]]; then
  fail "登录失败，请执行 make seed-redis 或检查验证码"
fi

TOKEN=$(echo "$login_resp" | grep -o '"data":"[^"]*"' | cut -d'"' -f4)
if [[ -z "$TOKEN" ]]; then
  fail "无法解析 Token"
fi
pass "登录成功"

echo ""
info "3. RAG 智能点评接口测试"
echo ""

# 测试用例列表：问题 | 预期包含关键词（可选，用于简单校验）
# 格式: "问题" 或 "问题|关键词1|关键词2"
TESTS=(
  "推荐一家朝阳区好吃的"
  "海淀区有什么咖啡店"
  "人均100以内的火锅"
  "西城区评分高的店铺"
  "东城区有什么好吃的"
)

PASSED=0
FAILED=0

for i in "${!TESTS[@]}"; do
  item="${TESTS[$i]}"
  q="${item%%|*}"
  idx=$((i + 1))
  echo -e "${CYAN}---------- 测试 $idx ----------${NC}"
  question "$q"

  response=$(call_rag_chat "$TOKEN" "$q") || true
  exit_code=$?

  if [[ $exit_code -ne 0 ]]; then
    fail "RAG 请求失败或返回错误"
    echo "响应摘要: ${response:0:200}..."
    FAILED=$((FAILED + 1))
  elif [[ -z "$response" ]] || [[ ${#response} -lt 5 ]]; then
    fail "RAG 返回内容过短或为空"
    echo "响应: $response"
    FAILED=$((FAILED + 1))
  else
    pass "RAG 返回成功 (${#response} 字符)"
    # 打印回复摘要（前 150 字）
    echo -e "  ${GREEN}回复摘要: ${response:0:150}...${NC}"
    PASSED=$((PASSED + 1))
  fi
  echo ""
done

# 4. 测试带 filter 的请求
info "4. 测试带预过滤的请求"
echo ""
question "推荐店铺（filter: 朝阳区 + 美食）"

filter_json='{"area":"朝阳区","typeName":"美食"}'
# 注意：JSON 中中文需正确传递
response=$(call_rag_chat "$TOKEN" "有什么好吃的" "$filter_json") || true
exit_code=$?

if [[ $exit_code -ne 0 ]]; then
  fail "带 filter 的 RAG 请求失败"
  FAILED=$((FAILED + 1))
elif [[ -z "$response" ]] || [[ ${#response} -lt 5 ]]; then
  fail "带 filter 的 RAG 返回内容过短"
  FAILED=$((FAILED + 1))
else
  pass "带 filter 的 RAG 返回成功"
  echo -e "  ${GREEN}回复摘要: ${response:0:150}...${NC}"
  PASSED=$((PASSED + 1))
fi

echo ""
echo -e "${GREEN}========== RAG 测试完成 ==========${NC}"
echo -e "通过: ${GREEN}$PASSED${NC}  失败: ${RED}$FAILED${NC}"
echo ""
if [[ $FAILED -gt 0 ]]; then
  echo "若出现「暂无相关店铺数据」或「RAG 服务未配置」，请检查："
  echo "  1. 是否已执行 make seed-vector（需 LLM_API_KEY + Redis Stack）"
  echo "  2. 服务启动时是否配置了 LLM_API_KEY"
  echo "  3. Redis 是否为 Redis Stack（支持 RediSearch 向量索引）"
  exit 1
fi
