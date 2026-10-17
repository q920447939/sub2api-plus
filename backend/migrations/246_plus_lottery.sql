-- [plus] 充值刮刮乐。所有表以 plus_lottery_ 为前缀，与上游表无外键之外的耦合。

-- 配置单行表：version 用于管理端保存时的乐观锁（CAS）。
CREATE TABLE IF NOT EXISTS plus_lottery_config (
    id          SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    version     BIGINT      NOT NULL DEFAULT 1,
    data        JSONB       NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 已计入活动的充值订单。order_id 主键保证每笔订单只发放一次机会；amount 为订单实付（pay_amount）。
-- settled_refund 记录已经处理过的上游退款额（payment_orders.refund_amount），用于退款对账的幂等。
CREATE TABLE IF NOT EXISTS plus_lottery_orders (
    order_id        BIGINT PRIMARY KEY,
    user_id         BIGINT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount          DECIMAL(20,8) NOT NULL,
    chances         INT           NOT NULL,
    settled_refund  DECIMAL(20,8) NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plus_lottery_orders_user ON plus_lottery_orders(user_id);
CREATE INDEX IF NOT EXISTS idx_plus_lottery_orders_created ON plus_lottery_orders(created_at);

-- 刮刮卡（抽奖机会）。status: unused / used / voided；过期 = unused 且 expires_at <= NOW()
CREATE TABLE IF NOT EXISTS plus_lottery_chances (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    order_id      BIGINT        NOT NULL REFERENCES plus_lottery_orders(order_id) ON DELETE CASCADE,
    seq           INT           NOT NULL,
    order_amount  DECIMAL(20,8) NOT NULL,
    status        VARCHAR(16)   NOT NULL DEFAULT 'unused',
    expires_at    TIMESTAMPTZ   NOT NULL,
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE (order_id, seq)
);
CREATE INDEX IF NOT EXISTS idx_plus_lottery_chances_user_status ON plus_lottery_chances(user_id, status, id);

-- 真实开奖记录。chance_id 唯一保证一张卡只开一次；(user_id, idempotency_key) 唯一保证重复提交返回同一结果。
CREATE TABLE IF NOT EXISTS plus_lottery_draws (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chance_id        BIGINT        NOT NULL UNIQUE REFERENCES plus_lottery_chances(id) ON DELETE CASCADE,
    order_id         BIGINT        NOT NULL,
    idempotency_key  VARCHAR(64)   NOT NULL,
    amount           DECIMAL(20,8) NOT NULL,
    tier_index       INT           NOT NULL,
    degraded         VARCHAR(32)   NOT NULL DEFAULT '',
    clawback_amount  DECIMAL(20,8) NOT NULL DEFAULT 0,
    clawed_at        TIMESTAMPTZ,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_plus_lottery_draws_created ON plus_lottery_draws(created_at);
CREATE INDEX IF NOT EXISTS idx_plus_lottery_draws_user_created ON plus_lottery_draws(user_id, created_at);

-- 中奖动态：真实 + 虚拟混存，供滚动记录、排行榜、参与人数使用。
-- virtual_slot 为分钟级时间戳，唯一约束保证多实例下每分钟最多一条虚拟记录。
CREATE TABLE IF NOT EXISTS plus_lottery_feed (
    id            BIGSERIAL PRIMARY KEY,
    display_name  VARCHAR(128)  NOT NULL,
    amount        DECIMAL(20,8) NOT NULL,
    is_virtual    BOOLEAN       NOT NULL,
    draw_id       BIGINT UNIQUE,
    virtual_slot  BIGINT UNIQUE,
    created_at    TIMESTAMPTZ   NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_plus_lottery_feed_created ON plus_lottery_feed(created_at);

-- 每日发奖预算计数器：条件 UPDATE 实现跨实例的原子预算扣减。
CREATE TABLE IF NOT EXISTS plus_lottery_daily_budget (
    day      DATE PRIMARY KEY,
    awarded  DECIMAL(20,8) NOT NULL DEFAULT 0
);
