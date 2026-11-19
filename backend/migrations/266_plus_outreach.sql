-- 266_plus_outreach.sql
-- [plus] 用户触达中心。全部独立表，不改动上游表；投递设置存 plus_settings（key = outreach）。
-- 邮箱是否已验证 / 无效只读 plus_email_status 视图（邮箱验证模块的对外契约）。
--   外壳   plus_mail_layouts      ：页头 / 版式 / 页脚品牌区（合规区由系统追加，不在这里）
--   模板   plus_mail_templates    ：按「目的」分类的主题 + 正文，可增删改查；内置模板不可删除
--   活动   plus_outreach_campaigns：目的 + 人群 + 内容 + 外壳 + 落地页 + 权益，本身不发信
--   批次   plus_outreach_batches  ：活动的一次执行，冻结内容 / 外壳 / 落地页 / 权益快照
--   收件人 plus_outreach_recipients：批次名单；直发（不经活动）的收件人 batch_id / campaign_id 为空
--   台账   plus_outreach_ledger   ：每封已发邮件一行，频控与「本活动已发送」只读它，批次清理后仍保留
--   退订   plus_outreach_unsubscribes：按目的分别退订（服务类不可退订）

CREATE TABLE IF NOT EXISTS plus_mail_layouts (
    id         BIGSERIAL    PRIMARY KEY,
    name       VARCHAR(60)  NOT NULL,
    is_default BOOLEAN      NOT NULL DEFAULT FALSE,
    config     JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_plus_mail_layouts_default ON plus_mail_layouts (is_default) WHERE is_default;

CREATE TABLE IF NOT EXISTS plus_mail_templates (
    id         BIGSERIAL    PRIMARY KEY,
    name       VARCHAR(60)  NOT NULL,
    purpose    VARCHAR(20)  NOT NULL,
    subject    VARCHAR(200) NOT NULL,
    body_md    TEXT         NOT NULL,
    layout_id  BIGINT       REFERENCES plus_mail_layouts(id),
    builtin    BOOLEAN      NOT NULL DEFAULT FALSE, -- 系统预置：可编辑，不可删除；直发（如单发验证邀请）使用该目的的内置模板
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plus_mail_templates_purpose ON plus_mail_templates (purpose, id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_plus_mail_templates_builtin ON plus_mail_templates (purpose) WHERE builtin;

CREATE TABLE IF NOT EXISTS plus_outreach_campaigns (
    id          BIGSERIAL     PRIMARY KEY,
    name        VARCHAR(120)  NOT NULL,
    purpose     VARCHAR(20)   NOT NULL,
    template_id BIGINT,                              -- 内容来源模板（仅展示用，内容已复制到活动）
    layout_id   BIGINT        REFERENCES plus_mail_layouts(id), -- 空 = 默认外壳
    subject     VARCHAR(200)  NOT NULL,
    body_md     TEXT          NOT NULL,
    segment     JSONB         NOT NULL DEFAULT '{}'::jsonb,
    benefits    JSONB         NOT NULL DEFAULT '[]'::jsonb,
    landing_url VARCHAR(500)  NOT NULL DEFAULT '',   -- 空 = 站点首页
    claim_days  INT           NOT NULL DEFAULT 30,   -- 附带权益时：发出后多少天内可领
    created_by  BIGINT        NOT NULL DEFAULT 0,
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS plus_outreach_batches (
    id           BIGSERIAL    PRIMARY KEY,
    campaign_id  BIGINT       NOT NULL REFERENCES plus_outreach_campaigns(id),
    name         VARCHAR(120) NOT NULL DEFAULT '',
    status       VARCHAR(20)  NOT NULL DEFAULT 'pending', -- pending | running | paused | done | cancelled
    pause_reason VARCHAR(255) NOT NULL DEFAULT '',
    pick_mode    VARCHAR(12)  NOT NULL DEFAULT 'all',     -- all | random | users
    pick_count   INT          NOT NULL DEFAULT 0,
    scope        VARCHAR(10)  NOT NULL DEFAULT 'fresh',   -- fresh = 只发本活动未收到的人 | again = 再发一轮
    -- 快照：生成批次时冻结，之后改活动 / 模板 / 外壳都不影响本批次
    purpose      VARCHAR(20)  NOT NULL,
    subject      VARCHAR(200) NOT NULL,
    body_md      TEXT         NOT NULL,
    layout       JSONB        NOT NULL DEFAULT '{}'::jsonb,
    landing_url  VARCHAR(500) NOT NULL DEFAULT '',
    benefits     JSONB        NOT NULL DEFAULT '[]'::jsonb,
    claim_days   INT          NOT NULL DEFAULT 30,
    segment      JSONB        NOT NULL DEFAULT '{}'::jsonb,
    total        INT          NOT NULL DEFAULT 0,
    created_by   BIGINT       NOT NULL DEFAULT 0,
    started_at   TIMESTAMPTZ,
    finished_at  TIMESTAMPTZ,
    deleted_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plus_outreach_batches_campaign ON plus_outreach_batches (campaign_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_plus_outreach_batches_running  ON plus_outreach_batches (id) WHERE status = 'running';

CREATE TABLE IF NOT EXISTS plus_outreach_recipients (
    id           BIGSERIAL    PRIMARY KEY,
    campaign_id  BIGINT,                                  -- 直发为空
    batch_id     BIGINT       REFERENCES plus_outreach_batches(id) ON DELETE CASCADE, -- 直发为空
    purpose      VARCHAR(20)  NOT NULL,
    user_id      BIGINT       NOT NULL,
    email        VARCHAR(255) NOT NULL,
    token        VARCHAR(64)  NOT NULL,                   -- 点击 / 领取 / 退订凭证
    invite_token VARCHAR(64)  NOT NULL DEFAULT '',        -- 验证邮箱：emailverify 的一次性 token，发信时签发
    vars         JSONB        NOT NULL DEFAULT '{}'::jsonb, -- 用户变量快照（名单生成时 / 实际发出时）
    status       VARCHAR(12)  NOT NULL DEFAULT 'queued',  -- queued | sending | sent | failed | skipped
    next_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    attempts     INT          NOT NULL DEFAULT 0,
    bounced      BOOLEAN      NOT NULL DEFAULT FALSE,
    error        VARCHAR(500) NOT NULL DEFAULT '',
    sent_at      TIMESTAMPTZ,
    clicked_at   TIMESTAMPTZ,
    click_count  INT          NOT NULL DEFAULT 0,
    claimed_at   TIMESTAMPTZ,
    unsub_at     TIMESTAMPTZ,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT plus_outreach_recipients_token_unique UNIQUE (token),
    CONSTRAINT plus_outreach_recipients_batch_user_unique UNIQUE (batch_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_plus_outreach_recipients_queue    ON plus_outreach_recipients (batch_id, id) WHERE status = 'queued';
CREATE INDEX IF NOT EXISTS idx_plus_outreach_recipients_campaign ON plus_outreach_recipients (campaign_id);
CREATE INDEX IF NOT EXISTS idx_plus_outreach_recipients_user     ON plus_outreach_recipients (user_id);

CREATE TABLE IF NOT EXISTS plus_outreach_ledger (
    id          BIGSERIAL   PRIMARY KEY,
    user_id     BIGINT      NOT NULL,
    campaign_id BIGINT,                 -- 直发为空
    batch_id    BIGINT,                 -- 直发为空
    purpose     VARCHAR(20) NOT NULL,
    category    VARCHAR(12) NOT NULL,   -- service | marketing：跨目的的营销频控只看它，不需要认识具体目的
    sent_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plus_outreach_ledger_user     ON plus_outreach_ledger (user_id, sent_at DESC);
CREATE INDEX IF NOT EXISTS idx_plus_outreach_ledger_campaign ON plus_outreach_ledger (campaign_id, user_id);
CREATE INDEX IF NOT EXISTS idx_plus_outreach_ledger_sent_at  ON plus_outreach_ledger (sent_at);

CREATE TABLE IF NOT EXISTS plus_outreach_unsubscribes (
    user_id    BIGINT      NOT NULL,
    purpose    VARCHAR(20) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, purpose)
);

-- ── 预置：默认外壳 + 每个目的一份内置模板 ─────────────────────────────────────
INSERT INTO plus_mail_layouts (name, is_default, config)
SELECT '默认卡片', TRUE,
       '{"header":"text","logo_url":"","header_link":"","theme_color":"#2563eb","style":"card","width":580,"footer_md":"© {{site_name}}"}'::jsonb
WHERE NOT EXISTS (SELECT 1 FROM plus_mail_layouts);

INSERT INTO plus_mail_templates (name, purpose, subject, body_md, builtin)
SELECT v.name, v.purpose, v.subject, v.body_md, TRUE
FROM (VALUES
  ('验证邮箱（内置）', 'verify_email', '【{{site_name}}】请验证您的邮箱',
   E'# {{username}}，请验证您的邮箱\n\n为了及时把余额提醒、订阅到期和重要通知送到您手上，请点击下方按钮完成邮箱验证：\n\n[[验证邮箱]]({{cta_url}})\n\n充值过的老用户验证成功后，可获得 {{verify_reward_points}} 积分（每人一次）。链接只能使用一次。'),
  ('召回（内置）', 'recall', '{{username}}，好久不见',
   E'# {{username}}，好久不见\n\n您已经 {{last_active_days}} 天没有来了，账户里还有 **{{balance}}** 余额等您使用。\n\n[[回来看看]]({{cta_url}})'),
  ('资讯（内置）', 'news', '【{{site_name}}】新功能上线',
   E'# 新功能上线\n\n- 在这里写第一条更新\n- 在这里写第二条更新\n\n[[去看看]]({{cta_url}})'),
  ('活动（内置）', 'promo', '【{{site_name}}】限时活动',
   E'# 限时活动\n\n在这里写活动内容和时间。\n\n[[立即参与]]({{cta_url}})')
) AS v(name, purpose, subject, body_md)
WHERE NOT EXISTS (SELECT 1 FROM plus_mail_templates t WHERE t.builtin AND t.purpose = v.purpose);
