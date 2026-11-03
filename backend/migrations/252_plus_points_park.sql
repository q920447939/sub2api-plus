-- [plus] 积分乐园：废弃刮刮乐；幸运九宫格改为概率 + 双奖金罐；兑换卡密送积分
-- 开发阶段重构，不兼容旧配置。历史 LOTTERY-* 余额兑换码属于上游 redeem_codes，保留。

-- 1. 刮刮乐整体下线
DROP TABLE IF EXISTS plus_lottery_redeems, plus_lottery_daily_budget, plus_lottery_feed,
    plus_lottery_draws, plus_lottery_chances, plus_lottery_orders, plus_lottery_config CASCADE;

-- 2. 修正器参数改由各业务配置提供（连签加成在签到配置里），不再单独建表
DROP TABLE IF EXISTS plus_slot_modifiers;

-- 3. 九宫格配置结构变化（权重 → 概率），清掉后服务端回落到默认配置（默认关闭）
DELETE FROM plus_slot_config;

-- 4. 每次摇奖记录本次因奖金罐不足被排除的奖档、出奖的罐与支出
ALTER TABLE plus_slot_spins
    ADD COLUMN IF NOT EXISTS excluded_tiers INT[]       NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS boosted        BOOLEAN     NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS pot            VARCHAR(16),
    ADD COLUMN IF NOT EXISTS pot_payout     BIGINT      NOT NULL DEFAULT 0;

-- 5. 奖金罐：small=小奖罐，jackpot=大奖罐；单位为积分（余额奖按积分估值折算）
CREATE TABLE IF NOT EXISTS plus_slot_pots (
    pot        VARCHAR(16) PRIMARY KEY,
    balance    BIGINT      NOT NULL DEFAULT 0 CHECK (balance >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO plus_slot_pots (pot, balance) VALUES ('small', 0), ('jackpot', 0) ON CONFLICT (pot) DO NOTHING;

-- 奖金罐流水：inflow 摇奖注入 / payout 中奖支出 / admin_add 管理员加水 / admin_sub 管理员抽水
CREATE TABLE IF NOT EXISTS plus_slot_pot_ledger (
    id          BIGSERIAL   PRIMARY KEY,
    pot         VARCHAR(16) NOT NULL REFERENCES plus_slot_pots(pot),
    delta       BIGINT      NOT NULL,
    balance     BIGINT      NOT NULL,
    reason      VARCHAR(16) NOT NULL,
    spin_id     BIGINT      REFERENCES plus_slot_spins(id) ON DELETE SET NULL,
    operator_id BIGINT,
    note        VARCHAR(255),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plus_slot_pot_ledger_created ON plus_slot_pot_ledger(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_plus_slot_pot_ledger_manual ON plus_slot_pot_ledger(id DESC)
    WHERE reason IN ('admin_add', 'admin_sub');

-- 6. 兑换卡密送积分：每张卡密最多一行，钩子与补偿任务靠主键去重
-- source: user 用户自助兑换 / payment 在线支付履约 / admin 管理员代发（后两者 points=0，仅登记）
CREATE TABLE IF NOT EXISTS plus_points_redeem_grants (
    redeem_code_id BIGINT        PRIMARY KEY,
    user_id        BIGINT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source         VARCHAR(16)   NOT NULL,
    code_value     DECIMAL(20,8) NOT NULL,
    points         BIGINT        NOT NULL DEFAULT 0,
    ledger_id      BIGINT        REFERENCES plus_points_ledger(id),
    via            VARCHAR(16)   NOT NULL,
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plus_points_redeem_grants_user ON plus_points_redeem_grants(user_id, created_at DESC);
