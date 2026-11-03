-- [plus] 签到日志与配置

-- 签到日志（每用户每日唯一，checked_date 为东8区自然日）
CREATE TABLE IF NOT EXISTS plus_checkin_logs (
    id             BIGSERIAL   PRIMARY KEY,
    user_id        BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    checked_date   DATE        NOT NULL,
    streak         INT         NOT NULL,           -- 签到时的连签天数 [1..7]，循环
    points_granted BIGINT      NOT NULL,
    ledger_id      BIGINT      NOT NULL REFERENCES plus_points_ledger(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, checked_date)
);
CREATE INDEX IF NOT EXISTS idx_plus_checkin_logs_user_date ON plus_checkin_logs(user_id, checked_date DESC);

-- IP 签到日志（用于同 IP 多账号限流）
CREATE TABLE IF NOT EXISTS plus_checkin_ip_logs (
    id         BIGSERIAL   PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ip         VARCHAR(45) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plus_checkin_ip_logs_ip_created ON plus_checkin_ip_logs(ip, created_at DESC);

-- 签到配置（单行乐观锁）
CREATE TABLE IF NOT EXISTS plus_checkin_config (
    id         SMALLINT    PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    version    BIGINT      NOT NULL DEFAULT 1,
    data       JSONB       NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
