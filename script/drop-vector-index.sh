#!/bin/bash
# 删除 RAG 向量索引（schema 变更后需执行，再 make seed-vector 重新导入）
# 用法: ./script/drop-vector-index.sh
REDIS_CMD="${REDIS_CMD:-docker exec local-review-redis redis-cli -a 8888.216}"
$REDIS_CMD FT.DROPINDEX idx:shop:vector DD 2>/dev/null || true
echo "向量索引已删除，重启服务或执行 make seed-vector 将自动重建"
