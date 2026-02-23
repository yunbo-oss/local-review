-- 1. get the argv
local voucherId = ARGV[1]
local userId = ARGV[2]
local orderId = ARGV[3]

-- 2. get the stock
local stockKey = "seckill:stock:" .. voucherId
local orderKey = "seckill:order:" .. voucherId

-- 判断秒杀库存是否足够
if tonumber(redis.call("get", stockKey)) <= 0 then
	-- the stock is not enough
	return 1
end

if redis.call("sismember", orderKey, userId) == 1 then
	return 2
end

-- 3. update the data（预减库存 + 防重复购买标记）
-- 消息发送由应用层通过 RocketMQ 完成
redis.call("incrby", stockKey, -1)
redis.call("sadd", orderKey, userId)
return 0