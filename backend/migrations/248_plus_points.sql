-- [plus] 积分账户与流水
-- 积分用 BIGINT 整数（最小单位 1 分），避免浮点精度问题

-- 积分账户（每用户一行，首次发放时 UPSERT 自动创建）
CREATE TABLE IF NOT EXISTS plus_points_accounts (
    user_id        BIGINT      PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    balance        BIGINT      NOT NULL DEFAULT 0,
    last_active_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 积分流水
-- delta 正=收入，负=支出；balance 为变动后快照
-- reason 驱动横向扩展：新增来源/消耗只加 reason 常量，不改表结构
-- ref_id 关联业务 ID（checkin_log.id / slot_spin.id）
CREATE TABLE IF NOT EXISTS plus_points_ledger (
    id         BIGSERIAL    PRIMARY KEY,
    user_id    BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    delta      BIGINT       NOT NULL,
    balance    BIGINT       NOT NULL,
    reason     VARCHAR(32)  NOT NULL,
    ref_id     BIGINT,
    note       VARCHAR(255),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plus_points_ledger_user_created ON plus_points_ledger(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_plus_points_ledger_created ON plus_points_ledger(created_at DESC);

-- 积分配置（单行乐观锁，同 lottery 模式）
CREATE TABLE IF NOT EXISTS plus_points_config (
    id         SMALLINT    PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    version    BIGINT      NOT NULL DEFAULT 1,
    data       JSONB       NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
