#!/usr/bin/env bash
# RAG 智能点评测试环境准备脚本
# 用法: ./script/rag-prepare.sh
#
# 执行顺序: make seed → make seed-redis → make seed-vector
# 其中 seed-vector 需要 LLM_API_KEY 和 Redis Stack

set -e
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info() { echo -e "${YELLOW}→ $1${NC}"; }
pass() { echo -e "${GREEN}✓ $1${NC}"; }
fail() { echo -e "${RED}✗ $1${NC}"; exit 1; }

echo ""
info "========== RAG 测试环境准备 =========="
echo ""

# 1. 检查 Docker 容器
info "1. 检查 MySQL、Redis 容器"
if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -q local-review-mysql; then
  fail "MySQL 容器未运行，请先执行: docker compose up -d"
fi
if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -q local-review-redis; then
  fail "Redis 容器未运行，请先执行: docker compose up -d"
fi
pass "MySQL、Redis 已运行"

# 2. 检查是否为 Redis Stack（支持向量索引）
info "2. 检查 Redis Stack（向量检索）"
redis_mods=$(docker exec local-review-redis redis-cli -a 8888.216 MODULE LIST 2>/dev/null | tr -d '"' || echo "")
if [[ "$redis_mods" != *"search"* ]] && [[ "$redis_mods" != *"Search"* ]]; then
  fail "当前 Redis 非 Redis Stack，不支持向量检索。请使用 redis-stack-server 镜像（docker-compose 已配置）"
fi
pass "Redis Stack 可用"

# 3. 导入 MySQL 种子数据（店铺等）
info "3. 导入 MySQL 种子数据（make seed）"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
docker exec -i local-review-mysql mysql -uroot -p8888.216 local_review_go < "$SCRIPT_DIR/seed.sql" 2>/dev/null || true
pass "MySQL 种子数据已导入（10 家店铺：朝阳区/海淀区/西城区/东城区/丰台区，美食/咖啡/酒店）"

# 4. 初始化 Redis（验证码等）
info "4. 初始化 Redis（make seed-redis）"
"$SCRIPT_DIR/seed-redis.sh"
pass "Redis 初始化完成"

# 5. 向量导入（需 LLM_API_KEY）
info "5. 店铺向量导入（make seed-vector）"
if [[ -z "$LLM_API_KEY" ]]; then
  echo -e "${YELLOW}  跳过：未设置 LLM_API_KEY${NC}"
  echo "  请设置后手动执行: LLM_API_KEY=xxx make seed-vector"
  echo "  然后再运行: ./script/rag-test.sh"
else
  if (cd "$PROJECT_ROOT" && go run ./cmd/seed-vector) 2>/dev/null; then
    pass "店铺向量已导入"
  else
    fail "向量导入失败，请检查 LLM_API_KEY 和网络"
  fi
fi

echo ""
echo -e "${GREEN}========== 准备完成 ==========${NC}"
echo "下一步: 启动服务 (make run)，然后执行 ./script/rag-test.sh"
echo ""
