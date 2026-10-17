-- [plus] 刮刮乐发卡来源由「在线支付订单」改为「余额卡密兑换」，并移除退款追回。
-- 线上不开放在线支付，246 发放的订单数据没有意义，直接清空后改表结构。

-- 1. 清空旧数据（依赖顺序：feed → draws → chances → orders）。配置表与预算表保留。
DELETE FROM plus_lottery_feed WHERE draw_id IS NOT NULL;
DELETE FROM plus_lottery_draws;
DELETE FROM plus_lottery_chances;
DELETE FROM plus_lottery_orders;

-- 2. 来源表：plus_lottery_orders → plus_lottery_redeems，主键改为 redeem_codes.id
ALTER TABLE plus_lottery_orders RENAME TO plus_lottery_redeems;
ALTER TABLE plus_lottery_redeems RENAME COLUMN order_id TO redeem_code_id;
ALTER TABLE plus_lottery_redeems DROP COLUMN IF EXISTS settled_refund;
ALTER INDEX IF EXISTS idx_plus_lottery_orders_user RENAME TO idx_plus_lottery_redeems_user;
ALTER INDEX IF EXISTS idx_plus_lottery_orders_created RENAME TO idx_plus_lottery_redeems_created;
COMMENT ON TABLE plus_lottery_redeems IS '[plus] 已计入刮刮乐的余额卡密兑换。redeem_code_id 主键保证每张卡密只发放一次；amount 为卡密面值';

-- 3. 刮刮卡：order_id → redeem_code_id，order_amount → code_value；外键与唯一约束随列名自动迁移
ALTER TABLE plus_lottery_chances RENAME COLUMN order_id TO redeem_code_id;
ALTER TABLE plus_lottery_chances RENAME COLUMN order_amount TO code_value;

-- 4. 开奖记录：去掉退款追回字段
ALTER TABLE plus_lottery_draws RENAME COLUMN order_id TO redeem_code_id;
ALTER TABLE plus_lottery_draws DROP COLUMN IF EXISTS clawback_amount;
ALTER TABLE plus_lottery_draws DROP COLUMN IF EXISTS clawed_at;
