// Package emailverify — [plus] 数据访问层
package emailverify

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

var ErrTokenNotFound = errors.New("emailverify: token not found or expired")

// Repository 封装 emailverify 模块的全部数据访问
type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func normEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// ─── 状态 ─────────────────────────────────────────────────────────────────────

// GetUserState 当前邮箱的验证 / 无效状态 + 用户级状态
func (r *Repository) GetUserState(ctx context.Context, userID int64) (UserState, error) {
	var s UserState
	err := r.db.QueryRowContext(ctx, `
		SELECT es.verified_at, es.invalid_at, st.reward_claimed_at
		FROM plus_email_status es
		LEFT JOIN plus_user_email_states st ON st.user_id = es.user_id
		WHERE es.user_id = $1`, userID).Scan(&s.VerifiedAt, &s.InvalidAt, &s.RewardClaimedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return UserState{}, nil
	}
	return s, err
}

// InsertVerification 记一条验证；该用户已验证过这个邮箱时返回 id=0
func (r *Repository) InsertVerification(ctx context.Context, tx DBTX, userID int64, email, source string, at time.Time) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO plus_email_verifications (user_id, email, source, verified_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, email) DO NOTHING
		RETURNING id`, userID, normEmail(email), source, at).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// ClearInvalid 验证成功说明邮箱能收信：清掉这个邮箱的无效标记
func (r *Repository) ClearInvalid(ctx context.Context, tx DBTX, userID int64, email string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE plus_user_email_states SET invalid_email = NULL, invalid_at = NULL, updated_at = NOW()
		WHERE user_id = $1 AND invalid_email = $2`, userID, normEmail(email))
	return err
}

// ClaimReward 抢占「每人一次」的奖励资格（CAS），返回 true 表示本次抢到
func (r *Repository) ClaimReward(ctx context.Context, tx DBTX, userID int64) (bool, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO plus_user_email_states (user_id, reward_claimed_at, updated_at) VALUES ($1, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET reward_claimed_at = NOW(), updated_at = NOW()
		WHERE plus_user_email_states.reward_claimed_at IS NULL`, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// SetRecordReward 把发放的积分记到验证记录上
func (r *Repository) SetRecordReward(ctx context.Context, tx DBTX, id, pts int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE plus_email_verifications SET reward_points = $2 WHERE id = $1`, id, pts)
	return err
}

// MarkInvalid 硬退信：把这个邮箱标为无效（换邮箱后自然失效）
func (r *Repository) MarkInvalid(ctx context.Context, userID int64, email string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO plus_user_email_states (user_id, invalid_email, invalid_at, updated_at) VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET invalid_email = EXCLUDED.invalid_email, invalid_at = NOW(), updated_at = NOW()`,
		userID, normEmail(email))
	return err
}

// ─── 管理端 ───────────────────────────────────────────────────────────────────

// GetOverviewStats 概览（排除管理员与已删除用户）
func (r *Repository) GetOverviewStats(ctx context.Context) (AdminOverviewStats, error) {
	s := AdminOverviewStats{BySource: map[string]int64{}}
	err := r.db.QueryRowContext(ctx, `
		SELECT
		  COUNT(*) FILTER (WHERE es.invalid_at IS NULL AND es.verified_at IS NOT NULL),
		  COUNT(*) FILTER (WHERE es.invalid_at IS NULL AND es.verified_at IS NULL),
		  COUNT(*) FILTER (WHERE es.invalid_at IS NOT NULL),
		  COUNT(*) FILTER (WHERE es.invalid_at IS NULL AND es.verified_at IS NULL AND u.total_recharged > 0),
		  COUNT(*) FILTER (WHERE es.invalid_at IS NULL AND es.verified_at IS NULL AND u.total_recharged = 0)
		FROM users u JOIN plus_email_status es ON es.user_id = u.id
		WHERE u.deleted_at IS NULL AND u.role <> 'admin'`).
		Scan(&s.Verified, &s.Pending, &s.Invalid, &s.PendingPaid, &s.PendingFree)
	if err != nil {
		return s, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT source, COUNT(*), COALESCE(SUM(reward_points), 0) FROM plus_email_verifications GROUP BY source`)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var src string
		var n, pts int64
		if err := rows.Scan(&src, &n, &pts); err != nil {
			return s, err
		}
		s.BySource[src] = n
		s.RewardTotal += pts
	}
	return s, rows.Err()
}

// ListRecords 验证记录，按时间倒序
func (r *Repository) ListRecords(ctx context.Context, q RecordQuery) ([]Record, int64, error) {
	where := `WHERE ($1 = '' OR v.email ILIKE '%' || $1 || '%' OR u.username ILIKE '%' || $1 || '%')
		AND ($2 = '' OR v.source = $2)`
	from := `FROM plus_email_verifications v LEFT JOIN users u ON u.id = v.user_id `
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+from+where, q.Q, q.Source).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT v.id, v.user_id, COALESCE(u.username, ''), v.email, COALESCE(LOWER(u.email) = v.email, FALSE),
		       v.source, v.reward_points, v.verified_at `+from+where+`
		ORDER BY v.verified_at DESC, v.id DESC LIMIT $3 OFFSET $4`,
		q.Q, q.Source, q.PageSize, (q.Page-1)*q.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		var x Record
		if err := rows.Scan(&x.ID, &x.UserID, &x.Username, &x.Email, &x.Current, &x.Source, &x.RewardPoints, &x.VerifiedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, x)
	}
	return out, total, rows.Err()
}

// ─── 邀请 token ───────────────────────────────────────────────────────────────

// CreateInviteToken 生成并存储一次性验证 token
func (r *Repository) CreateInviteToken(ctx context.Context, userID int64, email string, ttl time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO plus_emailverify_tokens (token, user_id, email, expires_at, created_at)
		VALUES ($1, $2, $3, $4, NOW())`, token, userID, normEmail(email), time.Now().Add(ttl))
	if err != nil {
		return "", err
	}
	return token, nil
}

// ConsumeInviteToken 原子消费 token；不存在、已消费、已过期均返回 ErrTokenNotFound
func (r *Repository) ConsumeInviteToken(ctx context.Context, token string) (userID int64, email string, err error) {
	err = r.db.QueryRowContext(ctx, `
		UPDATE plus_emailverify_tokens SET consumed_at = NOW()
		WHERE token = $1 AND consumed_at IS NULL AND expires_at > NOW()
		RETURNING user_id, email`, token).Scan(&userID, &email)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrTokenNotFound
	}
	return userID, email, err
}

// PurgeExpiredTokens 删除已过期未使用的 token
func (r *Repository) PurgeExpiredTokens(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM plus_emailverify_tokens WHERE expires_at < NOW() AND consumed_at IS NULL`)
	return err
}
