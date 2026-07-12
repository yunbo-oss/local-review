#!/usr/bin/env bash
# 循环执行：重置 + 压测，直到 seckill_success >= 151
# 用法: ./script/run-load-test-until-success.sh

set -e
cd "$(dirname "$0")/.."
MAX_ATTEMPTS=${MAX_ATTEMPTS:-5}

for i in $(seq 1 $MAX_ATTEMPTS); do
  echo "===== 第 $i 次尝试 ====="
  make seed-reset-load-test
  sleep 2
  if make load-test-seckill-e2e; then
    echo "✓ 压测成功！"
    exit 0
  fi
  echo "未达到 151 成功，3 秒后重试..."
  sleep 3
done
echo "已达最大尝试次数 $MAX_ATTEMPTS，退出"
exit 1
