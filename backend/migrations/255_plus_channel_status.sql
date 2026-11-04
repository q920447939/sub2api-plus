-- [plus] 渠道状态配置（单行乐观锁，沿用 plus_checkin_config 的做法）
CREATE TABLE IF NOT EXISTS plus_channel_status_config (
    id         SMALLINT    PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    version    BIGINT      NOT NULL DEFAULT 1,
    data       JSONB       NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
