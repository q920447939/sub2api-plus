// Package points — [plus] 积分账户增减（可并入外层事务）
package points

import (
	"context"
	"fmt"
	"time"
)

// LedgerEntry 流水条目（供外层包读取已插入的记录 ID）
type LedgerEntry struct {
	ID        int64
	Balance   int64
	CreatedAt time.Time
}

// Credit 给用户增加积分。dbtx 可以是 *sql.DB 或外层 *sql.Tx。
// reason/refID/note 写入流水；refID <= 0 表示无关联业务。
func Credit(ctx context.Context, dbtx DBTX, userID, delta int64, reason string, refID int64, note string) (LedgerEntry, error) {
	if delta <= 0 {
		return LedgerEntry{}, fmt.Errorf("points.Credit: delta must be > 0, got %d", delta)
	}
	return applyDelta(ctx, dbtx, userID, delta, reason, refID, note)
}

// Debit 从用户扣减积分。余额不足返回 ErrInsufficientPoints。
func Debit(ctx context.Context, dbtx DBTX, userID, delta int64, reason string, refID int64, note string) (LedgerEntry, error) {
	if delta <= 0 {
		return LedgerEntry{}, fmt.Errorf("points.Debit: delta must be > 0, got %d", delta)
	}
	return applyDelta(ctx, dbtx, userID, -delta, reason, refID, note)
}

// applyDelta UPSERT 账户余额并写入流水，原子执行。
// 扣减时若余额不足，UPDATE 命中 0 行，返回 ErrInsufficientPoints。
func applyDelta(ctx context.Context, dbtx DBTX, userID, delta int64, reason string, refID int64, note string) (LedgerEntry, error) {
	var (
		newBalance int64
	)

	if delta > 0 {
		// 增加：UPSERT 账户，余额不会为负
		err := dbtx.QueryRowContext(ctx,
			`INSERT INTO plus_points_accounts (user_id, balance, last_active_at, created_at, updated_at)
			 VALUES ($1, $2, NOW(), NOW(), NOW())
			 ON CONFLICT (user_id) DO UPDATE
			   SET balance        = plus_points_accounts.balance + EXCLUDED.balance,
			       last_active_at = NOW(),
			       updated_at     = NOW()
			 RETURNING balance`,
			userID, delta).Scan(&newBalance)
		if err != nil {
			return LedgerEntry{}, fmt.Errorf("points credit upsert: %w", err)
		}
	} else {
		// 扣减：仅当余额足够才更新
		absDebit := -delta
		err := dbtx.QueryRowContext(ctx,
			`UPDATE plus_points_accounts
			 SET balance        = balance - $1,
			     last_active_at = NOW(),
			     updated_at     = NOW()
			 WHERE user_id = $2 AND balance >= $1
			 RETURNING balance`,
			absDebit, userID).Scan(&newBalance)
		if err != nil {
			// sql.ErrNoRows 意味着账户不存在或余额不足
			return LedgerEntry{}, ErrInsufficientPoints
		}
	}

	// 写流水
	var entry LedgerEntry
	var refIDVal any
	if refID > 0 {
		refIDVal = refID
	}
	err := dbtx.QueryRowContext(ctx,
		`INSERT INTO plus_points_ledger (user_id, delta, balance, reason, ref_id, note, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW())
		 RETURNING id, balance, created_at`,
		userID, delta, newBalance, reason, refIDVal, note).Scan(&entry.ID, &entry.Balance, &entry.CreatedAt)
	if err != nil {
		return LedgerEntry{}, fmt.Errorf("points insert ledger: %w", err)
	}
	return entry, nil
}
