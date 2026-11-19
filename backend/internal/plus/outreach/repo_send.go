// Package outreach — 数据访问：发送队列、发送台账、点击 / 领取 / 退订。
package outreach

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ─── 发送队列 ────────────────────────────────────────────────────────────────

// SentCounts 最近 1 分钟 / 24 小时的发送数（全局，读台账，用于限速）
func (r *Repository) SentCounts(ctx context.Context) (perMin, perDay int, err error) {
	err = r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FILTER (WHERE sent_at > NOW() - INTERVAL '1 minute'), COUNT(*)
		FROM plus_outreach_ledger WHERE sent_at > NOW() - INTERVAL '24 hours'`).Scan(&perMin, &perDay)
	return
}

func (r *Repository) RunningBatchIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM plus_outreach_batches WHERE status='running' AND deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Leased 被租出去处理的收件人
type Leased struct {
	ID          int64
	UserID      int64
	Email       string
	Token       string
	InviteToken string
	Attempts    int
}

// Lease 原子领取最多 n 条到期的排队记录并置为 sending（SKIP LOCKED，多实例安全）
func (r *Repository) Lease(ctx context.Context, batchID int64, n int) ([]Leased, error) {
	rows, err := r.db.QueryContext(ctx, `
		UPDATE plus_outreach_recipients SET status='sending', attempts = attempts + 1
		WHERE id IN (SELECT id FROM plus_outreach_recipients
		             WHERE batch_id=$1 AND status='queued' AND next_at <= NOW()
		             ORDER BY id LIMIT $2 FOR UPDATE SKIP LOCKED)
		RETURNING id, user_id, email, token, invite_token, attempts`, batchID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Leased
	for rows.Next() {
		var l Leased
		if err := rows.Scan(&l.ID, &l.UserID, &l.Email, &l.Token, &l.InviteToken, &l.Attempts); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// RecoverStuck 进程异常退出时把 sending 的记录放回队列
func (r *Repository) RecoverStuck(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `UPDATE plus_outreach_recipients SET status='queued' WHERE status='sending'`)
	return err
}

func (r *Repository) SaveInviteToken(ctx context.Context, id int64, tok string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE plus_outreach_recipients SET invite_token=$2 WHERE id=$1`, id, tok)
	return err
}

// MarkSent 标记已发送，写入实际发出时的变量，并在同一条语句里记一行台账（直发的 campaign_id / batch_id 为空）
func (r *Repository) MarkSent(ctx context.Context, id int64, category string, vars RecipientVars) error {
	_, err := r.db.ExecContext(ctx, `
		WITH u AS (
		  UPDATE plus_outreach_recipients SET status='sent', sent_at=NOW(), error='', vars=$2::jsonb
		  WHERE id=$1 RETURNING user_id, campaign_id, batch_id, purpose, sent_at)
		INSERT INTO plus_outreach_ledger (user_id, campaign_id, batch_id, purpose, category, sent_at)
		SELECT user_id, campaign_id, batch_id, purpose, $3::varchar, sent_at FROM u`, id, marshalJSON(vars), category)
	return err
}

// CreateDirectRecipient 直发：不经活动 / 批次，直接建一条收件人记录（用于点击跟踪与台账）
func (r *Repository) CreateDirectRecipient(ctx context.Context, purpose string, userID int64, email string) (id int64, token string, err error) {
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO plus_outreach_recipients (purpose, user_id, email, token, status, attempts)
		VALUES ($1, $2, $3, replace(gen_random_uuid()::text, '-', ''), 'sending', 1) RETURNING id, token`,
		purpose, userID, email).Scan(&id, &token)
	return
}

// UserIDByEmail 测试邮件「用某个真实用户的数据渲染」时按邮箱找用户
func (r *Repository) UserIDByEmail(ctx context.Context, email string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT id FROM users WHERE lower(email)=lower($1) AND deleted_at IS NULL`, email).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

func (r *Repository) MarkFailed(ctx context.Context, id int64, bounced bool, msg string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE plus_outreach_recipients SET status='failed', bounced=$2, error=$3 WHERE id=$1`, id, bounced, truncate(msg, 480))
	return err
}

func (r *Repository) MarkSkipped(ctx context.Context, id int64, why string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE plus_outreach_recipients SET status='skipped', error=$2 WHERE id=$1`, id, why)
	return err
}

// Requeue 软失败：退回队列并延后重试；attempts 不回滚
func (r *Repository) Requeue(ctx context.Context, id int64, after time.Duration, msg string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE plus_outreach_recipients
		SET status='queued', next_at = NOW() + make_interval(secs => $2), error=$3 WHERE id=$1`, id, after.Seconds(), truncate(msg, 480))
	return err
}

// RequeueFree 限流类错误：不算一次尝试
func (r *Repository) RequeueFree(ctx context.Context, id int64, after time.Duration, msg string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE plus_outreach_recipients
		SET status='queued', attempts = GREATEST(attempts-1,0), next_at = NOW() + make_interval(secs => $2), error=$3 WHERE id=$1`,
		id, after.Seconds(), truncate(msg, 480))
	return err
}

// BounceRate 当前批次已有结论的发送数与硬退信数
func (r *Repository) BounceRate(ctx context.Context, batchID int64) (decided, bounced int, err error) {
	err = r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FILTER (WHERE status IN ('sent','failed')), COUNT(*) FILTER (WHERE bounced)
		FROM plus_outreach_recipients WHERE batch_id=$1`, batchID).Scan(&decided, &bounced)
	return
}

func (r *Repository) PendingLeft(ctx context.Context, batchID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM plus_outreach_recipients WHERE batch_id=$1 AND status IN ('queued','sending')`, batchID).Scan(&n)
	return n, err
}

// SendContext 发送前重新读取用户的最新状态，防止名单冻结之后状态变化（改邮箱、被封、已验证、已退订、已被其他批次触达）
type SendContext struct {
	Exists         bool
	Email          string
	Username       string
	Role           string
	Status         string
	Balance        float64
	LastActiveDays int
	Verified       bool
	Invalid        bool
	Unsubscribed   bool
	Recent         bool // 频控：近期已收过邮件
}

// vars 发送时的用户变量
func (sc SendContext) vars() RecipientVars {
	return RecipientVars{Username: sc.Username, Balance: fmt.Sprintf("%.2f", sc.Balance), LastActiveDays: sc.LastActiveDays}
}

func (r *Repository) LoadSendContext(ctx context.Context, userID int64, p Purpose, cfg DeliveryConfig) (SendContext, error) {
	a := buildAudience(p, Segment{}, cfg, 0)
	var q query
	unsub, recent := q.bind(a.unsub), q.bind(a.recent)
	uid := q.arg(userID)
	var sc SendContext
	err := r.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT u.email, u.username, u.role, u.status, u.balance,
		       GREATEST(0, EXTRACT(DAY FROM NOW() - %s)::int),
		       COALESCE(es.verified_at IS NOT NULL, FALSE), COALESCE(es.invalid_at IS NOT NULL, FALSE),
		       (%s), (%s)
		%s WHERE u.id = %s AND u.deleted_at IS NULL`, activityExpr, unsub, recent, audienceFrom, uid), q.args...).Scan(
		&sc.Email, &sc.Username, &sc.Role, &sc.Status, &sc.Balance, &sc.LastActiveDays, &sc.Verified, &sc.Invalid, &sc.Unsubscribed, &sc.Recent)
	if errors.Is(err, sql.ErrNoRows) {
		return sc, nil
	}
	sc.Exists = err == nil
	return sc, err
}

// ─── 点击 / 领取 / 退订 ──────────────────────────────────────────────────────

// Redirect 解析按钮去向所需信息
type Redirect struct {
	Purpose      string
	BenefitCount int
	Landing      string
	InviteToken  string
	Token        string
}

// RecordClick 记录点击并返回跳转所需信息；只对已发送的记录生效。直发记录没有批次：无权益、落地页为首页。
func (r *Repository) RecordClick(ctx context.Context, token string) (*Redirect, error) {
	var rd Redirect
	err := r.db.QueryRowContext(ctx, `
		WITH upd AS (
		  UPDATE plus_outreach_recipients
		  SET click_count = click_count + 1, clicked_at = COALESCE(clicked_at, NOW())
		  WHERE token = $1 AND status = 'sent'
		  RETURNING batch_id, purpose, invite_token, token)
		SELECT upd.purpose, COALESCE(jsonb_array_length(b.benefits), 0), COALESCE(b.landing_url, ''), upd.invite_token, upd.token
		FROM upd LEFT JOIN plus_outreach_batches b ON b.id = upd.batch_id`, token).
		Scan(&rd.Purpose, &rd.BenefitCount, &rd.Landing, &rd.InviteToken, &rd.Token)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTokenInvalid
	}
	return &rd, err
}

var (
	ErrAlreadyClaimed = errors.New("outreach: already claimed")
	ErrClaimExpired   = errors.New("outreach: claim expired")
)

// ClaimData 领取页展示信息
type ClaimData struct {
	Specs   []BenefitSpec
	Claimed bool
	Expired bool
	Next    Redirect // 领完之后去哪
}

// GetClaimData 领取有效期取批次快照里的 claim_days（活动配置，生成批次时冻结）
func (r *Repository) GetClaimData(ctx context.Context, token string, userID int64) (*ClaimData, error) {
	var (
		cd      ClaimData
		ben     []byte
		claimed sql.NullTime
		expired bool
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT b.benefits, r.claimed_at, COALESCE(r.sent_at + make_interval(days => b.claim_days) < NOW(), FALSE),
		       r.purpose, b.landing_url, r.invite_token, r.token
		FROM plus_outreach_recipients r JOIN plus_outreach_batches b ON b.id = r.batch_id
		WHERE r.token = $1 AND r.user_id = $2 AND r.status = 'sent' AND jsonb_array_length(b.benefits) > 0`, token, userID).
		Scan(&ben, &claimed, &expired, &cd.Next.Purpose, &cd.Next.Landing, &cd.Next.InviteToken, &cd.Next.Token)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTokenInvalid
	}
	if err != nil {
		return nil, err
	}
	cd.Specs = parseBenefits(ben)
	cd.Claimed = claimed.Valid
	cd.Expired = !claimed.Valid && expired
	return &cd, nil
}

// GrantFunc 在领取事务内发放该批次的全部权益，返回每项权益的描述
type GrantFunc func(tx DBTX, req GrantRequest, specs []BenefitSpec) ([]string, error)

// Claim 领取权益：标记 claimed_at 与全部权益入账在同一事务；重复领取返回 ErrAlreadyClaimed
func (r *Repository) Claim(ctx context.Context, token string, userID int64, grant GrantFunc) ([]string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		campID, batchID int64
		ben             []byte
		claimed         sql.NullTime
		expired         bool
	)
	err = tx.QueryRowContext(ctx, `
		SELECT r.campaign_id, r.batch_id, b.benefits, r.claimed_at,
		       COALESCE(r.sent_at + make_interval(days => b.claim_days) < NOW(), FALSE)
		FROM plus_outreach_recipients r JOIN plus_outreach_batches b ON b.id = r.batch_id
		WHERE r.token = $1 AND r.user_id = $2 AND r.status = 'sent' AND jsonb_array_length(b.benefits) > 0
		FOR UPDATE OF r`, token, userID).Scan(&campID, &batchID, &ben, &claimed, &expired)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTokenInvalid
	}
	if err != nil {
		return nil, err
	}
	if claimed.Valid {
		return nil, ErrAlreadyClaimed
	}
	if expired {
		return nil, ErrClaimExpired
	}
	if _, err := tx.ExecContext(ctx, `UPDATE plus_outreach_recipients SET claimed_at = NOW() WHERE token = $1`, token); err != nil {
		return nil, err
	}
	granted, err := grant(tx, GrantRequest{UserID: userID, CampaignID: campID, BatchID: batchID}, parseBenefits(ben))
	if err != nil {
		return nil, err
	}
	return granted, tx.Commit()
}

// Unsubscribe 按 token 退订该邮件的目的；canUnsub 返回 false（服务类）时不记录
func (r *Repository) Unsubscribe(ctx context.Context, token string, canUnsub func(purpose string) bool) (purpose string, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	var userID int64
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, purpose FROM plus_outreach_recipients WHERE token = $1`, token).Scan(&userID, &purpose)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrTokenInvalid
	}
	if err != nil {
		return "", err
	}
	if !canUnsub(purpose) {
		return purpose, nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO plus_outreach_unsubscribes (user_id, purpose) VALUES ($1,$2) ON CONFLICT DO NOTHING`, userID, purpose); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE plus_outreach_recipients SET unsub_at = COALESCE(unsub_at, NOW()) WHERE token = $1`, token); err != nil {
		return "", err
	}
	return purpose, tx.Commit()
}
