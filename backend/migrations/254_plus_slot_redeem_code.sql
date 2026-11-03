-- [plus] 幸运九宫格余额奖凭证改为随机码（SLOT- + 16 位随机字符），不再用摇奖 ID 拼接。
-- 旧记录该列为空，读取时回落到历史规则 SLOT-{spin_id}（这些码已写入 redeem_codes 且为已使用状态）。
ALTER TABLE plus_slot_spins ADD COLUMN IF NOT EXISTS redeem_code VARCHAR(32);
