-- 订单超时回滚：恢复 Redis 库存 + 移除用户购买标记
-- ARGV[1]: voucherId
-- ARGV[2]: userId
local voucherId = ARGV[1]
local userId = ARGV[2]

local stockKey = "seckill:stock:" .. voucherId
local orderKey = "seckill:order:" .. voucherId

redis.call("incrby", stockKey, 1)
redis.call("srem", orderKey, userId)
return 0
