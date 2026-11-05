-- [plus] 模型智商检测（鹈鹕画图）：配置单行 + 检测任务（分组 × 模型）+ 每次检测记录。
-- 预览链接用 preview_secret 做 HMAC 签名（iframe 无法带 Authorization 头）。

CREATE TABLE IF NOT EXISTS plus_iq_config (
    id             SMALLINT    PRIMARY KEY CHECK (id = 1),
    version        BIGINT      NOT NULL DEFAULT 0,
    data           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    preview_secret TEXT        NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO plus_iq_config (id, version, data, preview_secret)
VALUES (1, 0, '{}'::jsonb, replace(gen_random_uuid()::text || gen_random_uuid()::text, '-', ''))
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS plus_iq_task (
    id               BIGSERIAL   PRIMARY KEY,
    group_id         BIGINT      NOT NULL,
    model            TEXT        NOT NULL,
    key_mode         TEXT        NOT NULL DEFAULT 'auto', -- auto：自动生成的分组专用 Key；manual：管理员指定
    api_key_id       BIGINT      NOT NULL,
    format           TEXT        NOT NULL DEFAULT '',     -- 空 = 按分组平台自动选择
    thinking         TEXT        NOT NULL DEFAULT 'off',
    interval_minutes INT         NOT NULL DEFAULT 30,
    retention        INT         NOT NULL DEFAULT 48,
    enabled          BOOLEAN     NOT NULL DEFAULT TRUE,
    created_by       BIGINT      NOT NULL,
    next_run_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (group_id, model)
);

CREATE INDEX IF NOT EXISTS idx_plus_iq_task_due ON plus_iq_task (next_run_at) WHERE enabled;

-- status: queued → running → success | failed。排队也落库，多实例共享同一个队列。
CREATE TABLE IF NOT EXISTS plus_iq_run (
    id                BIGSERIAL   PRIMARY KEY,
    task_id           BIGINT      NOT NULL REFERENCES plus_iq_task (id) ON DELETE CASCADE,
    group_id          BIGINT      NOT NULL,
    model             TEXT        NOT NULL,
    trigger           TEXT        NOT NULL,              -- cron | manual
    triggered_by      BIGINT,
    status            TEXT        NOT NULL DEFAULT 'queued',
    prompt_version    INT         NOT NULL DEFAULT 1,
    api_key_id        BIGINT,
    client_request_id TEXT        NOT NULL DEFAULT '',   -- 网关响应头 X-Client-Request-ID，用于反查实际账号
    queued_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at        TIMESTAMPTZ,
    finished_at       TIMESTAMPTZ,
    duration_ms       INT,
    input_tokens      INT,
    output_tokens     INT,
    html              TEXT,
    external_refs     JSONB,
    error             TEXT        NOT NULL DEFAULT '',
    raw_excerpt       TEXT        NOT NULL DEFAULT '',
    account_chain     JSONB,
    attributed_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_plus_iq_run_task ON plus_iq_run (task_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_plus_iq_run_group ON plus_iq_run (group_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_plus_iq_run_active ON plus_iq_run (status, id) WHERE status IN ('queued', 'running');
