package lottery

import (
	"context"
	"time"
)

// DailyStats 管理端统计：汇总当日全部真实开奖记录
type DailyStats struct {
	Day              string               `json:"day"`               // YYYY-MM-DD
	DrawCount        int                  `json:"draw_count"`        // 抽奖次数
	UniqueUsers      int                  `json:"unique_users"`      // 参与人数
	TotalAwarded     float64              `json:"total_awarded"`     // 发出金额
	UnusedChances    int                  `json:"unused_chances"`    // 当日发放、未使用且未过期的机会
	ExpiredChances   int                  `json:"expired_chances"`   // 当日发放、未使用已过期的机会
	GrantedChances   int                  `json:"granted_chances"`   // 当日发放机会
	RedeemCount      int                  `json:"redeem_count"`      // 当日计入活动（得卡）的卡密数
	RedeemAmount     float64              `json:"redeem_amount"`     // 当日计入活动的卡密面值合计
	ReturnRate       float64              `json:"return_rate"`       // 返奖率 = 发出金额 / 卡密面值合计
	TierDistribution map[int]TierStatsRow `json:"tier_distribution"` // 奖档分布
}

type TierStatsRow struct {
	Count  int     `json:"count"`
	Amount float64 `json:"amount"`
}

// AdminDailyStats 管理端读取当日统计
func (r *Repository) AdminDailyStats(ctx context.Context, day DayRange) (*DailyStats, error) {
	st := &DailyStats{Day: day.Key, TierDistribution: map[int]TierStatsRow{}}
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COUNT(DISTINCT user_id), COALESCE(SUM(amount), 0)::float8
		 FROM plus_lottery_draws
		 WHERE created_at >= $1 AND created_at < $2`,
		day.Start, day.End).Scan(&st.DrawCount, &st.UniqueUsers, &st.TotalAwarded)
	if err != nil {
		return nil, err
	}

	err = r.db.QueryRowContext(ctx,
		`SELECT COUNT(*),
		        COUNT(*) FILTER (WHERE status = $3 AND expires_at > NOW()),
		        COUNT(*) FILTER (WHERE status = $3 AND expires_at <= NOW())
		 FROM plus_lottery_chances
		 WHERE created_at >= $1 AND created_at < $2`,
		day.Start, day.End, ChanceStatusUnused).Scan(&st.GrantedChances, &st.UnusedChances, &st.ExpiredChances)
	if err != nil {
		return nil, err
	}
	err = r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(amount), 0)::float8 FROM plus_lottery_redeems
		 WHERE created_at >= $1 AND created_at < $2 AND chances > 0`,
		day.Start, day.End).Scan(&st.RedeemCount, &st.RedeemAmount)
	if err != nil {
		return nil, err
	}
	if st.RedeemAmount > 0 {
		st.ReturnRate = roundCents(st.TotalAwarded/st.RedeemAmount*10000) / 10000
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT tier_index, COUNT(*), COALESCE(SUM(amount), 0)::float8
		 FROM plus_lottery_draws
		 WHERE created_at >= $1 AND created_at < $2
		 GROUP BY tier_index`, day.Start, day.End)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var idx int
		var row TierStatsRow
		if err := rows.Scan(&idx, &row.Count, &row.Amount); err != nil {
			return nil, err
		}
		st.TierDistribution[idx] = row
	}
	return st, rows.Err()
}

// AdminDrawRecord 管理端明细行，含来源卡密和降级原因
type AdminDrawRecord struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	DisplayName  string    `json:"display_name"` // 脱敏
	ChanceID     int64     `json:"chance_id"`
	RedeemCodeID int64     `json:"redeem_code_id"`
	RedeemCode   string    `json:"redeem_code"`
	CodeValue    float64   `json:"code_value"`
	Amount       float64   `json:"amount"`
	TierIndex    int       `json:"tier_index"`
	Degraded     string    `json:"degraded,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// AdminListDraws 管理端明细查询，分页，可按 user_id 过滤
func (r *Repository) AdminListDraws(ctx context.Context, day DayRange, userID int64, limit, offset int) ([]AdminDrawRecord, int, error) {
	query := `SELECT d.id, d.user_id, u.email, d.chance_id, d.redeem_code_id, COALESCE(rc.code, ''),
	          ch.code_value::float8, d.amount::float8, d.tier_index, d.degraded, d.created_at
	          FROM plus_lottery_draws d
	          JOIN users u ON u.id = d.user_id
	          JOIN plus_lottery_chances ch ON ch.id = d.chance_id
	          LEFT JOIN redeem_codes rc ON rc.id = d.redeem_code_id
	          WHERE d.created_at >= $1 AND d.created_at < $2 AND ($3::bigint = 0 OR d.user_id = $3::bigint)
	          ORDER BY d.id DESC LIMIT $4 OFFSET $5`
	rows, err := r.db.QueryContext(ctx, query, day.Start, day.End, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := []AdminDrawRecord{}
	for rows.Next() {
		var d AdminDrawRecord
		var email string
		if err := rows.Scan(&d.ID, &d.UserID, &email, &d.ChanceID, &d.RedeemCodeID, &d.RedeemCode,
			&d.CodeValue, &d.Amount, &d.TierIndex, &d.Degraded, &d.CreatedAt); err != nil {
			return nil, 0, err
		}
		d.DisplayName = MaskEmail(email)
		list = append(list, d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var total int
	query = `SELECT COUNT(*) FROM plus_lottery_draws d
	         WHERE d.created_at >= $1 AND d.created_at < $2 AND ($3::bigint = 0 OR d.user_id = $3::bigint)`
	err = r.db.QueryRowContext(ctx, query, day.Start, day.End, userID).Scan(&total)
	return list, total, err
}

// UserDraw 用户端"我的记录"
type UserDraw struct {
	ID        int64     `json:"id"`
	Amount    float64   `json:"amount"`
	CreatedAt time.Time `json:"created_at"`
}

// ListUserDraws 用户自己的开奖记录
func (r *Repository) ListUserDraws(ctx context.Context, userID int64, limit int) ([]UserDraw, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, amount::float8, created_at FROM plus_lottery_draws
		 WHERE user_id = $1 ORDER BY id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []UserDraw{}
	for rows.Next() {
		var d UserDraw
		if err := rows.Scan(&d.ID, &d.Amount, &d.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}
