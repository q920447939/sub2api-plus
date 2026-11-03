// Package points — [plus] 过期清零 + 账户/流水查询
package points

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"
)

// ExpireResult 一次过期扫描的结果统计
type ExpireResult struct {
	ExpiredAccounts int64
	ExpiredPoints   int64
}

// ExpireInactive 清零 last_active_at 早于 threshold 的账户，并为每个账户写一条 expire 流水。
// 幂等：余额为 0 的账户跳过。返回 more=true 表示仍有更多待处理，调用方应继续批处理。
func (r *Repository) ExpireInactive(ctx context.Context, threshold time.Time, limit int) (ExpireResult, bool, error) {
	var res ExpireResult
	more := false
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT user_id, balance FROM plus_points_accounts
			 WHERE balance > 0 AND last_active_at < $1
			 ORDER BY user_id LIMIT $2 FOR UPDATE`,
			threshold, limit)
		if err != nil {
			return fmt.Errorf("select expired accounts: %w", err)
		}
		type acc struct {
			userID  int64
			balance int64
		}
		var list []acc
		for rows.Next() {
			var a acc
			if err := rows.Scan(&a.userID, &a.balance); err != nil {
				_ = rows.Close()
				return err
			}
			list = append(list, a)
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(list) == limit {
			more = true
		}
		for _, a := range list {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO plus_points_ledger (user_id, delta, balance, reason, note, created_at)
				 VALUES ($1, $2, 0, $3, $4, NOW())`,
				a.userID, -a.balance, ReasonExpire, "积分过期清零"); err != nil {
				return fmt.Errorf("insert expire ledger for user %d: %w", a.userID, err)
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE plus_points_accounts
				 SET balance = 0, last_active_at = NOW(), updated_at = NOW()
				 WHERE user_id = $1`,
				a.userID); err != nil {
				return fmt.Errorf("zero expired account %d: %w", a.userID, err)
			}
			res.ExpiredAccounts++
			res.ExpiredPoints += a.balance
		}
		return nil
	})
	return res, more, err
}

// Account 读取用户积分账户；账户不存在时返回余额为 0 的零值结构
type Account struct {
	UserID       int64
	Balance      int64
	LastActiveAt time.Time
}

func (r *Repository) Account(ctx context.Context, userID int64) (Account, error) {
	var a Account
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id, balance, last_active_at FROM plus_points_accounts WHERE user_id = $1`,
		userID).Scan(&a.UserID, &a.Balance, &a.LastActiveAt)
	if err == sql.ErrNoRows {
		return Account{UserID: userID}, nil
	}
	if err != nil {
		return Account{}, err
	}
	return a, nil
}

// LedgerItem 流水展示条目
type LedgerItem struct {
	ID        int64     `json:"id"`
	Delta     int64     `json:"delta"`
	Balance   int64     `json:"balance"`
	Reason    string    `json:"reason"`
	RefID     int64     `json:"ref_id,omitempty"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ListLedger 分页查询用户流水
func (r *Repository) ListLedger(ctx context.Context, userID int64, page, pageSize int, reason string) ([]LedgerItem, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	baseArgs := []interface{}{userID}
	where := `WHERE user_id = $1`
	if reason != "" {
		where += ` AND reason = $2`
		baseArgs = append(baseArgs, reason)
	}

	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM plus_points_ledger `+where,
		baseArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	p := len(baseArgs) + 1
	listArgs := append(baseArgs, pageSize, offset)
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, delta, balance, reason, COALESCE(ref_id, 0), COALESCE(note, ''), created_at
		 FROM plus_points_ledger `+where+`
		 ORDER BY created_at DESC, id DESC
		 LIMIT $`+strconv.Itoa(p)+` OFFSET $`+strconv.Itoa(p+1),
		listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []LedgerItem
	for rows.Next() {
		var item LedgerItem
		if err := rows.Scan(&item.ID, &item.Delta, &item.Balance, &item.Reason, &item.RefID, &item.Note, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, item)
	}
	return list, total, rows.Err()
}
