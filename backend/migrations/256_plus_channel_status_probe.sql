-- [plus] 渠道状态兜底探测：每个分组一行最近一次探测状态。
-- last_started_at 兼作多实例互斥标记：抢占时用 CAS 更新，抢到的实例才发探测请求。
CREATE TABLE IF NOT EXISTS plus_channel_status_probe_state (
    group_id        BIGINT      PRIMARY KEY,
    last_started_at TIMESTAMPTZ,
    last_checked_at TIMESTAMPTZ,
    last_ok         BOOLEAN,
    latency_ms      INT,
    message         TEXT        NOT NULL DEFAULT '',
    idle_streak     INT         NOT NULL DEFAULT 0, -- 连续多少次探测时窗口内没有任何真实请求
    day             DATE,                           -- day_count 所属自然日（东八区）
    day_count       INT         NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
