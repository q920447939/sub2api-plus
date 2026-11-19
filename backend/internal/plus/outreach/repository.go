// Package outreach — 数据访问：活动、圈选、批次、统计（原生 SQL，与 points / feedback 保持一致）。
// 外壳 / 模板见 content.go；发送队列、点击、领取、退订见 repo_send.go。
package outreach

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotFound       = errors.New("outreach: not found")
	ErrBadState       = errors.New("outreach: invalid state for this operation")
	ErrTokenInvalid   = errors.New("outreach: token invalid")
	ErrAnotherRunning = errors.New("outreach: another batch is running")
)

// startLockKey 串行化「开始 / 恢复执行」，保证同一时间只有一个批次在执行（共用发信额度）
const startLockKey = 7203001

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

type rowScanner interface{ Scan(dest ...any) error }

// ─── 活动 ────────────────────────────────────────────────────────────────────

const campaignCols = `c.id, c.name, c.purpose, c.template_id, c.layout_id, c.subject, c.body_md, c.segment, c.benefits,
	c.landing_url, c.claim_days, c.created_by, c.created_at, c.updated_at`

func scanCampaignInto(r rowScanner, c *Campaign, extra ...any) error {
	var seg, ben []byte
	dest := []any{&c.ID, &c.Name, &c.Purpose, &c.TemplateID, &c.LayoutID, &c.Subject, &c.BodyMD, &seg, &ben,
		&c.LandingURL, &c.ClaimDays, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt}
	if err := r.Scan(append(dest, extra...)...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	_ = json.Unmarshal(seg, &c.Segment)
	c.Benefits = parseBenefits(ben)
	return nil
}

func scanCampaign(r rowScanner) (*Campaign, error) {
	var c Campaign
	if err := scanCampaignInto(r, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) CreateCampaign(ctx context.Context, in CampaignInput, adminID int64) (*Campaign, error) {
	return scanCampaign(r.db.QueryRowContext(ctx, `
		INSERT INTO plus_outreach_campaigns AS c (name, purpose, template_id, layout_id, subject, body_md, segment, benefits, landing_url, claim_days, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING `+campaignCols,
		in.Name, in.Purpose, in.TemplateID, in.LayoutID, in.Subject, in.BodyMD, marshalJSON(in.Segment), marshalBenefits(in.Benefits), in.LandingURL, in.ClaimDays, adminID))
}

func (r *Repository) GetCampaign(ctx context.Context, id int64) (*Campaign, error) {
	return scanCampaign(r.db.QueryRowContext(ctx,
		`SELECT `+campaignCols+` FROM plus_outreach_campaigns c WHERE c.id = $1 AND c.deleted_at IS NULL`, id))
}

// GetCampaignDetail 活动 + 来源模板 / 外壳名称 + 批次数
func (r *Repository) GetCampaignDetail(ctx context.Context, id int64) (*CampaignDetail, error) {
	var d CampaignDetail
	err := scanCampaignInto(r.db.QueryRowContext(ctx, `
		SELECT `+campaignCols+`, COALESCE(t.name, ''), COALESCE(l.name, ''),
		       (SELECT COUNT(*) FROM plus_outreach_batches b WHERE b.campaign_id = c.id AND b.deleted_at IS NULL)
		FROM plus_outreach_campaigns c
		LEFT JOIN plus_mail_templates t ON t.id = c.template_id
		LEFT JOIN plus_mail_layouts l ON l.id = c.layout_id
		WHERE c.id = $1 AND c.deleted_at IS NULL`, id), &d.Campaign, &d.TemplateName, &d.LayoutName, &d.BatchCount)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// UpdateCampaign 活动随时可改：已生成的批次持有自己的快照，不受影响
func (r *Repository) UpdateCampaign(ctx context.Context, id int64, in CampaignInput) (*Campaign, error) {
	return scanCampaign(r.db.QueryRowContext(ctx, `
		UPDATE plus_outreach_campaigns AS c
		SET name=$2, purpose=$3, template_id=$4, layout_id=$5, subject=$6, body_md=$7, segment=$8, benefits=$9, landing_url=$10, claim_days=$11, updated_at=NOW()
		WHERE c.id=$1 AND c.deleted_at IS NULL RETURNING `+campaignCols,
		id, in.Name, in.Purpose, in.TemplateID, in.LayoutID, in.Subject, in.BodyMD, marshalJSON(in.Segment), marshalBenefits(in.Benefits), in.LandingURL, in.ClaimDays))
}

func (r *Repository) DuplicateCampaign(ctx context.Context, id, adminID int64) (*Campaign, error) {
	return scanCampaign(r.db.QueryRowContext(ctx, `
		INSERT INTO plus_outreach_campaigns AS c (name, purpose, template_id, layout_id, subject, body_md, segment, benefits, landing_url, claim_days, created_by)
		SELECT left(name, 110) || ' (副本)', purpose, template_id, layout_id, subject, body_md, segment, benefits, landing_url, claim_days, $2
		FROM plus_outreach_campaigns WHERE id = $1 AND deleted_at IS NULL
		RETURNING `+campaignCols, id, adminID))
}

// DeleteCampaign 删除活动及其批次：有执行中 / 已暂停的批次时拒绝；
// 没发过邮件的批次直接物理删除，发过邮件的批次软删除，保留期满后由 worker 清理。
func (r *Repository) DeleteCampaign(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var locked int64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM plus_outreach_campaigns WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&locked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM plus_outreach_batches
		WHERE campaign_id=$1 AND deleted_at IS NULL AND status IN ('running','paused'))`, id).Scan(&active); err != nil {
		return err
	}
	if active {
		return badInput("还有执行中或已暂停的批次，请先取消后再删除活动")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM plus_outreach_batches b
		WHERE b.campaign_id=$1 AND b.deleted_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM plus_outreach_recipients r WHERE r.batch_id=b.id AND r.status='sent')`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE plus_outreach_batches SET deleted_at=NOW(), updated_at=NOW() WHERE campaign_id=$1 AND deleted_at IS NULL`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE plus_outreach_campaigns SET deleted_at=NOW(), updated_at=NOW() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// StatRow 一行活动统计：投递结果 + 每个指标的值
type StatRow struct {
	DeliveryStats
	Values map[string]int
}

func (st *StatRow) dest(keys []string, vals []int) []any {
	d := st.DeliveryStats.dest()
	for i := range keys {
		d = append(d, &vals[i])
	}
	return d
}

func (r *Repository) ListCampaigns(ctx context.Context, page, size int, m *MetricRegistry) ([]CampaignRow, []StatRow, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM plus_outreach_campaigns WHERE deleted_at IS NULL`).Scan(&total); err != nil {
		return nil, nil, 0, err
	}
	var q query
	cols, keys := m.selectCols(&q)
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+campaignCols+`, COALESCE(b.cnt,0), COALESCE(b.running,FALSE), s.*
		FROM plus_outreach_campaigns c
		LEFT JOIN (
		  SELECT campaign_id, COUNT(*) AS cnt, bool_or(status='running') AS running
		  FROM plus_outreach_batches WHERE deleted_at IS NULL GROUP BY campaign_id
		) b ON b.campaign_id = c.id
		LEFT JOIN LATERAL (SELECT `+cols+` `+statsFrom+` WHERE r.campaign_id = c.id) s ON TRUE
		WHERE c.deleted_at IS NULL
		ORDER BY c.id DESC LIMIT `+q.arg(size)+` OFFSET `+q.arg((page-1)*size), q.args...)
	if err != nil {
		return nil, nil, 0, err
	}
	defer rows.Close()
	out, stats := []CampaignRow{}, []StatRow{}
	for rows.Next() {
		var (
			cr CampaignRow
			st StatRow
		)
		vals := make([]int, len(keys))
		if err := scanCampaignInto(rows, &cr.Campaign, append([]any{&cr.BatchCount, &cr.Running}, st.dest(keys, vals)...)...); err != nil {
			return nil, nil, 0, err
		}
		st.Values = toMap(keys, vals)
		cr.Sent = st.Sent
		out, stats = append(out, cr), append(stats, st)
	}
	return out, stats, total, rows.Err()
}

// ─── 圈选 ────────────────────────────────────────────────────────────────────

func (r *Repository) PreviewAudience(ctx context.Context, a audience) (*AudiencePreview, error) {
	p := &AudiencePreview{Samples: []SampleUser{}}

	var q query
	target := a.target()
	cnt := func(f frag) string { return "COUNT(*) FILTER (WHERE " + q.bind(f) + ")" }
	cols := []string{
		"COUNT(*)",
		cnt(not(a.valid)),
		cnt(and(a.valid, not(a.state))),
		cnt(and(a.valid, a.state, a.unsub)),
		cnt(target),
		cnt(and(target, a.recent)),
		cnt(and(target, a.reached)),
		cnt(and(target, a.queued)),
		cnt(a.pickable(ScopeFresh)),
		cnt(a.pickable(ScopeAgain)),
	}
	sqlText := "SELECT " + strings.Join(cols, ", ") + " " + audienceFrom + " WHERE " + q.bind(a.base)
	if err := r.db.QueryRowContext(ctx, sqlText, q.args...).Scan(&p.Matched,
		&p.Excluded.Invalid, &p.Excluded.WrongState, &p.Excluded.Unsubscribed, &p.Eligible,
		&p.Notes.RecentMail, &p.Notes.Reached, &p.Notes.Queued, &p.Notes.AvailableFresh, &p.Notes.AvailableAgain); err != nil {
		return nil, err
	}

	var sq query
	rows, err := r.db.QueryContext(ctx,
		`SELECT u.id, u.email, u.username `+audienceFrom+` WHERE `+sq.bind(target)+` ORDER BY random() LIMIT 20`, sq.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s SampleUser
		if err := rows.Scan(&s.ID, &s.Email, &s.Username); err != nil {
			return nil, err
		}
		p.Samples = append(p.Samples, s)
	}
	return p, rows.Err()
}

// ─── 批次 ────────────────────────────────────────────────────────────────────

const batchCols = `b.id, b.campaign_id, c.name, b.name, b.status, b.pause_reason, b.pick_mode, b.pick_count, b.scope,
	b.purpose, b.subject, b.body_md, b.layout, b.landing_url, b.benefits, b.claim_days, b.segment, b.total, b.created_by, b.started_at, b.finished_at, b.created_at`

const batchFrom = ` FROM plus_outreach_batches b JOIN plus_outreach_campaigns c ON c.id = b.campaign_id`

const batchSelect = `SELECT ` + batchCols + batchFrom

func scanBatchInto(r rowScanner, b *Batch, extra ...any) error {
	var seg, ben, lay []byte
	dest := []any{&b.ID, &b.CampaignID, &b.CampaignName, &b.Name, &b.Status, &b.PauseReason, &b.PickMode, &b.PickCount, &b.Scope,
		&b.Purpose, &b.Subject, &b.BodyMD, &lay, &b.LandingURL, &ben, &b.ClaimDays, &seg, &b.Total, &b.CreatedBy, &b.StartedAt, &b.FinishedAt, &b.CreatedAt}
	if err := r.Scan(append(dest, extra...)...); err != nil {
		return err
	}
	_ = json.Unmarshal(seg, &b.Segment)
	_ = json.Unmarshal(lay, &b.Layout)
	b.Benefits = parseBenefits(ben)
	return nil
}

func scanBatch(r rowScanner) (*Batch, error) {
	var b Batch
	if err := scanBatchInto(r, &b); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &b, nil
}

func (r *Repository) GetBatch(ctx context.Context, id int64) (*Batch, error) {
	return scanBatch(r.db.QueryRowContext(ctx, batchSelect+` WHERE b.id = $1 AND b.deleted_at IS NULL`, id))
}

// ListBatches 批次列表只带投递结果：批次是执行单元，效果看活动
func (r *Repository) ListBatches(ctx context.Context, campaignID int64, page, size int) ([]BatchRow, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM plus_outreach_batches WHERE campaign_id=$1 AND deleted_at IS NULL`, campaignID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+batchCols+`, s.*`+batchFrom+`
		LEFT JOIN LATERAL (SELECT `+deliveryCols+` FROM plus_outreach_recipients r WHERE r.batch_id = b.id) s ON TRUE
		WHERE b.campaign_id = $1 AND b.deleted_at IS NULL
		ORDER BY b.id DESC LIMIT $2 OFFSET $3`, campaignID, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []BatchRow{}
	for rows.Next() {
		var br BatchRow
		if err := scanBatchInto(rows, &br.Batch, br.Delivery.dest()...); err != nil {
			return nil, 0, err
		}
		out = append(out, br)
	}
	return out, total, rows.Err()
}

// CreateBatch 在一个事务里：写入快照（内容 + 外壳 + 落地页 + 权益）、按选人方式冻结名单。
// 命中 0 人时回滚并返回 Matched=0 的结果（Batch 为 nil）。
func (r *Repository) CreateBatch(ctx context.Context, c *Campaign, layout LayoutConfig, a audience, req BatchRequest, adminID int64) (*BatchCreated, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	name := req.Name
	if name == "" {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) + 1 FROM plus_outreach_batches WHERE campaign_id=$1`, c.ID).Scan(&n); err != nil {
			return nil, err
		}
		name = fmt.Sprintf("批次 #%d", n)
	}
	pickCount := 0
	switch req.Mode {
	case PickRandom:
		pickCount = req.Count
	case PickUsers:
		pickCount = len(req.Emails)
	}
	var batchID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO plus_outreach_batches (campaign_id, name, pick_mode, pick_count, scope, purpose, subject, body_md, layout, landing_url, benefits, claim_days, segment, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id`,
		c.ID, name, req.Mode, pickCount, req.Scope, c.Purpose, c.Subject, c.BodyMD, marshalJSON(layout), c.LandingURL,
		marshalBenefits(c.Benefits), c.ClaimDays, marshalJSON(c.Segment), adminID).Scan(&batchID); err != nil {
		return nil, err
	}

	var q query
	campArg, batchArg, purposeArg := q.arg(c.ID), q.arg(batchID), q.arg(c.Purpose)
	where := q.bind(a.pickable(req.Scope))
	tail := ""
	switch req.Mode {
	case PickUsers:
		ph := make([]string, len(req.Emails))
		for i, e := range req.Emails {
			ph[i] = q.arg(e)
		}
		where += " AND lower(u.email) IN (" + strings.Join(ph, ",") + ")"
	case PickRandom:
		tail = " ORDER BY random() LIMIT " + q.arg(req.Count)
	}
	res, err := tx.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO plus_outreach_recipients (campaign_id, batch_id, purpose, user_id, email, token, vars)
		SELECT %s::bigint, %s::bigint, %s::varchar, u.id, u.email, replace(gen_random_uuid()::text, '-', ''),
		       jsonb_build_object('username', u.username, 'balance', round(u.balance, 2)::text,
		                          'last_active_days', GREATEST(0, EXTRACT(DAY FROM NOW() - %s)::int))
		%s WHERE %s%s`, campArg, batchArg, purposeArg, activityExpr, audienceFrom, where, tail), q.args...)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	out := &BatchCreated{Matched: int(n)}
	if req.Mode == PickUsers {
		out.Requested = len(req.Emails)
	}
	if n == 0 {
		return out, nil // 事务回滚，不留空批次
	}
	if _, err := tx.ExecContext(ctx, `UPDATE plus_outreach_batches SET total=$2 WHERE id=$1`, batchID, n); err != nil {
		return nil, err
	}
	if out.Batch, err = scanBatch(tx.QueryRowContext(ctx, batchSelect+` WHERE b.id=$1`, batchID)); err != nil {
		return nil, err
	}
	return out, tx.Commit()
}

// ActivateBatch 开始 / 恢复执行：串行化并保证同一时间只有一个执行中的批次
func (r *Repository) ActivateBatch(ctx context.Context, id int64, from ...string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, startLockKey); err != nil {
		return err
	}
	var other int64
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM plus_outreach_batches WHERE status='running' AND id <> $1 AND deleted_at IS NULL LIMIT 1`, id).Scan(&other)
	if err == nil {
		return ErrAnotherRunning
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	args, ph := inList(from, 1)
	args = append([]any{id}, args...)
	res, err := tx.ExecContext(ctx, `
		UPDATE plus_outreach_batches
		SET status='running', pause_reason='', started_at=COALESCE(started_at, NOW()), updated_at=NOW()
		WHERE id=$1 AND deleted_at IS NULL AND status IN (`+ph+`)`, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return r.missingOrBadState(ctx, tx, id)
	}
	return tx.Commit()
}

func (r *Repository) missingOrBadState(ctx context.Context, q DBTX, id int64) error {
	var one int
	if err := q.QueryRowContext(ctx, `SELECT 1 FROM plus_outreach_batches WHERE id=$1 AND deleted_at IS NULL`, id).Scan(&one); err != nil {
		return ErrNotFound
	}
	return ErrBadState
}

// SetBatchStatus 带前置状态的状态流转（CAS），用于暂停 / 完成
func (r *Repository) SetBatchStatus(ctx context.Context, id int64, to, reason string, from ...string) error {
	args, ph := inList(from, 3)
	args = append([]any{id, to, reason}, args...)
	res, err := r.db.ExecContext(ctx, `
		UPDATE plus_outreach_batches
		SET status=$2::varchar, pause_reason=$3::varchar, updated_at=NOW(),
		    finished_at = CASE WHEN $2::varchar IN ('done','cancelled') THEN NOW() ELSE finished_at END
		WHERE id=$1 AND deleted_at IS NULL AND status IN (`+ph+`)`, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return r.missingOrBadState(ctx, r.db, id)
	}
	return nil
}

// CancelBatch 取消批次，并把还没发的收件人标为跳过，让统计数字收敛
func (r *Repository) CancelBatch(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `
		UPDATE plus_outreach_batches
		SET status='cancelled', pause_reason='管理员取消，未发送的不再发送', finished_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND deleted_at IS NULL AND status IN ('pending','running','paused')`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return r.missingOrBadState(ctx, tx, id)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE plus_outreach_recipients SET status='skipped', error='批次已取消' WHERE batch_id=$1 AND status='queued'`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteBatch 删除批次：执行中 / 已暂停的不允许（先取消）；没发过邮件的物理删除；发过邮件的软删除并保留
func (r *Repository) DeleteBatch(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	if err := tx.QueryRowContext(ctx,
		`SELECT status FROM plus_outreach_batches WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if status == StatusRunning || status == StatusPaused {
		return ErrBadState
	}
	var sent bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM plus_outreach_recipients WHERE batch_id=$1 AND status='sent')`, id).Scan(&sent); err != nil {
		return err
	}
	if sent {
		_, err = tx.ExecContext(ctx, `UPDATE plus_outreach_batches SET deleted_at=NOW(), updated_at=NOW() WHERE id=$1`, id)
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM plus_outreach_batches WHERE id=$1`, id)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// PurgeDeleted 物理清理保留期已满的软删除批次（及其收件人明细）与活动。
// 带权益的批次还有未过期的领取链接（按批次自己的 claim_days）时继续保留，避免已发出的链接突然失效。
// 发送台账不清理，频控和防重复发送长期有效。
func (r *Repository) PurgeDeleted(ctx context.Context, retentionDays int) (batches, campaigns int64, err error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM plus_outreach_batches b
		WHERE b.deleted_at IS NOT NULL AND b.deleted_at < NOW() - make_interval(days => $1)
		  AND NOT (jsonb_array_length(b.benefits) > 0 AND EXISTS (SELECT 1 FROM plus_outreach_recipients r
		                  WHERE r.batch_id = b.id AND r.sent_at > NOW() - make_interval(days => b.claim_days)))`, retentionDays)
	if err != nil {
		return 0, 0, err
	}
	batches, _ = res.RowsAffected()
	res, err = r.db.ExecContext(ctx, `
		DELETE FROM plus_outreach_campaigns c
		WHERE c.deleted_at IS NOT NULL AND c.deleted_at < NOW() - make_interval(days => $1)
		  AND NOT EXISTS (SELECT 1 FROM plus_outreach_batches b WHERE b.campaign_id = c.id)`, retentionDays)
	if err != nil {
		return batches, 0, err
	}
	campaigns, _ = res.RowsAffected()
	return batches, campaigns, nil
}

// ─── 统计 / 明细 ─────────────────────────────────────────────────────────────

// CampaignStats 活动累计：投递结果 + 全部指标
func (r *Repository) CampaignStats(ctx context.Context, id int64, m *MetricRegistry) (*StatRow, error) {
	var q query
	cols, keys := m.selectCols(&q)
	var st StatRow
	vals := make([]int, len(keys))
	if err := r.db.QueryRowContext(ctx, `SELECT `+cols+` `+statsFrom+` WHERE r.campaign_id = `+q.arg(id), q.args...).
		Scan(st.dest(keys, vals)...); err != nil {
		return nil, err
	}
	st.Values = toMap(keys, vals)
	return &st, nil
}

// BatchDelivery 批次投递结果
func (r *Repository) BatchDelivery(ctx context.Context, id int64) (*DeliveryStats, error) {
	var st DeliveryStats
	if err := r.db.QueryRowContext(ctx, `SELECT `+deliveryCols+` FROM plus_outreach_recipients r WHERE r.batch_id = $1`, id).
		Scan(st.dest()...); err != nil {
		return nil, err
	}
	return &st, nil
}

func (r *Repository) ListRecipients(ctx context.Context, batchID int64, status, q string, page, size int) ([]RecipientRow, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM plus_outreach_recipients
		WHERE batch_id=$1 AND ($2='' OR status=$2) AND ($3='' OR email ILIKE '%'||$3||'%')`, batchID, status, q).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, email, status, bounced, error, sent_at, clicked_at, claimed_at, unsub_at
		FROM plus_outreach_recipients
		WHERE batch_id=$1 AND ($2='' OR status=$2) AND ($3='' OR email ILIKE '%'||$3||'%')
		ORDER BY id LIMIT $4 OFFSET $5`, batchID, status, q, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []RecipientRow{}
	for rows.Next() {
		var x RecipientRow
		if err := rows.Scan(&x.ID, &x.UserID, &x.Email, &x.Status, &x.Bounced, &x.Error, &x.SentAt, &x.ClickedAt, &x.ClaimedAt, &x.UnsubAt); err != nil {
			return nil, 0, err
		}
		out = append(out, x)
	}
	return out, total, rows.Err()
}

// RecipientPreviewData 预览单个收件人邮件所需的数据
type RecipientPreviewData struct {
	Email  string
	Token  string
	Status string
	Vars   RecipientVars
}

func (r *Repository) GetRecipientPreview(ctx context.Context, batchID, rid int64) (*RecipientPreviewData, error) {
	var (
		d    RecipientPreviewData
		vars []byte
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT email, token, status, vars FROM plus_outreach_recipients WHERE id=$1 AND batch_id=$2`, rid, batchID).Scan(&d.Email, &d.Token, &d.Status, &vars)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(vars, &d.Vars)
	return &d, nil
}

// AdminEmail 测试发送的收件地址：只发给操作者自己
func (r *Repository) AdminEmail(ctx context.Context, userID int64) (string, error) {
	var e string
	err := r.db.QueryRowContext(ctx, `SELECT email FROM users WHERE id=$1 AND deleted_at IS NULL`, userID).Scan(&e)
	return e, err
}

// ─── 工具 ────────────────────────────────────────────────────────────────────

// inList 生成 IN 列表的占位符，参数编号从 start+1 开始
func inList(vals []string, start int) (args []any, placeholders string) {
	ph := make([]string, len(vals))
	for i, v := range vals {
		args = append(args, v)
		ph[i] = fmt.Sprintf("$%d", start+i+1)
	}
	return args, strings.Join(ph, ",")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
