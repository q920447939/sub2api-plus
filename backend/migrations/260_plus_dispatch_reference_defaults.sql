-- [plus] 上游调度台：流量类检测项的参照默认值调整。
-- 号池通常每组只有 2~3 个上游，旧默认「至少 2 家可比」导致大多数账号永远没有参照、一直显示数据不足。
-- 只改仍是旧默认值的检测项（min_peers = 2），管理员改过的不动。
UPDATE plus_dispatch_item
SET rule = jsonb_set(rule, '{min_peers}', '1'::jsonb), updated_at = NOW()
WHERE type = 'traffic' AND rule->>'min_peers' = '2';

-- 报错率 / 限流率：同组、全站、自己的历史都比不了时，按绝对值判（固定值 0）
UPDATE plus_dispatch_item
SET rule = jsonb_set(jsonb_set(rule, '{bases}', (rule->'bases') || '["fixed"]'::jsonb), '{fixed}', '0'::jsonb), updated_at = NOW()
WHERE type = 'traffic' AND params->>'metric' IN ('err', 'r429')
  AND jsonb_typeof(rule->'bases') = 'array' AND NOT (rule->'bases') ? 'fixed';
