// Package checkin — [plus] 签到数据访问
package checkin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// Repository 封装签到模块的全部数据访问
type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

var ErrVersionMismatch = errors.New("checkin: config version mismatch")

// ─── 配置（单行 + 乐观锁） ───────────────────────────────────────────────

func (r *Repository) LoadConfig(ctx context.Context) (Config, int64, error) {
	var (
		ver  int64
		data []byte
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT version, data FROM plus_checkin_config WHERE id = 1`).Scan(&ver, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultConfig(), 0, nil
	}
	if err != nil {
		return Config{}, 0, err
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, 0, err
	}
	return cfg, ver, nil
}

func (r *Repository) SaveConfig(ctx context.Context, cfg Config, expectVer int64) (int64, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return 0, err
	}
	var newVer int64
	if expectVer == 0 {
		err = r.db.QueryRowContext(ctx,
			`INSERT INTO plus_checkin_config (id, version, data, updated_at)
			 VALUES (1, 1, $1, NOW())
			 ON CONFLICT (id) DO NOTHING
			 RETURNING version`, data).Scan(&newVer)
	} else {
		err = r.db.QueryRowContext(ctx,
			`UPDATE plus_checkin_config
			 SET version = version + 1, data = $1, updated_at = NOW()
			 WHERE id = 1 AND version = $2
			 RETURNING version`, data, expectVer).Scan(&newVer)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrVersionMismatch
	}
	return newVer, err
}

// ─── 签到 ──────────────────────────────────────────────────────────────

// CheckinLog 签到日志条目
type CheckinLog struct {
	ID            int64
	UserID        int64
	CheckedDate   time.Time
	Streak        int
	PointsGranted int64
	LedgerID      int64
	CreatedAt     time.Time
}

// TodayLog 返回用户今日签到记录，nil 表示今天未签
func (r *Repository) TodayLog(ctx context.Context, userID int64) (*CheckinLog, error) {
	today := timezone.StartOfDay(time.Now().In(timezone.Location())).Format("2006-01-02")
	var log CheckinLog
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, checked_date, streak, points_granted, ledger_id, created_at
		 FROM plus_checkin_logs WHERE user_id = $1 AND checked_date = $2`,
		userID, today).Scan(&log.ID, &log.UserID, &log.CheckedDate, &log.Streak,
		&log.PointsGranted, &log.LedgerID, &log.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &log, nil
}

// YesterdayStreak 返回昨日签到的 streak；0 表示昨日未签（连签中断）
func (r *Repository) YesterdayStreak(ctx context.Context, userID int64) (int, error) {
	yesterday := timezone.StartOfDay(time.Now().In(timezone.Location())).
		AddDate(0, 0, -1).Format("2006-01-02")
	var streak int
	err := r.db.QueryRowContext(ctx,
		`SELECT streak FROM plus_checkin_logs WHERE user_id = $1 AND checked_date = $2`,
		userID, yesterday).Scan(&streak)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return streak, err
}

// InsertLog 写入一条签到日志，关联流水 ID。UNIQUE(user_id, checked_date) 保证幂等。
func (r *Repository) InsertLog(ctx context.Context, tx *sql.Tx, userID int64, date string, streak int, points, ledgerID int64) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx,
		`INSERT INTO plus_checkin_logs (user_id, checked_date, streak, points_granted, ledger_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 RETURNING id`,
		userID, date, streak, points, ledgerID).Scan(&id)
	return id, err
}

// IPCheckinCount 统计同一 IP 在 since 之后的签到账号数（不同用户）
func (r *Repository) IPCheckinCount(ctx context.Context, ip string, since time.Time) (int, error) {
	// 签到日志本身不存 IP；通过 created_at 范围计数本 IP 近 24h 签到次数
	// 需要配合 IP 签到记录表；此处先返回 0（IP 限流用单独表，见 InsertIPLog）
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT user_id) FROM plus_checkin_ip_logs
		 WHERE ip = $1 AND created_at >= $2`,
		ip, since).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// InsertIPLog 记录本次签到的 IP
func (r *Repository) InsertIPLog(ctx context.Context, tx *sql.Tx, userID int64, ip string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO plus_checkin_ip_logs (user_id, ip, created_at) VALUES ($1, $2, NOW())`,
		userID, ip)
	return err
}

// MonthLogs 返回用户指定月份（格式 2006-01）的所有签到日期（东8区），用于前端月历渲染
func (r *Repository) MonthLogs(ctx context.Context, userID int64, year, month int) ([]time.Time, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT checked_date FROM plus_checkin_logs
		 WHERE user_id = $1
		   AND EXTRACT(YEAR  FROM checked_date) = $2
		   AND EXTRACT(MONTH FROM checked_date) = $3
		 ORDER BY checked_date`,
		userID, year, month)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var dates []time.Time
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		dates = append(dates, d)
	}
	return dates, rows.Err()
}

// BeginTx 开启事务（供 Service 使用）
func (r *Repository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, nil)
}
