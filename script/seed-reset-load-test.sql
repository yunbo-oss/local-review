-- 压测前重置：清空订单、恢复秒杀库存（与 seed-redis 配合使用）
-- 用法: make seed-reset-load-test

SET NAMES utf8mb4;

TRUNCATE TABLE tb_voucher_order;

UPDATE tb_seckill_voucher SET stock = 500, update_time = NOW() WHERE voucher_id = 6;
UPDATE tb_seckill_voucher SET stock = 300, update_time = NOW() WHERE voucher_id = 7;
UPDATE tb_seckill_voucher SET stock = 200, update_time = NOW() WHERE voucher_id = 8;
UPDATE tb_seckill_voucher SET stock = 200, update_time = NOW() WHERE voucher_id IN (9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30);
