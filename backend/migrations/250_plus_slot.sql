-- [plus] 积分老虎机配置与摇机记录

-- 老虎机配置（单行乐观锁）
CREATE TABLE IF NOT EXISTS plus_slot_config (
    id         SMALLINT    PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    version    BIGINT      NOT NULL DEFAULT 1,
    data       JSONB       NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 摇机记录
-- cost: 消耗积分；tier_index: 命中奖档下标，-1=谢谢惠顾
-- prize_type: 'none'|'points'|'balance'|'virtual_item'
-- ledger_id: 消耗积分流水；win_ledger_id: 积分中奖流水（prize_type=points时）
CREATE TABLE IF NOT EXISTS plus_slot_spins (
    id              BIGSERIAL    PRIMARY KEY,
    user_id         BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    idempotency_key VARCHAR(64)  NOT NULL,
    cost            BIGINT       NOT NULL,
    tier_index      INT          NOT NULL,
    prize_type      VARCHAR(32)  NOT NULL,
    prize_points    BIGINT,
    prize_balance   DECIMAL(20,8),
    prize_item_id   INT,
    ledger_id       BIGINT       REFERENCES plus_points_ledger(id),
    win_ledger_id   BIGINT       REFERENCES plus_points_ledger(id),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_plus_slot_spins_user_created ON plus_slot_spins(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_plus_slot_spins_created ON plus_slot_spins(created_at DESC);
