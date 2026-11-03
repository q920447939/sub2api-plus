-- [plus] 老虎机 v2：移除虚拟商品，新增概率修正器表与中奖跑马灯动态表

ALTER TABLE plus_slot_spins DROP COLUMN IF EXISTS prize_item_id;

-- v1 配置含 virtual_item 档位且结构已变，直接清掉，服务端回落到默认配置（默认关闭）
DELETE FROM plus_slot_config;

-- 概率修正器实例：按 priority 升序、id 次序依次作用于奖档权重
CREATE TABLE IF NOT EXISTS plus_slot_modifiers (
    id         BIGSERIAL   PRIMARY KEY,
    type       VARCHAR(32) NOT NULL,
    enabled    BOOLEAN     NOT NULL DEFAULT TRUE,
    priority   INT         NOT NULL DEFAULT 0,
    params     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 默认一条连签加成：连签 ≥3 天，第 4-7 档（下标 3..6）权重 ×1.25
INSERT INTO plus_slot_modifiers (type, enabled, priority, params)
VALUES ('streak_boost', TRUE, 0, '{"min_streak": 3, "multiplier": 1.25, "tiers": [3, 4, 5, 6]}'::jsonb);

-- 中奖跑马灯：真实中奖与虚拟动态同构；virtual_slot 为分钟序号，唯一约束保证多实例幂等
CREATE TABLE IF NOT EXISTS plus_slot_feed (
    id           BIGSERIAL   PRIMARY KEY,
    display_name VARCHAR(64) NOT NULL,
    tier_index   INT         NOT NULL,
    label        VARCHAR(32) NOT NULL,
    emoji        VARCHAR(16) NOT NULL DEFAULT '',
    is_virtual   BOOLEAN     NOT NULL DEFAULT FALSE,
    virtual_slot BIGINT      UNIQUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plus_slot_feed_created ON plus_slot_feed(created_at DESC);

-- 充值门槛统计：按用户 + 使用时间查已用余额卡密
CREATE INDEX IF NOT EXISTS idx_redeem_codes_used_by_balance
    ON redeem_codes(used_by, used_at)
    WHERE type = 'balance' AND status = 'used';

-- 签到 streak 改为真实连续天数（不再 1..7 循环），奖励按 (streak-1)%7+1 取档
COMMENT ON COLUMN plus_checkin_logs.streak IS '签到时的真实连续签到天数；7 天奖励循环由 (streak-1)%7+1 推导';
