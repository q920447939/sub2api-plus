-- 265_plus_email_verify.sql
-- [plus] 邮箱验证。全部独立表，不改动 users 表。
--
-- 「已验证」是邮箱的属性，不是用户的：一条验证记录 = 某用户验证过某个邮箱。
-- 用户换邮箱后，新邮箱没有记录，自然回到「待验证」，不需要感知上游的换邮箱流程。

-- 验证记录：每个 用户 × 邮箱 一行，来源包括 注册 / 绑定邮箱 / 第三方登录 / 站内验证码 / 邀请链接
CREATE TABLE IF NOT EXISTS plus_email_verifications (
    id            BIGSERIAL    PRIMARY KEY,
    user_id       BIGINT       NOT NULL,
    email         VARCHAR(255) NOT NULL,              -- 小写
    source        VARCHAR(16)  NOT NULL,              -- register | bind | oauth | code | invite
    reward_points BIGINT       NOT NULL DEFAULT 0,    -- 本次验证发放的积分（0 = 没发）
    verified_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT plus_email_verifications_user_email_unique UNIQUE (user_id, email)
);
CREATE INDEX IF NOT EXISTS idx_plus_email_verifications_time ON plus_email_verifications (verified_at DESC);

-- 用户级状态：无效邮箱（硬退信，绑定到具体邮箱）、奖励是否领过（每人一次）
CREATE TABLE IF NOT EXISTS plus_user_email_states (
    user_id           BIGINT       PRIMARY KEY,
    invalid_email     VARCHAR(255),
    invalid_at        TIMESTAMPTZ,
    reward_claimed_at TIMESTAMPTZ,
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- 一次性验证邀请 token：点击邮件链接即验证
CREATE TABLE IF NOT EXISTS plus_emailverify_tokens (
    id          BIGSERIAL    PRIMARY KEY,
    token       VARCHAR(64)  NOT NULL,
    user_id     BIGINT       NOT NULL,
    email       VARCHAR(255) NOT NULL,
    expires_at  TIMESTAMPTZ  NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT plus_emailverify_tokens_token_unique UNIQUE (token)
);
CREATE INDEX IF NOT EXISTS idx_plus_emailverify_tokens_user ON plus_emailverify_tokens (user_id) WHERE consumed_at IS NULL;

-- 对外契约：其他模块（如触达中心）只读这个视图，不读上面的表。
-- 每个用户「当前邮箱」的状态：verified_at 非空 = 当前邮箱已验证；invalid_at 非空 = 当前邮箱已硬退信。
CREATE OR REPLACE VIEW plus_email_status AS
SELECT u.id AS user_id,
       (SELECT MAX(v.verified_at) FROM plus_email_verifications v
         WHERE v.user_id = u.id AND v.email = LOWER(u.email))           AS verified_at,
       CASE WHEN s.invalid_email = LOWER(u.email) THEN s.invalid_at END AS invalid_at
FROM users u
LEFT JOIN plus_user_email_states s ON s.user_id = u.id;
