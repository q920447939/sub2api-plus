-- [plus] 渠道状态：用户侧业务限制错误（余额不足、Key 无效等）的小时汇总。
-- 上游 V2 预聚合表没有 is_business_limited，直接扫 ops_error_logs 做 30/84 天历史太慢，
-- 所以由 plus 后台任务按小时汇总到这里，渠道状态计算时从请求数与错误数中扣除。
CREATE TABLE IF NOT EXISTS plus_channel_status_excluded_1h (
    bucket_start   TIMESTAMPTZ NOT NULL,
    group_id       BIGINT      NOT NULL,
    error_requests BIGINT      NOT NULL DEFAULT 0,
    computed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (bucket_start, group_id)
);

CREATE INDEX IF NOT EXISTS idx_plus_cs_excluded_group_time
    ON plus_channel_status_excluded_1h (group_id, bucket_start DESC);

-- 历史补算游标：已补算到的最早时间点（不含），单行
CREATE TABLE IF NOT EXISTS plus_channel_status_excluded_cursor (
    id              SMALLINT    PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    backfill_before TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
