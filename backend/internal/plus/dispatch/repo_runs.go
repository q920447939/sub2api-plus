package dispatch

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

const runCols = `id, plan_id, item_id, account_id, cred_key, platform, model, trigger, triggered_by, status, outcome, summary, detail,
	error, duration_ms, input_tokens, output_tokens, queued_at, started_at, finished_at`

func scanRun(s scanner, withText bool) (Run, error) {
	var (
		r      Run
		planID sql.NullInt64
		by     sql.NullInt64
		detail []byte
		dur    sql.NullInt64
		in, ou sql.NullInt64
		st, fi sql.NullTime
	)
	dest := []any{&r.ID, &planID, &r.ItemID, &r.AccountID, &r.CredKey, &r.Platform, &r.Model, &r.Trigger, &by, &r.Status,
		&r.Outcome, &r.Summary, &detail, &r.Error, &dur, &in, &ou, &r.QueuedAt, &st, &fi}
	if withText {
		dest = append(dest, &r.Prompt, &r.Reply)
	}
	if err := s.Scan(dest...); err != nil {
		return r, err
	}
	if planID.Valid {
		r.PlanID = &planID.Int64
	}
	if by.Valid {
		r.TriggeredBy = &by.Int64
	}
	r.Detail = map[string]any{}
	_ = json.Unmarshal(detail, &r.Detail)
	r.DurationMs, r.InputTokens, r.OutputTokens = nullInt(dur), nullInt(in), nullInt(ou)
	if st.Valid {
		r.StartedAt = &st.Time
	}
	if fi.Valid {
		r.FinishedAt = &fi.Time
	}
	return r, nil
}

func nullInt(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int64)
	return &i
}

func (r *Repository) Enqueue(ctx context.Context, run Run) (int64, bool, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO plus_dispatch_run (plan_id, item_id, account_id, cred_key, platform, model, trigger, triggered_by)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8
		WHERE NOT EXISTS (SELECT 1 FROM plus_dispatch_run WHERE item_id=$2 AND cred_key=$4 AND model=$6 AND status IN ('queued','running'))
		RETURNING id`, run.PlanID, run.ItemID, run.AccountID, run.CredKey, run.Platform, run.Model, run.Trigger, run.TriggeredBy).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil, err
}

// ClaimDuePlans 抢占到期的计划并推进下次执行时间（多实例下 SKIP LOCKED 不会重复）
func (r *Repository) ClaimDuePlans(ctx context.Context, now time.Time, limit int) ([]Plan, error) {
	rows, err := r.db.QueryContext(ctx, `
		UPDATE plus_dispatch_plan p SET next_run_at = $1 + make_interval(mins => p.interval_minutes)
		WHERE p.id IN (SELECT id FROM plus_dispatch_plan WHERE enabled AND next_run_at <= $1 ORDER BY next_run_at LIMIT $2 FOR UPDATE SKIP LOCKED)
		RETURNING `+planColsP, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Plan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) StartQueued(ctx context.Context, limit int) ([]Run, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		UPDATE plus_dispatch_run SET status = 'running', started_at = NOW()
		WHERE id IN (SELECT id FROM plus_dispatch_run WHERE status = 'queued' ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED)
		RETURNING `+runCols, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Run
	for rows.Next() {
		run, err := scanRun(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (r *Repository) CountRunning(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM plus_dispatch_run WHERE status = 'running'`).Scan(&n)
	return n, err
}

func (r *Repository) FinishRun(ctx context.Context, id int64, res ExecResult, durationMs int) error {
	detail := res.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	_, err := r.db.ExecContext(ctx, `UPDATE plus_dispatch_run SET status='done', outcome=$2, summary=$3, detail=$4, prompt=$5, reply=$6,
		error=$7, duration_ms=$8, input_tokens=$9, output_tokens=$10, finished_at=NOW() WHERE id=$1`,
		id, res.Outcome, res.Summary, mustJSON(detail), res.Prompt, res.Reply, truncateRunes(res.Error, 2000), durationMs,
		res.InputTokens, res.OutputTokens)
	return err
}

func (r *Repository) FailStale(ctx context.Context, before time.Time, msg string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE plus_dispatch_run SET status='done', outcome='error', summary='请求出错', error=$2,
		finished_at=NOW() WHERE status='running' AND started_at < $1`, before, msg)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *Repository) GetRun(ctx context.Context, id int64) (*Run, error) {
	run, err := scanRun(r.db.QueryRowContext(ctx, `SELECT `+runCols+`, prompt, reply FROM plus_dispatch_run WHERE id = $1`, id), true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func runWhere(f RunFilter) (string, []any) {
	conds := []string{"status = 'done'"}
	var args []any
	add := func(c string, v any) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf(c, len(args)))
	}
	if f.ItemID > 0 {
		add("item_id = $%d", f.ItemID)
	}
	if f.PlanID > 0 {
		add("plan_id = $%d", f.PlanID)
	}
	if len(f.CredKeys) > 0 {
		add("cred_key = ANY($%d)", pq.StringArray(f.CredKeys))
	} else if f.AccountID > 0 {
		add("account_id = $%d", f.AccountID)
	}
	if f.Model != "" {
		add("model = $%d", f.Model)
	}
	if f.Outcome != "" {
		add("outcome = $%d", f.Outcome)
	}
	if !f.Since.IsZero() {
		add("queued_at >= $%d", f.Since)
	}
	if f.BeforeID > 0 {
		add("id < $%d", f.BeforeID)
	}
	return strings.Join(conds, " AND "), args
}

func (r *Repository) ListRuns(ctx context.Context, f RunFilter) ([]Run, error) {
	where, args := runWhere(f)
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 50
	}
	args = append(args, f.Limit)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`SELECT %s FROM plus_dispatch_run WHERE %s ORDER BY id DESC LIMIT $%d`,
		runCols, where, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Run
	for rows.Next() {
		run, err := scanRun(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (r *Repository) CountOutcomes(ctx context.Context, f RunFilter) (map[string]int, error) {
	f.Outcome = ""
	where, args := runWhere(f)
	rows, err := r.db.QueryContext(ctx, `SELECT outcome, COUNT(*) FROM plus_dispatch_run WHERE `+where+` GROUP BY outcome`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var (
			k string
			n int
		)
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

func (r *Repository) RecentRuns(ctx context.Context, perKey int) ([]Run, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+runCols+` FROM (
		  SELECT *, ROW_NUMBER() OVER (PARTITION BY item_id, cred_key, model ORDER BY id DESC) AS rn
		  FROM plus_dispatch_run WHERE status = 'done' AND queued_at >= NOW() - INTERVAL '30 days'
		) t WHERE rn <= $1 ORDER BY id DESC`, perKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Run
	for rows.Next() {
		run, err := scanRun(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (r *Repository) RunStats(ctx context.Context, since time.Time) ([]RunStat, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT item_id, account_id, model, outcome, COUNT(*), COALESCE(SUM(COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0)), 0)
		FROM plus_dispatch_run WHERE status = 'done' AND queued_at >= $1 GROUP BY 1, 2, 3, 4`, since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []RunStat
	for rows.Next() {
		var s RunStat
		if err := rows.Scan(&s.ItemID, &s.AccountID, &s.Model, &s.Outcome, &s.N, &s.Tokens); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) LastRunAt(ctx context.Context) (map[string]time.Time, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT item_id, cred_key, model, MAX(queued_at) FROM plus_dispatch_run
		WHERE queued_at >= NOW() - INTERVAL '8 days' GROUP BY 1, 2, 3`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]time.Time{}
	for rows.Next() {
		var (
			item  int64
			cred  string
			model string
			at    time.Time
		)
		if err := rows.Scan(&item, &cred, &model, &at); err != nil {
			return nil, err
		}
		out[runKeyOf(item, cred, model)] = at
	}
	return out, rows.Err()
}

func runKeyOf(item int64, cred, model string) string { return itoa(item) + "|" + cred + "|" + model }

func (r *Repository) PruneRuns(ctx context.Context, before time.Time, keepWorks int) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM plus_dispatch_run WHERE status = 'done' AND outcome <> $2 AND queued_at < $1`,
		before, OutcomeSaved); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM plus_dispatch_run WHERE id IN (
		SELECT id FROM (
		  SELECT id, ROW_NUMBER() OVER (PARTITION BY item_id, cred_key, model ORDER BY id DESC) AS rn
		  FROM plus_dispatch_run WHERE status = 'done' AND outcome = $1
		) t WHERE rn > $2)`, OutcomeSaved, keepWorks)
	return err
}
