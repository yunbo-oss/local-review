-- 压测用：多用户 + 多秒杀券
-- 用法: make seed-load-test 或 docker exec -i local-review-mysql mysql -uroot -p8888.216 local_review_go < script/seed-load-test.sql
-- 前置: 需先执行 make seed（基础数据）

SET NAMES utf8mb4;

-- 1. 多用户（50 个，手机号 13800138001-13800138050，id 2-51）
INSERT IGNORE INTO tb_user (id, phone, password, nick_name, icon, create_time, update_time) VALUES
(2, '13800138001', '', 'user_01', '', NOW(), NOW()),
(3, '13800138002', '', 'user_02', '', NOW(), NOW()),
(4, '13800138003', '', 'user_03', '', NOW(), NOW()),
(5, '13800138004', '', 'user_04', '', NOW(), NOW()),
(6, '13800138005', '', 'user_05', '', NOW(), NOW()),
(7, '13800138006', '', 'user_06', '', NOW(), NOW()),
(8, '13800138007', '', 'user_07', '', NOW(), NOW()),
(9, '13800138008', '', 'user_08', '', NOW(), NOW()),
(10, '13800138009', '', 'user_09', '', NOW(), NOW()),
(11, '13800138010', '', 'user_10', '', NOW(), NOW()),
(12, '13800138011', '', 'user_11', '', NOW(), NOW()),
(13, '13800138012', '', 'user_12', '', NOW(), NOW()),
(14, '13800138013', '', 'user_13', '', NOW(), NOW()),
(15, '13800138014', '', 'user_14', '', NOW(), NOW()),
(16, '13800138015', '', 'user_15', '', NOW(), NOW()),
(17, '13800138016', '', 'user_16', '', NOW(), NOW()),
(18, '13800138017', '', 'user_17', '', NOW(), NOW()),
(19, '13800138018', '', 'user_18', '', NOW(), NOW()),
(20, '13800138019', '', 'user_19', '', NOW(), NOW()),
(21, '13800138020', '', 'user_20', '', NOW(), NOW()),
(22, '13800138021', '', 'user_21', '', NOW(), NOW()),
(23, '13800138022', '', 'user_22', '', NOW(), NOW()),
(24, '13800138023', '', 'user_23', '', NOW(), NOW()),
(25, '13800138024', '', 'user_24', '', NOW(), NOW()),
(26, '13800138025', '', 'user_25', '', NOW(), NOW()),
(27, '13800138026', '', 'user_26', '', NOW(), NOW()),
(28, '13800138027', '', 'user_27', '', NOW(), NOW()),
(29, '13800138028', '', 'user_28', '', NOW(), NOW()),
(30, '13800138029', '', 'user_29', '', NOW(), NOW()),
(31, '13800138030', '', 'user_30', '', NOW(), NOW()),
(32, '13800138031', '', 'user_31', '', NOW(), NOW()),
(33, '13800138032', '', 'user_32', '', NOW(), NOW()),
(34, '13800138033', '', 'user_33', '', NOW(), NOW()),
(35, '13800138034', '', 'user_34', '', NOW(), NOW()),
(36, '13800138035', '', 'user_35', '', NOW(), NOW()),
(37, '13800138036', '', 'user_36', '', NOW(), NOW()),
(38, '13800138037', '', 'user_37', '', NOW(), NOW()),
(39, '13800138038', '', 'user_38', '', NOW(), NOW()),
(40, '13800138039', '', 'user_39', '', NOW(), NOW()),
(41, '13800138040', '', 'user_40', '', NOW(), NOW()),
(42, '13800138041', '', 'user_41', '', NOW(), NOW()),
(43, '13800138042', '', 'user_42', '', NOW(), NOW()),
(44, '13800138043', '', 'user_43', '', NOW(), NOW()),
(45, '13800138044', '', 'user_44', '', NOW(), NOW()),
(46, '13800138045', '', 'user_45', '', NOW(), NOW()),
(47, '13800138046', '', 'user_46', '', NOW(), NOW()),
(48, '13800138047', '', 'user_47', '', NOW(), NOW()),
(49, '13800138048', '', 'user_48', '', NOW(), NOW()),
(50, '13800138049', '', 'user_49', '', NOW(), NOW()),
(51, '13800138050', '', 'user_50', '', NOW(), NOW());

-- 2. 多秒杀券（10 个，voucher_id 9-18，每券库存 100）
INSERT IGNORE INTO tb_voucher (id, shop_id, title, sub_title, rules, pay_value, actual_value, type, status, create_time, update_time) VALUES
(9, 1, '秒杀-炸酱面5元', '限时秒杀', '每人限1份', 500, 3500, 1, 0, NOW(), NOW()),
(10, 2, '秒杀-川味小厨', '限时秒杀', '每人限1份', 800, 5000, 1, 0, NOW(), NOW()),
(11, 3, '秒杀-星巴克', '限时秒杀', '每人限1份', 1500, 6000, 1, 0, NOW(), NOW()),
(12, 4, '秒杀-瑞幸', '限时秒杀', '每人限1份', 990, 2500, 1, 0, NOW(), NOW()),
(13, 5, '秒杀-海底捞', '限时秒杀', '每人限1份', 9900, 20000, 1, 0, NOW(), NOW()),
(14, 6, '秒杀-如家', '限时秒杀', '每人限1份', 5000, 20000, 1, 0, NOW(), NOW()),
(15, 7, '秒杀-外婆家', '限时秒杀', '每人限1份', 3000, 8000, 1, 0, NOW(), NOW()),
(16, 8, '秒杀-Manner', '限时秒杀', '每人限1份', 990, 3000, 1, 0, NOW(), NOW()),
(17, 9, '秒杀-全聚德', '限时秒杀', '每人限1份', 5000, 15000, 1, 0, NOW(), NOW()),
(18, 10, '秒杀-汉庭', '限时秒杀', '每人限1份', 3000, 18000, 1, 0, NOW(), NOW());

INSERT IGNORE INTO tb_seckill_voucher (voucher_id, stock, create_time, begin_time, end_time, update_time) VALUES
(9, 100, NOW(), DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 24 HOUR), NOW()),
(10, 100, NOW(), DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 24 HOUR), NOW()),
(11, 100, NOW(), DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 24 HOUR), NOW()),
(12, 100, NOW(), DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 24 HOUR), NOW()),
(13, 100, NOW(), DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 24 HOUR), NOW()),
(14, 100, NOW(), DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 24 HOUR), NOW()),
(15, 100, NOW(), DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 24 HOUR), NOW()),
(16, 100, NOW(), DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 24 HOUR), NOW()),
(17, 100, NOW(), DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 24 HOUR), NOW()),
(18, 100, NOW(), DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 24 HOUR), NOW());
