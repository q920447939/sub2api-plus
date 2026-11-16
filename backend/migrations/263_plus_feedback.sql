-- [plus] 用户反馈中心：反馈、时间线、附件、配置

-- 反馈主表
-- kind/scenario 保留用户原选（*_user）与管理员修正后的值；统计和积分按修正后的值
-- status: pending 待处理 / awaiting 待补充 / resolved 已解决 / adopted 已采纳 / declined 不处理 / duplicate 重复
CREATE TABLE IF NOT EXISTS plus_feedback (
    id               BIGSERIAL    PRIMARY KEY,
    user_id          BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind_user        VARCHAR(16)  NOT NULL,
    kind             VARCHAR(16)  NOT NULL,
    scenario_user    VARCHAR(48)  NOT NULL,
    scenario         VARCHAR(48)  NOT NULL,
    content          TEXT         NOT NULL,
    extra            JSONB        NOT NULL DEFAULT '{}'::jsonb,   -- 场景补充字段
    context          JSONB        NOT NULL DEFAULT '{}'::jsonb,   -- 自动采集：页面、UA、版本、IP、账号快照
    attachments      JSONB        NOT NULL DEFAULT '[]'::jsonb,   -- 首次提交的附件 ID
    status           VARCHAR(16)  NOT NULL DEFAULT 'pending',
    severity         VARCHAR(16),                                 -- critical / major / minor / invalid，管理员定级
    tags             TEXT[]       NOT NULL DEFAULT '{}',
    close_reason     TEXT,
    merged_into_id   BIGINT       REFERENCES plus_feedback(id) ON DELETE SET NULL,
    prev_id          BIGINT       REFERENCES plus_feedback(id) ON DELETE SET NULL,
    reward_points    BIGINT       NOT NULL DEFAULT 0,
    reward_ledger_id BIGINT,
    user_unread      BOOLEAN      NOT NULL DEFAULT FALSE,
    asked_at         TIMESTAMPTZ,                                 -- 最近一次进入「待补充」的时间，用于超时自动结案
    last_activity_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    closed_at        TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_plus_feedback_user_activity ON plus_feedback(user_id, last_activity_at DESC);
CREATE INDEX IF NOT EXISTS idx_plus_feedback_status_activity ON plus_feedback(status, last_activity_at DESC);
CREATE INDEX IF NOT EXISTS idx_plus_feedback_created ON plus_feedback(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_plus_feedback_merged ON plus_feedback(merged_into_id) WHERE merged_into_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_plus_feedback_awaiting ON plus_feedback(asked_at) WHERE status = 'awaiting';
CREATE INDEX IF NOT EXISTS idx_plus_feedback_user_unread ON plus_feedback(user_id) WHERE user_unread;

-- 时间线：用户补充、管理员回复/追问/备注、系统事件。只追加，不修改
-- author_type: user / admin / system；kind: supplement / reply / ask / note / event
-- internal=TRUE 的条目只在管理端返回（内部备注、定级/标签等事件）
CREATE TABLE IF NOT EXISTS plus_feedback_messages (
    id          BIGSERIAL    PRIMARY KEY,
    feedback_id BIGINT       NOT NULL REFERENCES plus_feedback(id) ON DELETE CASCADE,
    author_type VARCHAR(8)   NOT NULL,
    author_id   BIGINT,
    kind        VARCHAR(16)  NOT NULL,
    body        TEXT         NOT NULL DEFAULT '',
    attachments JSONB        NOT NULL DEFAULT '[]'::jsonb,
    event       JSONB,                                          -- 系统事件的结构化内容
    internal    BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plus_feedback_messages_feedback ON plus_feedback_messages(feedback_id, id);

-- 附件（先上传后绑定；文件存本地数据卷，只能经鉴权接口读取）
CREATE TABLE IF NOT EXISTS plus_feedback_attachments (
    id          BIGSERIAL    PRIMARY KEY,
    uploader_id BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    feedback_id BIGINT       REFERENCES plus_feedback(id) ON DELETE CASCADE,
    path        TEXT         NOT NULL,
    mime        VARCHAR(32)  NOT NULL,
    size        BIGINT       NOT NULL,
    width       INT          NOT NULL DEFAULT 0,
    height      INT          NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plus_feedback_attachments_feedback ON plus_feedback_attachments(feedback_id);
CREATE INDEX IF NOT EXISTS idx_plus_feedback_attachments_orphan ON plus_feedback_attachments(created_at) WHERE feedback_id IS NULL;

-- 配置（单行乐观锁）
CREATE TABLE IF NOT EXISTS plus_feedback_config (
    id         SMALLINT    PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    version    BIGINT      NOT NULL DEFAULT 1,
    data       JSONB       NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
