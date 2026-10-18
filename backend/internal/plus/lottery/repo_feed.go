package lottery

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// FeedItem 中奖动态滚动记录。对用户端不暴露是否虚拟。
type FeedItem struct {
	DisplayName string    `json:"display_name"`
	Amount      float64   `json:"amount"`
	CreatedAt   time.Time `json:"created_at"`
	IsVirtual   bool      `json:"-"`
}

// ListFeed 最近的中奖动态，includeVirtual=false 时只返回真实记录
func (r *Repository) ListFeed(ctx context.Context, limit int, includeVirtual bool) ([]FeedItem, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT display_name, amount::float8, created_at, is_virtual FROM plus_lottery_feed
		 WHERE created_at <= NOW() AND ($2 OR NOT is_virtual)
		 ORDER BY created_at DESC, id DESC LIMIT $1`, limit, includeVirtual)
	if err != nil {
		return nil, err
	}
	return scanFeed(rows)
}

// LeaderboardItem 手气榜条目，只展示昵称与金额
type LeaderboardItem struct {
	DisplayName string  `json:"display_name"`
	Amount      float64 `json:"amount"`
}

// Leaderboard 当日手气榜：每个展示名取单次最高金额，同额先中者在前
func (r *Repository) Leaderboard(ctx context.Context, day DayRange, limit int, includeVirtual bool) ([]LeaderboardItem, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT display_name, amount FROM (
		   SELECT DISTINCT ON (display_name) display_name, amount::float8 AS amount, created_at
		   FROM plus_lottery_feed
		   WHERE created_at >= $1 AND created_at < $2 AND created_at <= NOW() AND ($4 OR NOT is_virtual)
		   ORDER BY display_name, amount DESC, created_at
		 ) t ORDER BY amount DESC, created_at LIMIT $3`,
		day.Start, day.End, limit, includeVirtual)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []LeaderboardItem{}
	for rows.Next() {
		var item LeaderboardItem
		if err := rows.Scan(&item.DisplayName, &item.Amount); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

func scanFeed(rows *sql.Rows) ([]FeedItem, error) {
	defer rows.Close()
	list := []FeedItem{}
	for rows.Next() {
		var f FeedItem
		if err := rows.Scan(&f.DisplayName, &f.Amount, &f.CreatedAt, &f.IsVirtual); err != nil {
			return nil, err
		}
		list = append(list, f)
	}
	return list, rows.Err()
}

// InsertVirtual 写入一条虚拟动态。slot 为分钟级时间戳，多实例下同一分钟只会成功一次。
func (r *Repository) InsertVirtual(ctx context.Context, slot int64, name string, amount float64, at time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO plus_lottery_feed (display_name, amount, is_virtual, virtual_slot, created_at)
		 VALUES ($1, $2, TRUE, $3, $4) ON CONFLICT (virtual_slot) DO NOTHING`,
		name, amount, slot, at)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// EmailSuffixWhitelist 读取上游"注册邮箱域名白名单"设置（只读，解析规则复用上游），未配置时返回空
func (r *Repository) EmailSuffixWhitelist(ctx context.Context) ([]string, error) {
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`,
		service.SettingKeyRegistrationEmailSuffixWhitelist).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return service.ParseRegistrationEmailSuffixWhitelist(raw), nil
}

// AtmosphereTotals 真实累计参与人数（feed 中不同展示名）与累计金额，含虚拟
func (r *Repository) AtmosphereTotals(ctx context.Context) (participants int64, awarded float64, err error) {
	err = r.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT display_name), COALESCE(SUM(amount), 0)::float8
		 FROM plus_lottery_feed WHERE created_at <= NOW()`).Scan(&participants, &awarded)
	return
}

// MaskEmail 与上游邀请返利一致的脱敏风格：a***@g***.com
func MaskEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.Index(email, "@")
	if at <= 0 || at >= len(email)-1 {
		return "***"
	}
	local, domain := email[:at], email[at+1:]
	dot := strings.LastIndex(domain, ".")
	if dot <= 0 || dot >= len(domain)-1 {
		return maskSegment(local) + "@" + maskSegment(domain)
	}
	return maskSegment(local) + "@" + maskSegment(domain[:dot]) + domain[dot:]
}

func maskSegment(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return "***"
	}
	return string(r[0]) + "***"
}
