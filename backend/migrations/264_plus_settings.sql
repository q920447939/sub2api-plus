-- 264_plus_settings.sql
-- [plus] 按模块隔离的运行时设置：每个模块一行（key = 模块名），JSON + 乐观锁。
-- 模块之间不共享配置结构，见 internal/plus/plusconfig。
CREATE TABLE IF NOT EXISTS plus_settings (
    key        VARCHAR(40) PRIMARY KEY,
    version    BIGINT      NOT NULL DEFAULT 1,
    data       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
