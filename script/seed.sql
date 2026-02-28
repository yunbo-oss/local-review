-- local-review-go 压测用种子数据
-- 用法: make seed 或 docker exec -i local-review-mysql mysql -uroot -p8888.216 local_review_go < script/seed.sql

SET NAMES utf8mb4;

-- 1. 店铺类型
INSERT IGNORE INTO tb_shop_type (id, name, icon, sort, create_time, update_time) VALUES
(1, '美食', 'catering', 1, NOW(), NOW()),
(2, '咖啡', 'coffee', 2, NOW(), NOW()),
(3, '酒店', 'hotel', 3, NOW(), NOW());

-- 2. 店铺（10 家）
INSERT IGNORE INTO tb_shop (id, name, type_id, images, area, address, x, y, avg_price, sold, comments, score, open_hours, create_time, update_time) VALUES
(1, '老北京炸酱面', 1, 'https://picsum.photos/200', '朝阳区', '朝阳路100号', 116.4, 39.9, 35, 1200, 320, 48, '09:00-22:00', NOW(), NOW()),
(2, '川味小厨', 1, 'https://picsum.photos/201', '海淀区', '中关村大街88号', 116.3, 39.98, 45, 800, 180, 46, '10:00-21:30', NOW(), NOW()),
(3, '星巴克臻选', 2, 'https://picsum.photos/202', '西城区', '西单大悦城', 116.38, 39.91, 55, 2000, 500, 50, '08:00-23:00', NOW(), NOW()),
(4, '瑞幸咖啡', 2, 'https://picsum.photos/203', '东城区', '王府井大街1号', 116.41, 39.92, 25, 3500, 800, 47, '07:30-22:00', NOW(), NOW()),
(5, '海底捞火锅', 1, 'https://picsum.photos/204', '朝阳区', '望京SOHO', 116.48, 39.99, 120, 5000, 1200, 49, '10:00-24:00', NOW(), NOW()),
(6, '如家酒店', 3, 'https://picsum.photos/205', '海淀区', '五道口', 116.34, 39.99, 200, 300, 150, 45, '24小时', NOW(), NOW()),
(7, '外婆家', 1, 'https://picsum.photos/206', '朝阳区', '三里屯', 116.45, 39.93, 80, 1800, 400, 48, '11:00-22:00', NOW(), NOW()),
(8, 'Manner Coffee', 2, 'https://picsum.photos/207', '海淀区', '清华东路', 116.35, 40.0, 30, 1500, 350, 46, '08:00-20:00', NOW(), NOW()),
(9, '全聚德烤鸭', 1, 'https://picsum.photos/208', '东城区', '前门大街', 116.39, 39.9, 150, 2500, 600, 49, '11:00-21:00', NOW(), NOW()),
(10, '汉庭酒店', 3, 'https://picsum.photos/209', '丰台区', '北京南站', 116.38, 39.87, 180, 200, 80, 44, '24小时', NOW(), NOW());

-- 3. 普通优惠券（店铺 1-5）
INSERT IGNORE INTO tb_voucher (id, shop_id, title, sub_title, rules, pay_value, actual_value, type, status, create_time, update_time) VALUES
(1, 1, '满30减10', '新客专享', '满30可用', 1000, 3000, 0, 0, NOW(), NOW()),
(2, 2, '满50减15', '限时优惠', '满50可用', 1500, 5000, 0, 0, NOW(), NOW()),
(3, 3, '第二杯半价', '咖啡日', '限同款', 0, 0, 0, 0, NOW(), NOW()),
(4, 4, '9.9元拿铁', '新人专享', '每人限1次', 990, 2500, 0, 0, NOW(), NOW()),
(5, 5, '满200减50', '火锅季', '满200可用', 5000, 20000, 0, 0, NOW(), NOW());

-- 4. 秒杀优惠券（voucher_id 6,7,8 需先插入主表，再插入秒杀表）
INSERT IGNORE INTO tb_voucher (id, shop_id, title, sub_title, rules, pay_value, actual_value, type, status, create_time, update_time) VALUES
(6, 1, '秒杀-炸酱面5元', '限时秒杀', '每人限1份', 500, 3500, 1, 0, NOW(), NOW()),
(7, 4, '秒杀-9.9拿铁', '爆款秒杀', '每人限1杯', 990, 2500, 1, 0, NOW(), NOW()),
(8, 5, '秒杀-火锅套餐99', '超值秒杀', '每人限1份', 9900, 20000, 1, 0, NOW(), NOW());

-- 5. 秒杀券详情（begin_time 已开始，end_time 未来24小时，便于压测）
INSERT IGNORE INTO tb_seckill_voucher (voucher_id, stock, create_time, begin_time, end_time, update_time) VALUES
(6, 500, NOW(), DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 24 HOUR), NOW()),
(7, 300, NOW(), DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 24 HOUR), NOW()),
(8, 200, NOW(), DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 24 HOUR), NOW());

-- 6. 测试用户（用于压测登录，手机号 13800138000）
INSERT IGNORE INTO tb_user (id, phone, password, nick_name, icon, create_time, update_time) VALUES
(1, '13800138000', '', 'test_user', '', NOW(), NOW());
