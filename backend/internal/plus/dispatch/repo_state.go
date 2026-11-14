package dispatch

import (
	"time"

	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/plus/dispatch/hook"
)

// ---------- 误报标记 ----------

func (r *Repository) ListIgnores(ctx context.Context) ([]Ignore, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT cred_key, model, item_id, up_to_run, operator, created_at FROM plus_dispatch_ignore`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ignore
	for rows.Next() {
		var ig Ignore
		if err := rows.Scan(&ig.CredKey, &ig.Model, &ig.ItemID, &ig.UpToRun, &ig.Operator, &ig.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ig)
	}
	return out, rows.Err()
}

func (r *Repository) SetIgnore(ctx context.Context, ig Ignore, actor Actor) error {
	var op *int64
	if actor.ID > 0 {
		op = &actor.ID
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO plus_dispatch_ignore (cred_key, model, item_id, up_to_run, operator_id, operator)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (cred_key, model, item_id) DO UPDATE SET up_to_run = EXCLUDED.up_to_run,
		operator_id = EXCLUDED.operator_id, operator = EXCLUDED.operator, created_at = NOW()`,
		ig.CredKey, ig.Model, ig.ItemID, ig.UpToRun, op, actor.Email)
	return err
}

func (r *Repository) DeleteIgnore(ctx context.Context, credKey, model string, itemID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM plus_dispatch_ignore WHERE cred_key = $1 AND model = $2 AND item_id = $3`, credKey, model, itemID)
	return err
}

// ---------- 权重设置 ----------

func (r *Repository) ListRules(ctx context.Context) ([]Rule, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT account_id, group_id, model, share, hard, pct, record_id, created_at FROM plus_dispatch_rule
		ORDER BY created_at DESC, account_id, group_id, model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Rule
	for rows.Next() {
		var (
			x   Rule
			pct sql.NullFloat64
			rec sql.NullInt64
		)
		if err := rows.Scan(&x.AccountID, &x.GroupID, &x.Model, &x.Share, &x.Hard, &pct, &rec, &x.CreatedAt); err != nil {
			return nil, err
		}
		if pct.Valid {
			x.Pct = &pct.Float64
		}
		if rec.Valid {
			x.RecordID = &rec.Int64
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// LoadRules 给钩子用：总开关 + 规则
func (r *Repository) LoadRules(ctx context.Context) (bool, []hook.Rule, error) {
	cfg, _, err := r.LoadConfig(ctx)
	if err != nil {
		return false, nil, err
	}
	rs, err := r.ListRules(ctx)
	if err != nil {
		return false, nil, err
	}
	out := make([]hook.Rule, 0, len(rs))
	for _, x := range rs {
		out = append(out, hook.Rule{GroupID: x.GroupID, AccountID: x.AccountID, Model: x.Model, Share: x.Share, Hard: x.Hard})
	}
	return cfg.Apply, out, nil
}

func (r *Repository) ApplyRecord(ctx context.Context, rec Record, touched []ruleKey, next []Rule, undoID int64) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if rec.Groups == nil {
		rec.Groups = []RecordGroup{}
	}
	if rec.Side == nil {
		rec.Side = []RecordSide{}
	}
	if rec.Predicted == nil {
		rec.Predicted = []RecordPredict{}
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO plus_dispatch_record (account_id, model, kind, note, groups, side, predicted, operator_id, operator)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, rec.AccountID, rec.Model, rec.Kind, rec.Note, mustJSON(rec.Groups),
		mustJSON(rec.Side), mustJSON(rec.Predicted), rec.OperatorID, rec.Operator).Scan(&id); err != nil {
		return 0, err
	}
	want := map[ruleKey]Rule{}
	for _, x := range next {
		want[x.key()] = x
	}
	for _, k := range touched {
		x, ok := want[k]
		if !ok {
			if _, err := tx.ExecContext(ctx, `DELETE FROM plus_dispatch_rule WHERE account_id = $1 AND group_id = $2 AND model = $3`,
				k.Account, k.Group, k.Model); err != nil {
				return 0, err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO plus_dispatch_rule (account_id, group_id, model, share, hard, pct, record_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (account_id, group_id, model) DO UPDATE SET share = EXCLUDED.share,
			hard = EXCLUDED.hard, pct = EXCLUDED.pct, record_id = EXCLUDED.record_id, created_at = NOW()`,
			x.AccountID, x.GroupID, x.Model, x.Share, x.Hard, x.Pct, id); err != nil {
			return 0, err
		}
	}
	if undoID > 0 {
		res, err := tx.ExecContext(ctx, `UPDATE plus_dispatch_record SET undone_at = NOW() WHERE id = $1 AND undone_at IS NULL`, undoID)
		if err := affected(res, err); err != nil {
			return 0, fmt.Errorf("%w：这条调整已经恢复过了", ErrConflict)
		}
	}
	return id, tx.Commit()
}

const recordCols = `id, account_id, model, kind, note, groups, side, predicted, undone_at, operator_id, operator, created_at`

func scanRecord(s scanner) (Record, error) {
	var (
		x      Record
		g, sd  []byte
		pr     []byte
		undone sql.NullTime
		op     sql.NullInt64
	)
	if err := s.Scan(&x.ID, &x.AccountID, &x.Model, &x.Kind, &x.Note, &g, &sd, &pr, &undone, &op, &x.Operator, &x.CreatedAt); err != nil {
		return x, err
	}
	_ = json.Unmarshal(g, &x.Groups)
	_ = json.Unmarshal(sd, &x.Side)
	_ = json.Unmarshal(pr, &x.Predicted)
	if undone.Valid {
		x.UndoneAt = &undone.Time
	}
	if op.Valid {
		x.OperatorID = &op.Int64
	}
	return x, nil
}

func (r *Repository) ListRecords(ctx context.Context, accountID int64, limit int) ([]Record, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT ` + recordCols + ` FROM plus_dispatch_record`
	args := []any{limit}
	if accountID > 0 {
		q += ` WHERE account_id = $2`
		args = append(args, accountID)
	}
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY id DESC LIMIT $1`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		x, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) GetRecord(ctx context.Context, id int64) (*Record, error) {
	x, err := scanRecord(r.db.QueryRowContext(ctx, `SELECT `+recordCols+` FROM plus_dispatch_record WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &x, nil
}

// ---------- 操作日志 ----------

func (r *Repository) AddAudit(ctx context.Context, e AuditEntry) error {
	var before, after any
	if e.Before != nil {
		before = []byte(mustJSON(e.Before))
	}
	if e.After != nil {
		after = []byte(mustJSON(e.After))
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO plus_dispatch_audit (operator_id, operator, action, target_type, target_id, summary,
		before, after) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, e.OperatorID, e.Operator, e.Action, e.TargetType, e.TargetID, e.Summary, before, after)
	return err
}

func (r *Repository) PruneAudit(ctx context.Context, before time.Time) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM plus_dispatch_audit WHERE created_at < $1`, before)
	return err
}

// auditKind v1 留下来的 target_type 归到三类里
const auditKindSQL = `CASE WHEN target_type IN ('weight', 'adjustment') THEN 'weight'
	WHEN target_type IN ('detect', 'run', 'event') OR action = 'run' THEN 'detect' ELSE 'config' END`

func (r *Repository) ListAudit(ctx context.Context, f AuditFilter) ([]AuditEntry, int64, map[string]int64, error) {
	conds, args := []string{"TRUE"}, []any{}
	add := func(c string, v any) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf(c, len(args)))
	}
	if f.Keyword != "" {
		add("(summary ILIKE '%%' || $%[1]d || '%%' OR operator ILIKE '%%' || $%[1]d || '%%')", f.Keyword)
	}
	base := strings.Join(conds, " AND ")
	// 各类的条数（不受类型筛选影响，给 Tab 上的数字用）
	counts := map[string]int64{}
	crow, err := r.db.QueryContext(ctx, `SELECT `+auditKindSQL+`, COUNT(*) FROM plus_dispatch_audit WHERE `+base+` GROUP BY 1`, args...)
	if err != nil {
		return nil, 0, nil, err
	}
	for crow.Next() {
		var (
			k string
			n int64
		)
		if err := crow.Scan(&k, &n); err != nil {
			crow.Close()
			return nil, 0, nil, err
		}
		counts[k] = n
	}
	crow.Close()
	if f.TargetType != "" {
		add("("+auditKindSQL+") = $%d", f.TargetType)
	}
	where := strings.Join(conds, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM plus_dispatch_audit WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, nil, err
	}
	if f.PageSize <= 0 || f.PageSize > 200 {
		f.PageSize = 50
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`SELECT id, operator_id, operator, action, `+auditKindSQL+`, target_id, summary, before, after,
		created_at FROM plus_dispatch_audit WHERE %s ORDER BY id DESC LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, nil, err
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
			return nil, 0, nil, err
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
	return out, total, counts, rows.Err()
}

func (r *Repository) OperatorEmail(ctx context.Context, userID int64) string {
	var email string
	_ = r.db.QueryRowContext(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email)
	return email
}
