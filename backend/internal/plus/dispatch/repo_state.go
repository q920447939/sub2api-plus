package dispatch

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/plus/dispatch/hook"
	"github.com/lib/pq"
)

// ---------- 判定 ----------

func (r *Repository) LoadVerdicts(ctx context.Context) ([]StoredVerdict, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT group_id, account_id, model, item_id, level, short, long, value, ref_value, evidence,
		since, updated_at FROM plus_dispatch_verdict`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoredVerdict
	for rows.Next() {
		var (
			v     StoredVerdict
			val   sql.NullFloat64
			ref   sql.NullFloat64
			evRaw []byte
		)
		if err := rows.Scan(&v.GroupID, &v.AccountID, &v.Model, &v.ItemID, &v.Level, &v.Short, &v.Long, &val, &ref, &evRaw,
			&v.Since, &v.UpdatedAt); err != nil {
			return nil, err
		}
		if val.Valid {
			v.Value = &val.Float64
		}
		if ref.Valid {
			v.Ref = &ref.Float64
		}
		_ = json.Unmarshal(evRaw, &v.Evidence)
		out = append(out, v)
	}
	return out, rows.Err()
}

// ReplaceVerdicts 整体替换当前判定（一次评估的结果是完整快照），并追加变化历史
func (r *Repository) ReplaceVerdicts(ctx context.Context, vs []StoredVerdict, changes []VerdictChange) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM plus_dispatch_verdict`); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, pq.CopyIn("plus_dispatch_verdict", "group_id", "account_id", "model", "item_id", "level",
		"short", "long", "value", "ref_value", "evidence", "since", "updated_at"))
	if err != nil {
		return err
	}
	for _, v := range vs {
		ev := v.Evidence
		if ev == nil {
			ev = map[string]any{}
		}
		if _, err := stmt.ExecContext(ctx, v.GroupID, v.AccountID, v.Model, v.ItemID, string(v.Level), v.Short, v.Long, v.Value, v.Ref,
			string(mustJSON(ev)), v.Since, v.UpdatedAt); err != nil {
			_ = stmt.Close()
			return err
		}
	}
	if _, err := stmt.ExecContext(ctx); err != nil {
		_ = stmt.Close()
		return err
	}
	if err := stmt.Close(); err != nil {
		return err
	}
	for _, c := range changes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO plus_dispatch_verdict_change (group_id, account_id, model, item_id, from_level,
			to_level, text) VALUES ($1,$2,$3,$4,$5,$6,$7)`, c.GroupID, c.AccountID, c.Model, c.ItemID, c.From, c.To, c.Text); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM plus_dispatch_verdict_change WHERE created_at < NOW() - INTERVAL '30 days'`); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) ListChanges(ctx context.Context, accountID int64, since time.Time, limit int) ([]VerdictChange, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, group_id, account_id, model, item_id, from_level, to_level, text, created_at
		FROM plus_dispatch_verdict_change WHERE account_id = $1 AND created_at >= $2 ORDER BY id DESC LIMIT $3`, accountID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VerdictChange
	for rows.Next() {
		var c VerdictChange
		if err := rows.Scan(&c.ID, &c.GroupID, &c.AccountID, &c.Model, &c.ItemID, &c.From, &c.To, &c.Text, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------- 待处理 ----------

const eventCols = `id, kind, dedup_key, group_id, account_id, model, dimension_id, item_id, level, state, title, evidence,
	first_at, last_at, resolved_at, handled_by, handled_at`

func scanEvent(s scanner) (Event, error) {
	var (
		e    Event
		ev   []byte
		res  sql.NullTime
		by   sql.NullInt64
		hdat sql.NullTime
	)
	err := s.Scan(&e.ID, &e.Kind, &e.DedupKey, &e.GroupID, &e.AccountID, &e.Model, &e.DimensionID, &e.ItemID, &e.Level, &e.State,
		&e.Title, &ev, &e.FirstAt, &e.LastAt, &res, &by, &hdat)
	if err != nil {
		return e, err
	}
	_ = json.Unmarshal(ev, &e.Evidence)
	if res.Valid {
		e.ResolvedAt = &res.Time
	}
	if by.Valid {
		e.HandledBy = &by.Int64
	}
	if hdat.Valid {
		e.HandledAt = &hdat.Time
	}
	return e, nil
}

func (r *Repository) queryEvents(ctx context.Context, q string, args ...any) ([]Event, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) OpenEvents(ctx context.Context) ([]Event, error) {
	return r.queryEvents(ctx, `SELECT `+eventCols+` FROM plus_dispatch_event WHERE state <> 'resolved'`)
}

// UpsertEvent 有未结束的同类记录就更新等级 / 证据 / 时间，否则新建（state = todo）
func (r *Repository) UpsertEvent(ctx context.Context, e Event) error {
	ev := e.Evidence
	if ev == nil {
		ev = []string{}
	}
	res, err := r.db.ExecContext(ctx, `UPDATE plus_dispatch_event SET level=$2, title=$3, evidence=$4, model=$5, last_at=NOW(),
		state = CASE WHEN state = 'false' AND level <> $2 AND $2 = 'bad' THEN 'todo' ELSE state END
		WHERE dedup_key=$1 AND state <> 'resolved'`, e.DedupKey, e.Level, e.Title, mustJSON(ev), e.Model)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO plus_dispatch_event (kind, dedup_key, group_id, account_id, model, dimension_id, item_id,
		level, title, evidence) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`,
		e.Kind, e.DedupKey, e.GroupID, e.AccountID, e.Model, e.DimensionID, e.ItemID, e.Level, e.Title, mustJSON(ev))
	return err
}

func (r *Repository) ResolveEvents(ctx context.Context, keys []string, at time.Time) error {
	if len(keys) == 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `UPDATE plus_dispatch_event SET state='resolved', resolved_at=$2
		WHERE dedup_key = ANY($1) AND state <> 'resolved'`, pq.StringArray(keys), at)
	return err
}

func (r *Repository) SetEventState(ctx context.Context, id int64, state string, by int64) (Event, Event, error) {
	old, err := r.GetEvent(ctx, id)
	if err != nil {
		return Event{}, Event{}, err
	}
	if old.State == EventResolved {
		return *old, *old, fmt.Errorf("%w：这条已经自动恢复，不能再改", ErrInvalid)
	}
	e, err := scanEvent(r.db.QueryRowContext(ctx, `UPDATE plus_dispatch_event SET state=$2, handled_by=$3, handled_at=NOW()
		WHERE id=$1 RETURNING `+eventCols, id, state, by))
	return *old, e, err
}

func (r *Repository) ListEvents(ctx context.Context, states []string, limit int) ([]Event, error) {
	return r.queryEvents(ctx, `SELECT `+eventCols+` FROM plus_dispatch_event WHERE state = ANY($1)
		ORDER BY CASE level WHEN 'bad' THEN 0 WHEN 'warn' THEN 1 ELSE 2 END, last_at DESC LIMIT $2`, pq.StringArray(states), limit)
}

func (r *Repository) GetEvent(ctx context.Context, id int64) (*Event, error) {
	e, err := scanEvent(r.db.QueryRowContext(ctx, `SELECT `+eventCols+` FROM plus_dispatch_event WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *Repository) LastResolved(ctx context.Context, groupID, accountID int64) (*time.Time, error) {
	var t sql.NullTime
	err := r.db.QueryRowContext(ctx, `SELECT MAX(resolved_at) FROM plus_dispatch_event WHERE kind='dim' AND group_id=$1 AND account_id=$2`,
		groupID, accountID).Scan(&t)
	if err != nil || !t.Valid {
		return nil, err
	}
	return &t.Time, nil
}

// ---------- 调整 ----------

func (r *Repository) ListAdjustments(ctx context.Context) ([]Adjustment, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT group_id, account_id, action, factor, source, reason, log_id, created_by, created_at
		FROM plus_dispatch_adjustment ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Adjustment
	for rows.Next() {
		var (
			a     Adjustment
			logID sql.NullInt64
			by    sql.NullInt64
		)
		if err := rows.Scan(&a.GroupID, &a.AccountID, &a.Action, &a.Factor, &a.Source, &a.Reason, &logID, &by, &a.CreatedAt); err != nil {
			return nil, err
		}
		if logID.Valid {
			a.LogID = &logID.Int64
		}
		if by.Valid {
			a.CreatedBy = &by.Int64
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// LoadRules 给钩子用：总开关 + 当前调整
func (r *Repository) LoadRules(ctx context.Context) (bool, []hook.Rule, error) {
	cfg, _, err := r.LoadConfig(ctx)
	if err != nil {
		return false, nil, err
	}
	adj, err := r.ListAdjustments(ctx)
	if err != nil {
		return false, nil, err
	}
	rules := make([]hook.Rule, 0, len(adj))
	for _, a := range adj {
		rules = append(rules, hook.Rule{GroupID: a.GroupID, AccountID: a.AccountID, Action: hook.Action(a.Action), Factor: a.Factor})
	}
	return cfg.Mode == ModeManual, rules, nil
}

func (r *Repository) ApplyAdjustment(ctx context.Context, a Adjustment, l AdjustmentLog) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var logID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO plus_dispatch_adjustment_log (group_id, account_id, from_action, from_factor, to_action,
		to_factor, source, reason, snapshot, operator_id, operator) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		l.GroupID, l.AccountID, l.FromAction, l.FromFactor, l.ToAction, l.ToFactor, l.Source, l.Reason, mustJSON(l.Snapshot),
		l.OperatorID, l.Operator).Scan(&logID); err != nil {
		return 0, err
	}
	if a.Action == "normal" {
		_, err = tx.ExecContext(ctx, `DELETE FROM plus_dispatch_adjustment WHERE group_id=$1 AND account_id=$2`, a.GroupID, a.AccountID)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO plus_dispatch_adjustment (group_id, account_id, action, factor, source, reason, log_id,
			created_by, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW())
			ON CONFLICT (group_id, account_id) DO UPDATE SET action=EXCLUDED.action, factor=EXCLUDED.factor, source=EXCLUDED.source,
			reason=EXCLUDED.reason, log_id=EXCLUDED.log_id, created_by=EXCLUDED.created_by, created_at=NOW()`,
			a.GroupID, a.AccountID, a.Action, a.Factor, a.Source, a.Reason, logID, a.CreatedBy)
	}
	if err != nil {
		return 0, err
	}
	return logID, tx.Commit()
}

const adjLogCols = `id, group_id, account_id, from_action, from_factor, to_action, to_factor, source, reason, snapshot, operator_id, operator, created_at`

func scanAdjLog(s scanner) (AdjustmentLog, error) {
	var (
		l    AdjustmentLog
		snap []byte
		op   sql.NullInt64
	)
	err := s.Scan(&l.ID, &l.GroupID, &l.AccountID, &l.FromAction, &l.FromFactor, &l.ToAction, &l.ToFactor, &l.Source, &l.Reason, &snap,
		&op, &l.Operator, &l.CreatedAt)
	_ = json.Unmarshal(snap, &l.Snapshot)
	if op.Valid {
		l.OperatorID = &op.Int64
	}
	return l, err
}

func (r *Repository) ListAdjustmentLogs(ctx context.Context, groupID, accountID int64, limit int) ([]AdjustmentLog, error) {
	conds, args := []string{"TRUE"}, []any{}
	if groupID > 0 {
		args = append(args, groupID)
		conds = append(conds, fmt.Sprintf("group_id = $%d", len(args)))
	}
	if accountID > 0 {
		args = append(args, accountID)
		conds = append(conds, fmt.Sprintf("account_id = $%d", len(args)))
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`SELECT %s FROM plus_dispatch_adjustment_log WHERE %s ORDER BY id DESC LIMIT $%d`,
		adjLogCols, strings.Join(conds, " AND "), len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdjustmentLog
	for rows.Next() {
		l, err := scanAdjLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *Repository) GetAdjustmentLog(ctx context.Context, id int64) (*AdjustmentLog, error) {
	l, err := scanAdjLog(r.db.QueryRowContext(ctx, `SELECT `+adjLogCols+` FROM plus_dispatch_adjustment_log WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *Repository) AddHits(ctx context.Context, bucket time.Time, hits map[hook.HitKey]int64) error {
	for k, n := range hits {
		if _, err := r.db.ExecContext(ctx, `INSERT INTO plus_dispatch_hook_hits (bucket, group_id, account_id, action, hits)
			VALUES ($1,$2,$3,$4,$5) ON CONFLICT (bucket, group_id, account_id, action) DO UPDATE SET hits = plus_dispatch_hook_hits.hits + EXCLUDED.hits`,
			bucket, k.GroupID, k.AccountID, string(k.Action), n); err != nil {
			return err
		}
	}
	return nil
}

// HookHits 分组里每个账号自 since（按小时取整）以来被钩子拿掉的次数；account 0 = 安全阀放行
func (r *Repository) HookHits(ctx context.Context, groupID int64, since time.Time) (map[int64]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT account_id, SUM(hits) FROM plus_dispatch_hook_hits WHERE group_id=$1 AND bucket >= $2
		GROUP BY account_id`, groupID, since.Truncate(time.Hour))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var (
			id int64
			n  int64
		)
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// ---------- 操作记录 ----------

func (r *Repository) AddAudit(ctx context.Context, e AuditEntry) error {
	var before, after any
	if e.Before != nil {
		before = mustJSON(e.Before)
	}
	if e.After != nil {
		after = mustJSON(e.After)
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO plus_dispatch_audit (operator_id, operator, action, target_type, target_id, summary,
		before, after) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, e.OperatorID, e.Operator, e.Action, e.TargetType, e.TargetID, e.Summary, before, after)
	return err
}

func (r *Repository) ListAudit(ctx context.Context, f AuditFilter) ([]AuditEntry, int64, error) {
	conds, args := []string{"TRUE"}, []any{}
	add := func(c string, v any) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf(c, len(args)))
	}
	if f.OperatorID > 0 {
		add("operator_id = $%d", f.OperatorID)
	}
	if f.TargetType != "" {
		add("target_type = $%d", f.TargetType)
	}
	if f.TargetID != "" {
		add("target_id = $%d", f.TargetID)
	}
	if f.Keyword != "" {
		add("(summary ILIKE '%%' || $%[1]d || '%%' OR operator ILIKE '%%' || $%[1]d || '%%')", f.Keyword)
	}
	if !f.Since.IsZero() {
		add("created_at >= $%d", f.Since)
	}
	if !f.Until.IsZero() {
		add("created_at < $%d", f.Until)
	}
	where := strings.Join(conds, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM plus_dispatch_audit WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if f.PageSize <= 0 || f.PageSize > 200 {
		f.PageSize = 50
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`SELECT id, operator_id, operator, action, target_type, target_id, summary, before, after,
		created_at FROM plus_dispatch_audit WHERE %s ORDER BY id DESC LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var (
			e     AuditEntry
			op    sql.NullInt64
			b, af []byte
		)
		if err := rows.Scan(&e.ID, &op, &e.Operator, &e.Action, &e.TargetType, &e.TargetID, &e.Summary, &b, &af, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		if op.Valid {
			e.OperatorID = &op.Int64
		}
		if len(b) > 0 {
			e.Before = json.RawMessage(b)
		}
		if len(af) > 0 {
			e.After = json.RawMessage(af)
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func (r *Repository) OperatorEmail(ctx context.Context, userID int64) string {
	var email string
	_ = r.db.QueryRowContext(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email)
	return email
}
