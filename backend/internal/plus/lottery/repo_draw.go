package lottery

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// DrawRequest 一次刮卡请求。ChanceID=0 表示自动选取最早的一张未使用卡片。
type DrawRequest struct {
	UserID         int64
	ChanceID       int64
	IdempotencyKey string
	Day            DayRange
}

// DrawResult 开奖结果。Replayed=true 表示命中幂等键，返回的是已有结果。
type DrawResult struct {
	DrawID      int64     `json:"draw_id"`
	ChanceID    int64     `json:"chance_id"`
	Amount      float64   `json:"amount"`
	TierIndex   int       `json:"tier_index"`
	Degraded    string    `json:"degraded,omitempty"`
	Remaining   int       `json:"remaining"`
	Replayed    bool      `json:"replayed"`
	CreatedAt   time.Time `json:"created_at"`
	DisplayName string    `json:"-"`
}

// DecideFunc 在事务内、持有用户行锁的情况下计算奖金
type DecideFunc func(in DrawInput) DrawOutcome

// RedeemCodePrefix 抽奖奖金兑换码前缀，码为 LOTTERY-<draw_id>，长度不超过 32
const RedeemCodePrefix = "LOTTERY-"

// Draw 在单个事务中完成：幂等检查 → 锁用户 → 选卡 → 计算奖金 → 预算扣减 → 消费卡片 →
// 写开奖记录 → 写兑换码 → 加余额 → 写动态。锁用户行保证同一用户的抽奖串行执行，
// 单用户每日上限因此是精确的；预算用条件 UPDATE 保证跨用户不超发（保底金额除外）。
func (r *Repository) Draw(ctx context.Context, req DrawRequest, decide DecideFunc, floor float64, dailyBudget float64, notes string) (*DrawResult, error) {
	var res *DrawResult
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		var email string
		if err := tx.QueryRowContext(ctx,
			`SELECT email FROM users WHERE id = $1 FOR UPDATE`, req.UserID).Scan(&email); err != nil {
			return fmt.Errorf("lock user: %w", err)
		}

		// 1. 幂等：同一 key 已开过奖则直接返回原结果（在用户锁之后检查，避免并发同 key 双开）
		if existing, err := r.findDrawByKey(ctx, tx, req.UserID, req.IdempotencyKey); err != nil {
			return err
		} else if existing != nil {
			res = existing
			return nil
		}

		// 2. 选卡并加锁
		var (
			chanceID     int64
			redeemCodeID int64
			codeValue    float64
			status       string
			expired      bool
		)
		var err error
		if req.ChanceID > 0 {
			err = tx.QueryRowContext(ctx,
				`SELECT id, redeem_code_id, code_value::float8, status, expires_at <= NOW() FROM plus_lottery_chances
				 WHERE id = $1 AND user_id = $2 FOR UPDATE`,
				req.ChanceID, req.UserID).Scan(&chanceID, &redeemCodeID, &codeValue, &status, &expired)
		} else {
			// 优先刮最快过期的卡
			status = ChanceStatusUnused
			err = tx.QueryRowContext(ctx,
				`SELECT id, redeem_code_id, code_value::float8 FROM plus_lottery_chances
				 WHERE user_id = $1 AND status = $2 AND expires_at > NOW()
				 ORDER BY expires_at, id LIMIT 1 FOR UPDATE`,
				req.UserID, ChanceStatusUnused).Scan(&chanceID, &redeemCodeID, &codeValue)
		}
		if errors.Is(err, sql.ErrNoRows) {
			if req.ChanceID > 0 {
				return ErrChanceNotFound
			}
			return ErrNoChancesLeft
		}
		if err != nil {
			return fmt.Errorf("select chance: %w", err)
		}
		if status != ChanceStatusUnused {
			return ErrChanceAlreadyUsed
		}
		if expired {
			return ErrChanceExpired
		}

		// 3. 读取风控上下文并计算奖金
		in := DrawInput{CodeValue: codeValue}
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE(SUM(amount), 0)::float8 FROM plus_lottery_draws
			 WHERE user_id = $1 AND created_at >= $2 AND created_at < $3`,
			req.UserID, req.Day.Start, req.Day.End).Scan(&in.UserAwardedToday); err != nil {
			return fmt.Errorf("sum user awarded: %w", err)
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE((SELECT awarded FROM plus_lottery_daily_budget WHERE day = $1), 0)::float8`,
			req.Day.Key).Scan(&in.BudgetUsedToday); err != nil {
			return fmt.Errorf("read budget: %w", err)
		}
		out := decide(in)

		// 4. 预算扣减：条件更新失败（并发下被别的用户用掉）则降级为保底；保底不受预算约束
		if err := r.reserveBudget(ctx, tx, req.Day.Key, &out, floor, dailyBudget); err != nil {
			return err
		}

		// 5. 消费卡片
		if _, err := tx.ExecContext(ctx,
			`UPDATE plus_lottery_chances SET status = $1, updated_at = NOW() WHERE id = $2`,
			ChanceStatusUsed, chanceID); err != nil {
			return fmt.Errorf("consume chance: %w", err)
		}

		// 6. 开奖记录
		d := &DrawResult{ChanceID: chanceID, Amount: out.Amount, TierIndex: out.TierIndex, Degraded: out.Degraded}
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO plus_lottery_draws
			   (user_id, chance_id, redeem_code_id, idempotency_key, amount, tier_index, degraded)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 RETURNING id, created_at`,
			req.UserID, chanceID, redeemCodeID, req.IdempotencyKey, out.Amount, out.TierIndex, out.Degraded,
		).Scan(&d.DrawID, &d.CreatedAt); err != nil {
			return fmt.Errorf("insert draw: %w", err)
		}

		// 7. 发放余额：唯一兑换码让余额历史可见，也是第三层幂等兜底
		code := RedeemCodePrefix + strconv.FormatInt(d.DrawID, 10)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO redeem_codes (code, type, value, status, used_by, used_at, notes)
			 VALUES ($1, $2, $3, $4, $5, NOW(), $6)`,
			code, redeemTypeBalance, out.Amount, redeemStatusUsed, req.UserID, notes); err != nil {
			return fmt.Errorf("insert redeem code: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET balance = balance + $1, updated_at = NOW() WHERE id = $2`,
			out.Amount, req.UserID); err != nil {
			return fmt.Errorf("credit balance: %w", err)
		}

		// 8. 中奖动态（真实记录）
		d.DisplayName = MaskEmail(email)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO plus_lottery_feed (display_name, amount, is_virtual, draw_id, created_at)
			 VALUES ($1, $2, FALSE, $3, $4)`,
			d.DisplayName, out.Amount, d.DrawID, d.CreatedAt); err != nil {
			return fmt.Errorf("insert feed: %w", err)
		}
		res = d
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (r *Repository) reserveBudget(ctx context.Context, tx *sql.Tx, day string, out *DrawOutcome, floor, budget float64) error {
	if budget > 0 && out.Amount > floor {
		var awarded float64
		err := tx.QueryRowContext(ctx,
			`INSERT INTO plus_lottery_daily_budget (day, awarded) VALUES ($1, $2)
			 ON CONFLICT (day) DO UPDATE SET awarded = plus_lottery_daily_budget.awarded + EXCLUDED.awarded
			 WHERE plus_lottery_daily_budget.awarded + EXCLUDED.awarded <= $3
			 RETURNING awarded::float8`,
			day, out.Amount, budget).Scan(&awarded)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("reserve budget: %w", err)
		}
		*out = DrawOutcome{Amount: floor, TierIndex: -1, Degraded: DegradedDailyBudget}
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO plus_lottery_daily_budget (day, awarded) VALUES ($1, $2)
		 ON CONFLICT (day) DO UPDATE SET awarded = plus_lottery_daily_budget.awarded + EXCLUDED.awarded`,
		day, out.Amount)
	if err != nil {
		return fmt.Errorf("add budget: %w", err)
	}
	return nil
}

func (r *Repository) findDrawByKey(ctx context.Context, tx *sql.Tx, userID int64, key string) (*DrawResult, error) {
	d := &DrawResult{Replayed: true}
	err := tx.QueryRowContext(ctx,
		`SELECT id, chance_id, amount::float8, tier_index, degraded, created_at
		 FROM plus_lottery_draws WHERE user_id = $1 AND idempotency_key = $2`,
		userID, key).Scan(&d.DrawID, &d.ChanceID, &d.Amount, &d.TierIndex, &d.Degraded, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find draw by key: %w", err)
	}
	return d, nil
}
