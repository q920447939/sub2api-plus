// Package feedback — [plus] 端口的默认实现：积分模块、用户表、错误日志、本地文件
package feedback

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/plus/points"
	"github.com/lib/pq"
)

// ─── 积分 ──────────────────────────────────────────────────────────────

// ReasonFeedbackReward 积分流水来源
const ReasonFeedbackReward = "feedback_reward"

type pointsRewarder struct{}

func (pointsRewarder) Grant(ctx context.Context, tx DBTX, userID, feedbackID, pts int64) (int64, error) {
	e, err := points.Credit(ctx, tx, userID, pts, ReasonFeedbackReward, feedbackID, fmt.Sprintf("建议 #%d 被采纳", feedbackID))
	return e.ID, err
}

// ─── 用户 ──────────────────────────────────────────────────────────────

type sqlAccounts struct{ db *sql.DB }

func (a sqlAccounts) subscriptions(ctx context.Context, userID int64) ([]string, error) {
	rows, err := a.db.QueryContext(ctx,
		`SELECT g.name FROM user_subscriptions s JOIN groups g ON g.id = s.group_id
		 WHERE s.user_id = $1 AND s.status = 'active' AND s.expires_at > NOW() AND s.deleted_at IS NULL
		 ORDER BY s.expires_at`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (a sqlAccounts) Snapshot(ctx context.Context, userID int64) (AccountSnapshot, error) {
	var s AccountSnapshot
	if err := a.db.QueryRowContext(ctx, `SELECT balance FROM users WHERE id = $1`, userID).Scan(&s.Balance); err != nil {
		return s, err
	}
	subs, err := a.subscriptions(ctx, userID)
	s.Subscriptions = subs
	return s, err
}

func (a sqlAccounts) Card(ctx context.Context, userID int64) (*UserCard, error) {
	c := &UserCard{ID: userID}
	err := a.db.QueryRowContext(ctx,
		`SELECT u.email, COALESCE(u.username, ''), u.balance, u.created_at,
		        (SELECT COUNT(*) FROM plus_feedback f WHERE f.user_id = u.id),
		        (SELECT COUNT(*) FROM plus_feedback f WHERE f.user_id = u.id AND f.status IN ('resolved','adopted')),
		        (SELECT COALESCE(SUM(reward_points), 0) FROM plus_feedback f WHERE f.user_id = u.id)
		 FROM users u WHERE u.id = $1`, userID).
		Scan(&c.Email, &c.Username, &c.Balance, &c.RegisteredAt, &c.Feedbacks, &c.Accepted, &c.RewardTotal)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.Subscriptions, err = a.subscriptions(ctx, userID)
	return c, err
}

// ─── 错误日志 ──────────────────────────────────────────────────────────

type sqlErrorLogs struct{ db *sql.DB }

func (e sqlErrorLogs) Around(ctx context.Context, userID int64, from, to time.Time, limit int) ([]ErrorLog, error) {
	rows, err := e.db.QueryContext(ctx,
		`SELECT l.id, l.created_at, COALESCE(l.status_code, 0), COALESCE(l.model, ''), COALESCE(g.name, ''),
		        COALESCE(l.platform, ''), COALESCE(l.request_path, ''), COALESCE(l.error_phase, ''),
		        LEFT(COALESCE(NULLIF(l.error_message, ''), l.upstream_error_message, ''), 300)
		 FROM ops_error_logs l LEFT JOIN groups g ON g.id = l.group_id
		 WHERE l.user_id = $1 AND l.created_at BETWEEN $2 AND $3
		 ORDER BY l.created_at DESC LIMIT $4`, userID, from, to, limit)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "42P01" { // 表不存在（运维监控未启用）
			return []ErrorLog{}, nil
		}
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []ErrorLog{}
	for rows.Next() {
		var l ErrorLog
		if err := rows.Scan(&l.ID, &l.CreatedAt, &l.StatusCode, &l.Model, &l.Group, &l.Platform, &l.Path, &l.Phase, &l.Message); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ─── 本地文件 ──────────────────────────────────────────────────────────

// localBlobs 存在数据卷 <root>/YYYY/MM/<随机名>.<ext>；path 字段只存相对路径
type localBlobs struct{ root string }

// dataRoot 与 setup.GetDataDir 一致：DATA_DIR > /app/data > ./data
func dataRoot() string {
	if d := strings.TrimSpace(os.Getenv("DATA_DIR")); d != "" {
		return d
	}
	if info, err := os.Stat("/app/data"); err == nil && info.IsDir() {
		return "/app/data"
	}
	return "data"
}

func newLocalBlobs() localBlobs {
	return localBlobs{root: filepath.Join(dataRoot(), "plus", "feedback")}
}

func (b localBlobs) Put(_ context.Context, data []byte, ext string) (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	rel := filepath.Join(time.Now().Format("2006"), time.Now().Format("01"), hex.EncodeToString(buf[:])+"."+ext)
	full := filepath.Join(b.root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, data, 0o640); err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func (b localBlobs) resolve(rel string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return "", errors.New("feedback: bad attachment path")
	}
	return filepath.Join(b.root, clean), nil
}

func (b localBlobs) Open(rel string) (io.ReadCloser, error) {
	p, err := b.resolve(rel)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

func (b localBlobs) Remove(rel string) error {
	p, err := b.resolve(rel)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
