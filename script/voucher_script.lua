-- 1. get the argv
local voucherId = ARGV[1]
local userId = ARGV[2]
local orderId = ARGV[3]

-- 2. get the stock
local stockKey = "seckill:stock:" .. voucherId
local orderKey = "seckill:order:" .. voucherId

-- 判断秒杀库存是否足够（key 不存在或过期时 stock 为 nil，需拒绝避免 Redis 负库存）
local stock = redis.call("get", stockKey)
if stock == false or stock == nil then
	return 1  -- key 不存在/过期，拒绝请求，需从 MySQL 回填 Redis
end
if tonumber(stock) == nil or tonumber(stock) <= 0 then
	return 1  -- 库存不足
end

if redis.call("sismember", orderKey, userId) == 1 then
	return 2
end

-- 3. update the data（预减库存 + 防重复购买标记）
-- 消息发送由应用层通过 RocketMQ 完成
redis.call("incrby", stockKey, -1)
redis.call("sadd", orderKey, userId)
return 0