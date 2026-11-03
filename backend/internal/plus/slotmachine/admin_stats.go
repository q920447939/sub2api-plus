// Package slotmachine — [plus] 管理端统计：摇奖趋势、实际返奖率、各奖档实际中出与配置概率对比
package slotmachine

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/plus/points"
)

// SpinDayStat 某天（或某时间段）的摇奖汇总
type SpinDayStat struct {
	Date         string  `json:"date,omitempty"`
	Spins        int64   `json:"spins"`
	Users        int64   `json:"users"`
	Cost         int64   `json:"cost"`          // 消耗积分
	Payout       int64   `json:"payout"`        // 奖金罐支出（积分口径，含余额奖折算）
	PrizePoints  int64   `json:"prize_points"`  // 发出的积分奖
	PrizeBalance float64 `json:"prize_balance"` // 发出的余额奖（$）
	BalanceWins  int64   `json:"balance_wins"`  // 余额奖中出次数
}

// TierStat 某奖档在统计区间内的实际表现
type TierStat struct {
	Index     int    `json:"index"`
	Label     string `json:"label"`
	Emoji     string `json:"emoji,omitempty"`
	PrizeType string `json:"prize_type"`
	ProbBP    int    `json:"prob_bp"` // 当前配置概率
	Hits      int64  `json:"hits"`
	Excluded  int64  `json:"excluded"` // 因奖金罐不足被排除的次数
}

// SlotStats 管理端九宫格统计
type SlotStats struct {
	Days  int           `json:"days"`
	Today SpinDayStat   `json:"today"`
	Range SpinDayStat   `json:"range"` // 统计区间合计
	Trend []SpinDayStat `json:"trend"`
	Tiers []TierStat    `json:"tiers"`
	// TargetRate 按当前配置概率计算的理论返奖率（期望支出 / 单次消耗），不含连签加成
	TargetRate float64 `json:"target_rate"`
}

const spinAggCols = `COUNT(*), COUNT(DISTINCT user_id), COALESCE(SUM(cost), 0), COALESCE(SUM(pot_payout), 0),
	COALESCE(SUM(prize_points), 0), COALESCE(SUM(prize_balance), 0)::float8,
	COUNT(*) FILTER (WHERE prize_type = 'balance')`

// AdminStats 近 days 天（含今天，东八区自然日）的摇奖统计
func (s *Service) AdminStats(ctx context.Context, days int) (*SlotStats, error) {
	if days <= 0 || days > 90 {
		days = 7
	}
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	today := s.dayStart()
	start := today.AddDate(0, 0, -(days - 1))
	db := s.repo.db
	out := &SlotStats{Days: days, Trend: []SpinDayStat{}, Tiers: []TierStat{}}

	scan := func(row interface{ Scan(...any) error }, st *SpinDayStat) error {
		return row.Scan(&st.Spins, &st.Users, &st.Cost, &st.Payout, &st.PrizePoints, &st.PrizeBalance, &st.BalanceWins)
	}
	if err := scan(db.QueryRowContext(ctx, `SELECT `+spinAggCols+` FROM plus_slot_spins WHERE created_at >= $1`, today), &out.Today); err != nil {
		return nil, err
	}
	if err := scan(db.QueryRowContext(ctx, `SELECT `+spinAggCols+` FROM plus_slot_spins WHERE created_at >= $1`, start), &out.Range); err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx,
		`SELECT (created_at AT TIME ZONE $2)::date AS d, `+spinAggCols+`
		 FROM plus_slot_spins WHERE created_at >= $1 GROUP BY d`, start, points.PgTimeZone())
	if err != nil {
		return nil, err
	}
	byDate := map[string]SpinDayStat{}
	for rows.Next() {
		var d time.Time
		var st SpinDayStat
		if err := rows.Scan(&d, &st.Spins, &st.Users, &st.Cost, &st.Payout, &st.PrizePoints, &st.PrizeBalance, &st.BalanceWins); err != nil {
			rows.Close()
			return nil, err
		}
		st.Date = d.Format("2006-01-02")
		byDate[st.Date] = st
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := days - 1; i >= 0; i-- {
		k := today.AddDate(0, 0, -i).In(timezone.Location()).Format("2006-01-02")
		st, ok := byDate[k]
		if !ok {
			st = SpinDayStat{Date: k}
		}
		out.Trend = append(out.Trend, st)
	}

	hits := map[int]int64{}
	excl := map[int]int64{}
	rows, err = db.QueryContext(ctx,
		`SELECT tier_index, COUNT(*) FROM plus_slot_spins WHERE created_at >= $1 GROUP BY tier_index`, start)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var i int
		var n int64
		if err := rows.Scan(&i, &n); err != nil {
			rows.Close()
			return nil, err
		}
		hits[i] = n
	}
	rows.Close()
	rows, err = db.QueryContext(ctx,
		`SELECT t, COUNT(*) FROM plus_slot_spins, UNNEST(excluded_tiers) AS t WHERE created_at >= $1 GROUP BY t`, start)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var i int
		var n int64
		if err := rows.Scan(&i, &n); err != nil {
			rows.Close()
			return nil, err
		}
		excl[i] = n
	}
	rows.Close()
	for i, t := range cfg.Tiers {
		out.Tiers = append(out.Tiers, TierStat{Index: i, Label: t.Label, Emoji: t.Emoji, PrizeType: t.PrizeType,
			ProbBP: t.ProbBP, Hits: hits[i], Excluded: excl[i]})
	}

	if cfg.CostPerSpin > 0 && cfg.Validate() == nil {
		small, jackpot := cfg.PotExpectation(cfg.Probs())
		out.TargetRate = (small + jackpot) / float64(cfg.CostPerSpin)
	}
	return out, nil
}
