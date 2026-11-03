// Package points — [plus] 兑换卡密送积分：上游兑换钩子即时发放 + 定时补偿
//
// 每张卡密在 plus_points_redeem_grants 里最多一行（主键 redeem_code_id），
// 钩子与补偿任务并发也只会发一次。管理员代发、在线支付履约的卡密写一条 0 积分的跳过记录。
package points

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

// 兑换来源，与上游 service.PlusRedeemSource* 取值一致
const (
	RedeemSourceUser    = "user"
	RedeemSourcePayment = "payment"
	RedeemSourceAdmin   = "admin"
)

// 不计入的系统发奖卡密前缀
var systemCodePrefixes = []string{"LOTTERY-", "SLOT-"}

// RedeemEvent 一次已提交的兑换
type RedeemEvent struct {
	UserID int64
	CodeID int64
	Code   string
	Type   string
	Value  float64
	Source string
}

// RedeemGrant 一次兑换送积分的结果
type RedeemGrant struct {
	CodeValue float64   `json:"code_value"`
	Points    int64     `json:"points"`
	CreatedAt time.Time `json:"created_at"`
}

func isSystemCode(code string) bool {
	for _, p := range systemCodePrefixes {
		if strings.HasPrefix(code, p) {
			return true
		}
	}
	return false
}

// HandleRedeem 上游兑换钩子入口：同步发放，错误返回给调用方记日志
func (s *Service) HandleRedeem(ctx context.Context, ev RedeemEvent) error {
	if ev.Type != "balance" || ev.Value <= 0 || isSystemCode(ev.Code) {
		return nil
	}
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return err
	}
	if !cfg.RedeemBonus.Enabled {
		return nil
	}
	pts := cfg.RedeemBonus.PointsFor(ev.Value)
	if ev.Source != RedeemSourceUser {
		pts = 0 // 代发与支付履约只登记，不送积分
	}
	_, err = s.repo.grantRedeemBonus(ctx, ev.CodeID, ev.UserID, ev.Source, ev.Value, pts, "hook")
	return err
}

// BackfillRedeemBonus 补偿钩子漏发的用户兑换。只扫开关打开之后、且已过 2 分钟的卡密，避开钩子正在处理的窗口。
func (s *Service) BackfillRedeemBonus(ctx context.Context) (int, error) {
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return 0, err
	}
	rb := cfg.RedeemBonus
	if !rb.Enabled || rb.EnabledAt == nil {
		return 0, nil
	}
	pending, err := s.repo.pendingRedeems(ctx, *rb.EnabledAt, time.Now().Add(-2*time.Minute), 500)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range pending {
		if isSystemCode(p.code) {
			continue
		}
		ok, err := s.repo.grantRedeemBonus(ctx, p.codeID, p.userID, RedeemSourceUser, p.value, rb.PointsFor(p.value), "backfill")
		if err != nil {
			return n, fmt.Errorf("backfill code %d: %w", p.codeID, err)
		}
		if ok {
			n++
		}
	}
	return n, nil
}

// LatestRedeemGrant 用户最近 within 内的一次兑换送积分（兑换页提示用），没有返回 nil
func (s *Service) LatestRedeemGrant(ctx context.Context, userID int64, within time.Duration) (*RedeemGrant, error) {
	var g RedeemGrant
	err := s.repo.db.QueryRowContext(ctx,
		`SELECT code_value::float8, points, created_at FROM plus_points_redeem_grants
		 WHERE user_id = $1 AND points > 0 AND created_at >= $2
		 ORDER BY created_at DESC LIMIT 1`, userID, time.Now().Add(-within)).Scan(&g.CodeValue, &g.Points, &g.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

type pendingRedeem struct {
	codeID int64
	userID int64
	code   string
	value  float64
}

func (r *Repository) pendingRedeems(ctx context.Context, since, before time.Time, limit int) ([]pendingRedeem, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT rc.id, rc.used_by, rc.code, rc.value::float8 FROM redeem_codes rc
		 WHERE rc.type = 'balance' AND rc.status = 'used' AND rc.used_by IS NOT NULL AND rc.value > 0
		   AND rc.used_at >= $1 AND rc.used_at < $2
		   AND NOT EXISTS (SELECT 1 FROM payment_orders po WHERE po.recharge_code = rc.code)
		   AND NOT EXISTS (SELECT 1 FROM plus_points_redeem_grants g WHERE g.redeem_code_id = rc.id)
		 ORDER BY rc.id LIMIT $3`, since, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []pendingRedeem
	for rows.Next() {
		var p pendingRedeem
		if err := rows.Scan(&p.codeID, &p.userID, &p.code, &p.value); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

// grantRedeemBonus 登记并发放；已登记过返回 false
func (r *Repository) grantRedeemBonus(ctx context.Context, codeID, userID int64, source string, value float64, pts int64, via string) (bool, error) {
	granted := false
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO plus_points_redeem_grants (redeem_code_id, user_id, source, code_value, points, via, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, NOW()) ON CONFLICT (redeem_code_id) DO NOTHING`,
			codeID, userID, source, value, pts, via)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil
		}
		granted = true
		if pts <= 0 {
			return nil
		}
		entry, err := Credit(ctx, tx, userID, pts, ReasonRedeemBonus, codeID,
			fmt.Sprintf("兑换 $%g 卡密赠送 %d 积分", value, pts))
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			`UPDATE plus_points_redeem_grants SET ledger_id = $1 WHERE redeem_code_id = $2`, entry.ID, codeID)
		return err
	})
	if err == nil && granted && pts > 0 && via == "backfill" {
		log.Printf("[plus.points] backfilled redeem bonus code=%d user=%d points=%d", codeID, userID, pts)
	}
	return granted, err
}
