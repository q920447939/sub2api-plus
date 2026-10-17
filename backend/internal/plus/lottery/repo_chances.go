package lottery

import (
	"context"
	"database/sql"
	"time"
)

// ─── 机会发放：从上游已兑换的余额卡密同步，不侵入兑换链路 ──────────────────────

// pendingRedeem 尚未计入活动的已兑换余额卡密
type pendingRedeem struct {
	RedeemCodeID int64
	UserID       int64
	Value        float64 // 卡密面值
	UsedAt       time.Time
}

// listPendingRedeems 查询 used_at >= since 且未计入活动的余额卡密。userID=0 表示全站。
// 排除：刮刮乐自己发奖的 LOTTERY-* 码、在线支付生成的码（payment_orders.recharge_code）。
// notesKeyword 非空时只计入备注包含该关键词的卡密。管理员手动调余额是 admin_balance 类型，天然不在范围内。
func (r *Repository) listPendingRedeems(ctx context.Context, since time.Time, userID int64, notesKeyword string, limit int) ([]pendingRedeem, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT rc.id, rc.used_by, rc.value::float8, rc.used_at
		 FROM redeem_codes rc
		 WHERE rc.type = $1 AND rc.status = $2 AND rc.used_by IS NOT NULL AND rc.used_at >= $3
		   AND rc.value > 0
		   AND ($4::bigint = 0 OR rc.used_by = $4::bigint)
		   AND rc.code NOT LIKE $5
		   AND ($6 = '' OR COALESCE(rc.notes, '') LIKE '%' || $6 || '%')
		   AND NOT EXISTS (SELECT 1 FROM payment_orders po WHERE po.recharge_code = rc.code)
		   AND NOT EXISTS (SELECT 1 FROM plus_lottery_redeems lr WHERE lr.redeem_code_id = rc.id)
		 ORDER BY rc.id
		 LIMIT $7`,
		redeemTypeBalance, redeemStatusUsed, since, userID, RedeemCodePrefix+"%", notesKeyword, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []pendingRedeem
	for rows.Next() {
		var p pendingRedeem
		if err := rows.Scan(&p.RedeemCodeID, &p.UserID, &p.Value, &p.UsedAt); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

// grantRedeem 幂等地登记卡密并发放机会。redeem_code_id 主键冲突即视为已发放。
// 不满最低档的卡密同样登记（chances=0），避免反复扫描。卡片有效期从兑换时刻起算。
func (r *Repository) grantRedeem(ctx context.Context, p pendingRedeem, chances int, valid time.Duration) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO plus_lottery_redeems (redeem_code_id, user_id, amount, chances)
			 VALUES ($1, $2, $3, $4) ON CONFLICT (redeem_code_id) DO NOTHING`,
			p.RedeemCodeID, p.UserID, p.Value, chances)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 || chances == 0 {
			return nil
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO plus_lottery_chances (user_id, redeem_code_id, seq, code_value, expires_at)
			 SELECT $1, $2, s, $3, $5 FROM generate_series(1, $4) AS s
			 ON CONFLICT (redeem_code_id, seq) DO NOTHING`,
			p.UserID, p.RedeemCodeID, p.Value, chances, p.UsedAt.Add(valid))
		return err
	})
}

// CountUnusedChances 用户剩余（未过期）刮刮卡数，以及最早一张的过期时间
func (r *Repository) CountUnusedChances(ctx context.Context, userID int64) (int, *time.Time, error) {
	var (
		n    int
		next sql.NullTime
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), MIN(expires_at) FROM plus_lottery_chances
		 WHERE user_id = $1 AND status = $2 AND expires_at > NOW()`,
		userID, ChanceStatusUnused).Scan(&n, &next)
	if err != nil || !next.Valid {
		return n, nil, err
	}
	return n, &next.Time, nil
}

// LatestRedeemGrant 用户最近一次兑换的余额卡密带来的刮刮卡数；未计入活动或没有兑换返回 0。
// 供兑换成功后的提示使用：宿主页兑换后会清空输入框，组件拿不到卡密，按"最近一次"查即可。
// 只看 within 时间窗内的兑换，避免页面停留很久后误报。
func (r *Repository) LatestRedeemGrant(ctx context.Context, userID int64, within time.Duration) (int, error) {
	var n sql.NullInt64
	err := r.db.QueryRowContext(ctx,
		`SELECT lr.chances FROM redeem_codes rc
		 LEFT JOIN plus_lottery_redeems lr ON lr.redeem_code_id = rc.id
		 WHERE rc.used_by = $1 AND rc.type = $2 AND rc.status = $3 AND rc.used_at >= $4
		   AND rc.code NOT LIKE $5
		 ORDER BY rc.used_at DESC, rc.id DESC LIMIT 1`,
		userID, redeemTypeBalance, redeemStatusUsed, time.Now().Add(-within), RedeemCodePrefix+"%").Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return int(n.Int64), err
}
