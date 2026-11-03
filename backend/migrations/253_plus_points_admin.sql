-- [plus] 积分乐园管理端：手动调整积分、统计查询索引、九宫格默认冷却 10s → 3s

-- 1. 管理员手动加减积分：记录操作人与内部备注（remark 只在管理端可见，用户端只看 note）
ALTER TABLE plus_points_ledger
    ADD COLUMN IF NOT EXISTS operator_id BIGINT,
    ADD COLUMN IF NOT EXISTS remark      VARCHAR(255);

-- 2. 统计查询：按日期 / 来源聚合
CREATE INDEX IF NOT EXISTS idx_plus_checkin_logs_date ON plus_checkin_logs(checked_date);
CREATE INDEX IF NOT EXISTS idx_plus_points_ledger_reason_created ON plus_points_ledger(reason, created_at);

-- 3. 冷却只用于防脚本连刷，动画本身约 3-4 秒；已保存配置里仍是旧默认 10 秒的改为 3 秒
UPDATE plus_slot_config
SET data = jsonb_set(
        data, '{rules}',
        (SELECT jsonb_agg(
                    CASE WHEN r->>'type' = 'cooldown' AND (r->'params'->>'seconds')::numeric = 10
                         THEN jsonb_set(r, '{params,seconds}', '3'::jsonb)
                         ELSE r END ORDER BY ord)
         FROM jsonb_array_elements(data->'rules') WITH ORDINALITY AS t(r, ord))),
    version = version + 1,
    updated_at = NOW()
WHERE jsonb_typeof(data->'rules') = 'array'
  AND EXISTS (SELECT 1 FROM jsonb_array_elements(data->'rules') r
              WHERE r->>'type' = 'cooldown' AND (r->'params'->>'seconds')::numeric = 10);
