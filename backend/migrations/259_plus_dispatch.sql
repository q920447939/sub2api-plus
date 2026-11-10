-- [plus] 上游调度台：检测（维度 / 检测项 / 检测计划 / 执行记录）+ 判定 + 待处理 + 调度调整 + 操作记录。
-- 所有表以 plus_dispatch_ 为前缀，只读上游表（accounts / groups / account_groups / usage_logs / ops_error_logs），不写上游表。

-- 配置单行 + 乐观锁（mode: off | observe | manual）
CREATE TABLE IF NOT EXISTS plus_dispatch_config (
    id         SMALLINT    PRIMARY KEY CHECK (id = 1),
    version    BIGINT      NOT NULL DEFAULT 0,
    data       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO plus_dispatch_config (id, version, data) VALUES (1, 0, '{}'::jsonb) ON CONFLICT (id) DO NOTHING;

-- 维度。role: gate（门槛）| score（参与调度）| display（仅展示）
CREATE TABLE IF NOT EXISTS plus_dispatch_dimension (
    id         BIGSERIAL   PRIMARY KEY,
    name       TEXT        NOT NULL,
    role       TEXT        NOT NULL DEFAULT 'display',
    gate       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    sort       INT         NOT NULL DEFAULT 0,
    enabled    BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 检测项：某种检测器类型的一个实例。params / rule 的结构由 type 决定。
-- platforms 只给被动类型（流量统计、账号属性）用：在哪些平台上生效；主动类型由检测计划决定范围。
CREATE TABLE IF NOT EXISTS plus_dispatch_item (
    id           BIGSERIAL   PRIMARY KEY,
    name         TEXT        NOT NULL,
    type         TEXT        NOT NULL,
    dimension_id BIGINT      NOT NULL REFERENCES plus_dispatch_dimension (id),
    display_only BOOLEAN     NOT NULL DEFAULT FALSE,
    params       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    rule         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    platforms    TEXT[]      NOT NULL DEFAULT '{}',
    enabled      BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 检测计划：检测项 × 一个平台 × 若干模型 × 账号范围，每 N 分钟执行一次。
-- scopes：账号范围规则（可多选，命中任意一条就测），如 [{"type":"traffic","params":{"min_share":5}},{"type":"new","params":{"days":3}},{"type":"manual","account_ids":[1]}]
-- exclude_ids：不测的账号（优先于 scopes）
CREATE TABLE IF NOT EXISTS plus_dispatch_plan (
    id               BIGSERIAL   PRIMARY KEY,
    item_id          BIGINT      NOT NULL REFERENCES plus_dispatch_item (id) ON DELETE CASCADE,
    platform         TEXT        NOT NULL,
    models           TEXT[]      NOT NULL DEFAULT '{}',
    scopes           JSONB       NOT NULL DEFAULT '[]'::jsonb,
    exclude_ids      BIGINT[]    NOT NULL DEFAULT '{}',
    interval_minutes INT         NOT NULL DEFAULT 60,
    timeout_seconds  INT         NOT NULL DEFAULT 180,
    enabled          BOOLEAN     NOT NULL DEFAULT TRUE,
    next_run_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plus_dispatch_plan_due ON plus_dispatch_plan (next_run_at) WHERE enabled;

-- 主动检测的每一次执行（账号直连）。排队也落库，多实例共享。
-- cred_key：同一个凭证（地址 + Key）只测一次，挂在多个分组 / 多条账号记录上时结果共用。
-- outcome：由检测器类型声明（pass / fail / low / error / saved / skipped …）
CREATE TABLE IF NOT EXISTS plus_dispatch_run (
    id            BIGSERIAL   PRIMARY KEY,
    plan_id       BIGINT      REFERENCES plus_dispatch_plan (id) ON DELETE SET NULL,
    item_id       BIGINT      NOT NULL REFERENCES plus_dispatch_item (id) ON DELETE CASCADE,
    account_id    BIGINT      NOT NULL,
    cred_key      TEXT        NOT NULL,
    platform      TEXT        NOT NULL,
    model         TEXT        NOT NULL,
    trigger       TEXT        NOT NULL DEFAULT 'cron',
    triggered_by  BIGINT,
    status        TEXT        NOT NULL DEFAULT 'queued',
    outcome       TEXT        NOT NULL DEFAULT '',
    summary       TEXT        NOT NULL DEFAULT '',
    detail        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    prompt        TEXT        NOT NULL DEFAULT '',
    reply         TEXT        NOT NULL DEFAULT '',
    error         TEXT        NOT NULL DEFAULT '',
    duration_ms   INT,
    input_tokens  INT,
    output_tokens INT,
    queued_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at    TIMESTAMPTZ,
    finished_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_plus_dispatch_run_lookup ON plus_dispatch_run (item_id, cred_key, model, id DESC);
CREATE INDEX IF NOT EXISTS idx_plus_dispatch_run_account ON plus_dispatch_run (account_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_plus_dispatch_run_plan ON plus_dispatch_run (plan_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_plus_dispatch_run_queue ON plus_dispatch_run (status, id) WHERE status IN ('queued', 'running');

-- 真实流量的小时汇总（分组 × 账号 × 模型），由汇总任务定时重算最近两个小时
CREATE TABLE IF NOT EXISTS plus_dispatch_traffic_hourly (
    bucket      TIMESTAMPTZ NOT NULL,
    group_id    BIGINT      NOT NULL,
    account_id  BIGINT      NOT NULL,
    model       TEXT        NOT NULL,
    requests    INT         NOT NULL DEFAULT 0,
    errors      INT         NOT NULL DEFAULT 0,
    limited     INT         NOT NULL DEFAULT 0,
    ttft_p90    REAL,
    ttft_n      INT         NOT NULL DEFAULT 0,
    out_tokens  BIGINT      NOT NULL DEFAULT 0,
    gen_ms      BIGINT      NOT NULL DEFAULT 0,
    cache_read  BIGINT      NOT NULL DEFAULT 0,
    cache_total BIGINT      NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket, group_id, account_id, model)
);
CREATE INDEX IF NOT EXISTS idx_plus_dispatch_traffic_group ON plus_dispatch_traffic_hourly (group_id, bucket DESC);

-- 每个（分组 × 账号 × 模型 × 检测项）当前的判定，由判定任务定时刷新
CREATE TABLE IF NOT EXISTS plus_dispatch_verdict (
    group_id   BIGINT      NOT NULL,
    account_id BIGINT      NOT NULL,
    model      TEXT        NOT NULL,
    item_id    BIGINT      NOT NULL REFERENCES plus_dispatch_item (id) ON DELETE CASCADE,
    level      TEXT        NOT NULL,
    short      TEXT        NOT NULL DEFAULT '',
    long       TEXT        NOT NULL DEFAULT '',
    value      DOUBLE PRECISION,
    ref_value  DOUBLE PRECISION,
    evidence   JSONB       NOT NULL DEFAULT '{}'::jsonb,
    since      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_id, account_id, model, item_id)
);

-- 判定变化历史（单个账号页的「状态变化」时间线）
CREATE TABLE IF NOT EXISTS plus_dispatch_verdict_change (
    id         BIGSERIAL   PRIMARY KEY,
    group_id   BIGINT      NOT NULL,
    account_id BIGINT      NOT NULL,
    model      TEXT        NOT NULL,
    item_id    BIGINT      NOT NULL,
    from_level TEXT        NOT NULL,
    to_level   TEXT        NOT NULL,
    text       TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plus_dispatch_vchange ON plus_dispatch_verdict_change (account_id, created_at DESC);

-- 待处理：维度变成可疑 / 异常（kind=dim）、检测本身连续出错（kind=fault）、有流量的模型没被计划覆盖（kind=gap）
-- state: todo | confirmed | false（误报）| ignored | resolved（自动恢复）
CREATE TABLE IF NOT EXISTS plus_dispatch_event (
    id           BIGSERIAL   PRIMARY KEY,
    kind         TEXT        NOT NULL,
    dedup_key    TEXT        NOT NULL,
    group_id     BIGINT      NOT NULL DEFAULT 0,
    account_id   BIGINT      NOT NULL DEFAULT 0,
    model        TEXT        NOT NULL DEFAULT '',
    dimension_id BIGINT      NOT NULL DEFAULT 0,
    item_id      BIGINT      NOT NULL DEFAULT 0,
    level        TEXT        NOT NULL,
    state        TEXT        NOT NULL DEFAULT 'todo',
    title        TEXT        NOT NULL DEFAULT '',
    evidence     JSONB       NOT NULL DEFAULT '[]'::jsonb,
    first_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at  TIMESTAMPTZ,
    handled_by   BIGINT,
    handled_at   TIMESTAMPTZ
);
-- 同一个对象同时只有一条未结束的记录
CREATE UNIQUE INDEX IF NOT EXISTS uq_plus_dispatch_event_open ON plus_dispatch_event (dedup_key) WHERE state <> 'resolved';
CREATE INDEX IF NOT EXISTS idx_plus_dispatch_event_state ON plus_dispatch_event (state, last_at DESC);

-- 调度调整：分组 × 账号的当前状态（没有记录 = 正常）。action: pause | factor（0 < factor < 1）
CREATE TABLE IF NOT EXISTS plus_dispatch_adjustment (
    group_id   BIGINT      NOT NULL,
    account_id BIGINT      NOT NULL,
    action     TEXT        NOT NULL,
    factor     DOUBLE PRECISION NOT NULL DEFAULT 1,
    source     TEXT        NOT NULL DEFAULT 'manual',
    reason     TEXT        NOT NULL DEFAULT '',
    log_id     BIGINT,
    created_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_id, account_id)
);

-- 调整历史：每一次变化（包括恢复）一条，效果对比挂在这上面
CREATE TABLE IF NOT EXISTS plus_dispatch_adjustment_log (
    id          BIGSERIAL   PRIMARY KEY,
    group_id    BIGINT      NOT NULL,
    account_id  BIGINT      NOT NULL,
    from_action TEXT        NOT NULL,
    from_factor DOUBLE PRECISION NOT NULL DEFAULT 1,
    to_action   TEXT        NOT NULL,
    to_factor   DOUBLE PRECISION NOT NULL DEFAULT 1,
    source      TEXT        NOT NULL DEFAULT 'manual',
    reason      TEXT        NOT NULL DEFAULT '',
    snapshot    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    operator_id BIGINT,
    operator    TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plus_dispatch_adjlog ON plus_dispatch_adjustment_log (group_id, created_at DESC);

-- 钩子介入次数（每小时一行），用来证明调整确实在起作用
CREATE TABLE IF NOT EXISTS plus_dispatch_hook_hits (
    bucket     TIMESTAMPTZ NOT NULL,
    group_id   BIGINT      NOT NULL,
    account_id BIGINT      NOT NULL,
    action     TEXT        NOT NULL,
    hits       BIGINT      NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket, group_id, account_id, action)
);

-- 操作记录
CREATE TABLE IF NOT EXISTS plus_dispatch_audit (
    id          BIGSERIAL   PRIMARY KEY,
    operator_id BIGINT,
    operator    TEXT        NOT NULL DEFAULT '',
    action      TEXT        NOT NULL,
    target_type TEXT        NOT NULL,
    target_id   TEXT        NOT NULL DEFAULT '',
    summary     TEXT        NOT NULL DEFAULT '',
    before      JSONB,
    after       JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plus_dispatch_audit_time ON plus_dispatch_audit (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_plus_dispatch_audit_target ON plus_dispatch_audit (target_type, target_id, created_at DESC);

-- 默认维度与检测项（只在空表时插入）
DO $$
DECLARE
    d_identity BIGINT; d_iq BIGINT; d_stab BIGINT; d_speed BIGINT; d_cap BIGINT; d_cost BIGINT;
BEGIN
    IF EXISTS (SELECT 1 FROM plus_dispatch_dimension) THEN
        RETURN;
    END IF;
    INSERT INTO plus_dispatch_dimension (name, role, gate, sort) VALUES
        ('模型身份', 'gate', '{"before":"cut","before_pct":3,"after":"stop","after_pct":0}', 10) RETURNING id INTO d_identity;
    INSERT INTO plus_dispatch_dimension (name, role, gate, sort) VALUES
        ('智力', 'gate', '{"before":"cut","before_pct":3,"after":"cut","after_pct":3}', 20) RETURNING id INTO d_iq;
    INSERT INTO plus_dispatch_dimension (name, role, sort) VALUES ('稳定', 'score', 30) RETURNING id INTO d_stab;
    INSERT INTO plus_dispatch_dimension (name, role, sort) VALUES ('速度', 'score', 40) RETURNING id INTO d_speed;
    INSERT INTO plus_dispatch_dimension (name, role, sort) VALUES ('容量', 'score', 50) RETURNING id INTO d_cap;
    INSERT INTO plus_dispatch_dimension (name, role, sort) VALUES ('成本', 'display', 60) RETURNING id INTO d_cost;

    INSERT INTO plus_dispatch_item (name, type, dimension_id, params, rule) VALUES
        ('数字指纹', 'fingerprint', d_identity, '{}', '{"bad_consecutive":3,"min_confidence":60}'),
        ('糖果题', 'graded', d_iq,
         jsonb_build_object('prompt', E'在一个黑色的袋子里放有三种口味的糖果，每种糖果有两种不同的形状（圆形和五角星形，不同的形状靠手感可以分辨）。现已知不同口味的糖和不同形状的数量统计如下表。参赛者需要在活动前决定摸出的糖果数目，那么，最少取出多少个糖果才能保证手中同时拥有不同形状的苹果味和桃子味的糖？（同时手中有圆形苹果味匹配五角星桃子味糖果，或者有圆形桃子味匹配五角星苹果味糖果都满足要求）\n          苹果味  桃子味  西瓜味\n圆形        7       9       8\n五角星形    7       6       4\n\n请给出最终的数值答案。',
                            'answers', jsonb_build_array('29'), 'match', 'num', 'max_tokens', 8000),
         '{"window":10,"how":"fixed","warn_below":6,"bad_below":3,"peer_warn":3,"peer_bad":5}');

    INSERT INTO plus_dispatch_item (name, type, dimension_id, params, rule, platforms) VALUES
        ('报错率', 'traffic', d_stab, '{"metric":"err"}', '{"bases":["peer","site","self"],"min_peers":2,"min_samples":50,"mode":"diff","warn":2,"bad":5,"floor":1,"duration_minutes":60}', '{anthropic,openai,gemini,antigravity}'),
        ('限流率', 'traffic', d_cap, '{"metric":"r429"}', '{"bases":["peer","site","self"],"min_peers":2,"min_samples":50,"mode":"diff","warn":5,"bad":12,"floor":2,"duration_minutes":60}', '{anthropic,openai,gemini,antigravity}'),
        ('首字耗时', 'traffic', d_speed, '{"metric":"ttft"}', '{"bases":["peer","site","self"],"min_peers":2,"min_samples":50,"mode":"ratio","warn":1.5,"bad":2,"floor":3,"duration_minutes":60}', '{anthropic,openai,gemini,antigravity}'),
        ('输出速度', 'traffic', d_speed, '{"metric":"tps"}', '{"bases":["peer","site","self"],"min_peers":2,"min_samples":50,"mode":"pct","warn":70,"bad":50,"duration_minutes":60}', '{anthropic,openai,gemini,antigravity}'),
        ('缓存命中率', 'traffic', d_speed, '{"metric":"cache"}', '{"bases":["peer","site","self"],"min_peers":2,"min_samples":50,"mode":"diff","warn":15,"bad":30,"duration_minutes":60}', '{anthropic,openai}'),
        ('成本倍率', 'field', d_cost, '{"field":"rate_multiplier"}', '{}', '{anthropic,openai,gemini,antigravity}');
END $$;
