-- [plus] 上游调度台 v2：体检按「账号 × 模型」（流量跨分组合并），分流按「分组 × 模型」（线路）。
-- 不兼容 v1：去掉维度、判定表、待处理、分组 × 账号的调整；检测结论改为按检测项规则从检测记录现算。

DROP TABLE IF EXISTS plus_dispatch_hook_hits;
DROP TABLE IF EXISTS plus_dispatch_adjustment_log;
DROP TABLE IF EXISTS plus_dispatch_adjustment;
DROP TABLE IF EXISTS plus_dispatch_event;
DROP TABLE IF EXISTS plus_dispatch_verdict_change;
DROP TABLE IF EXISTS plus_dispatch_verdict;

-- 检测项：只留主动检测（指纹识别 / 判分题 / 存档题），流量类指标改由健康分统一给出
DELETE FROM plus_dispatch_item WHERE type NOT IN ('fingerprint', 'graded', 'archive');
ALTER TABLE plus_dispatch_item DROP COLUMN IF EXISTS dimension_id;
ALTER TABLE plus_dispatch_item DROP COLUMN IF EXISTS display_only;
ALTER TABLE plus_dispatch_item DROP COLUMN IF EXISTS platforms;
DROP TABLE IF EXISTS plus_dispatch_dimension;

-- 判定规则统一成「最近 window 次有效结果」口径
UPDATE plus_dispatch_item
SET rule = jsonb_build_object(
        'window', 10,
        'bad_min', COALESCE((rule->>'bad_consecutive')::int, 3),
        'min_confidence', COALESCE((rule->>'min_confidence')::numeric, 60),
        'min_valid', 3),
    updated_at = NOW()
WHERE type = 'fingerprint';
UPDATE plus_dispatch_item
SET rule = jsonb_build_object(
        'window', COALESCE((rule->>'window')::int, 10),
        'bad_below', COALESCE((rule->>'bad_below')::int, 3),
        'min_valid', 5),
    updated_at = NOW()
WHERE type = 'graded';

-- 默认带上鹈鹕画图（题面和「模型智商检测」一致），不建计划，管理员自己决定测谁
INSERT INTO plus_dispatch_item (name, type, params, rule)
SELECT '鹈鹕画图', 'archive',
       jsonb_build_object('prompt', '请生成可直接运行的单文件HTML，使用内联SVG绘制鹈鹕骑自行车的二维循环动画。画面以鹈鹕和自行车为主体，展示清晰的身体结构、踩踏动作和车轮转动，配合协调的背景、配色与层次。动画应流畅自然、衔接连续，并适配不同屏幕尺寸。禁止依赖外部资源，只输出完整HTML，不要代码围栏或解释文字。'),
       '{}'::jsonb
WHERE NOT EXISTS (SELECT 1 FROM plus_dispatch_item WHERE type = 'archive');

-- 检测计划：检测项 × 一个平台 × 若干模型 × 指定账号（空 = 调度开着、自己提供这个模型的全部账号）
ALTER TABLE plus_dispatch_plan DROP COLUMN IF EXISTS scopes;
ALTER TABLE plus_dispatch_plan DROP COLUMN IF EXISTS exclude_ids;
ALTER TABLE plus_dispatch_plan ADD COLUMN IF NOT EXISTS account_ids BIGINT[] NOT NULL DEFAULT '{}';

-- 流量：首字耗时中位数（健康分和渠道状态同口径）；输出速度改为只统计流式且输出不少于 50 token 的请求
ALTER TABLE plus_dispatch_traffic_hourly ADD COLUMN IF NOT EXISTS ttft_p50 REAL;
ALTER TABLE plus_dispatch_traffic_hourly ADD COLUMN IF NOT EXISTS p50_n INT NOT NULL DEFAULT 0;
TRUNCATE plus_dispatch_traffic_hourly; -- 口径变了，启动时按新口径回填

-- 配置回到默认
UPDATE plus_dispatch_config SET data = '{}'::jsonb, version = version + 1, updated_at = NOW() WHERE id = 1;

-- 权重设置：账号在某个分组（某个模型）上承接多少。没有记录 = 正常
--   model = ''：这个分组里它的全部模型；同一个分组里模型上的设置优先
--   share：0 = 暂停；(0,1] = 在这条线路上给它的比例；pct：保存时它在这个分组预计承接的百分比（展示用）
--   hard：暂停时连「只有它能跑」的部分也停掉（请求会失败）
CREATE TABLE IF NOT EXISTS plus_dispatch_rule (
    account_id BIGINT      NOT NULL,
    group_id   BIGINT      NOT NULL,
    model      TEXT        NOT NULL DEFAULT '',
    share      DOUBLE PRECISION NOT NULL,
    hard       BOOLEAN     NOT NULL DEFAULT FALSE,
    pct        DOUBLE PRECISION,
    record_id  BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (account_id, group_id, model)
);
CREATE INDEX IF NOT EXISTS idx_plus_dispatch_rule_group ON plus_dispatch_rule (group_id);

-- 调整记录：一次保存一条（一个账号或它的一个模型，涉及若干分组），效果对比挂在这上面
--   groups：[{group, from, to}]，from / to = {share, hard, pct} 或 null（正常）
--   side：同组里被连带改动的其他账号 [{account, group, model, from, to}]
--   predicted：保存时的预计 [{group, now, next}]
CREATE TABLE IF NOT EXISTS plus_dispatch_record (
    id          BIGSERIAL   PRIMARY KEY,
    account_id  BIGINT      NOT NULL,
    model       TEXT        NOT NULL DEFAULT '',
    kind        TEXT        NOT NULL DEFAULT 'adjust', -- adjust | restore | undo
    note        TEXT        NOT NULL DEFAULT '',
    groups      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    side        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    predicted   JSONB       NOT NULL DEFAULT '[]'::jsonb,
    undone_at   TIMESTAMPTZ,
    operator_id BIGINT,
    operator    TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plus_dispatch_record_account ON plus_dispatch_record (account_id, id DESC);

-- 标为误报：这个凭证 × 模型 × 检测项，到 up_to_run 为止的未通过不算数（之后的新结果照常判）
CREATE TABLE IF NOT EXISTS plus_dispatch_ignore (
    cred_key    TEXT        NOT NULL,
    model       TEXT        NOT NULL,
    item_id     BIGINT      NOT NULL REFERENCES plus_dispatch_item (id) ON DELETE CASCADE,
    up_to_run   BIGINT      NOT NULL,
    operator_id BIGINT,
    operator    TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (cred_key, model, item_id)
);
