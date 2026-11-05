// Package iq — [plus] 数据访问：自有表 plus_iq_*，只读上游 groups / accounts / api_keys / users
package iq

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

// Repository 封装本模块的数据访问
type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

var (
	_ ConfigStore  = (*Repository)(nil)
	_ TaskStore    = (*Repository)(nil)
	_ RunReader    = (*Repository)(nil)
	_ RunQueue     = (*Repository)(nil)
	_ GroupCatalog = (*Repository)(nil)
	_ KeyStore     = (*Repository)(nil)
)

// ---------- 配置 ----------

func (r *Repository) LoadConfig(ctx context.Context) (Config, int64, error) {
	var (
		ver  int64
		data []byte
	)
	err := r.db.QueryRowContext(ctx, `SELECT version, data FROM plus_iq_config WHERE id = 1`).Scan(&ver, &data)
	cfg := DefaultConfig()
	if errors.Is(err, sql.ErrNoRows) {
		return cfg, 0, nil
	}
	if err != nil {
		return Config{}, 0, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, 0, err
	}
	cfg.fillDefaults()
	return cfg, ver, nil
}

func (r *Repository) SaveConfig(ctx context.Context, cfg Config, expectVer int64) (int64, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return 0, err
	}
	var ver int64
	err = r.db.QueryRowContext(ctx,
		`UPDATE plus_iq_config SET version = version + 1, data = $1, updated_at = NOW()
		 WHERE id = 1 AND version = $2 RETURNING version`, data, expectVer).Scan(&ver)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrVersionMismatch
	}
	return ver, err
}

func (r *Repository) PreviewSecret(ctx context.Context) ([]byte, error) {
	var s string
	if err := r.db.QueryRowContext(ctx, `SELECT preview_secret FROM plus_iq_config WHERE id = 1`).Scan(&s); err != nil {
		return nil, err
	}
	if len(s) < 32 {
		return nil, errors.New("iq: preview secret too short")
	}
	return []byte(s), nil
}

// ---------- 任务 ----------

const taskCols = `id, group_id, model, key_mode, api_key_id, format, thinking, interval_minutes, retention,
	enabled, created_by, next_run_at, created_at, updated_at`

type scanner interface{ Scan(dest ...any) error }

func scanTask(s scanner) (Task, error) {
	var t Task
	err := s.Scan(&t.ID, &t.GroupID, &t.Model, &t.KeyMode, &t.APIKeyID, &t.Format, &t.Thinking, &t.IntervalMinutes,
		&t.Retention, &t.Enabled, &t.CreatedBy, &t.NextRunAt, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (r *Repository) ListTasks(ctx context.Context) ([]Task, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+taskCols+` FROM plus_iq_task ORDER BY group_id, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) GetTask(ctx context.Context, id int64) (*Task, error) {
	t, err := scanTask(r.db.QueryRowContext(ctx, `SELECT `+taskCols+` FROM plus_iq_task WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func isUniqueViolation(err error) bool {
	var pe *pq.Error
	return errors.As(err, &pe) && pe.Code == "23505"
}

func (r *Repository) CreateTask(ctx context.Context, t Task) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO plus_iq_task (group_id, model, key_mode, api_key_id, format, thinking, interval_minutes, retention, enabled, created_by, next_run_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW()) RETURNING id`,
		t.GroupID, t.Model, t.KeyMode, t.APIKeyID, t.Format, t.Thinking, t.IntervalMinutes, t.Retention, t.Enabled, t.CreatedBy).Scan(&id)
	if isUniqueViolation(err) {
		return 0, ErrDuplicateTask
	}
	return id, err
}

// UpdateTask 不改分组和创建人；间隔变化时把下次检测时间按新间隔重新计算
func (r *Repository) UpdateTask(ctx context.Context, t Task) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE plus_iq_task SET model = $2, key_mode = $3, api_key_id = $4, format = $5, thinking = $6,
		   next_run_at = CASE WHEN interval_minutes <> $7 THEN LEAST(next_run_at, NOW() + make_interval(mins => $7)) ELSE next_run_at END,
		   interval_minutes = $7, retention = $8, enabled = $9, updated_at = NOW()
		 WHERE id = $1`,
		t.ID, t.Model, t.KeyMode, t.APIKeyID, t.Format, t.Thinking, t.IntervalMinutes, t.Retention, t.Enabled)
	if isUniqueViolation(err) {
		return ErrDuplicateTask
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) DeleteTask(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM plus_iq_task WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- 检测记录：排队与执行 ----------

func (r *Repository) Enqueue(ctx context.Context, t Task, trigger Trigger, by *int64, promptVersion int) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO plus_iq_run (task_id, group_id, model, trigger, triggered_by, status, prompt_version, api_key_id)
		 SELECT $1, $2, $3, $4, $5, 'queued', $6, $7
		 WHERE NOT EXISTS (SELECT 1 FROM plus_iq_run WHERE task_id = $1 AND status IN ('queued', 'running'))
		 RETURNING id`,
		t.ID, t.GroupID, t.Model, trigger, by, promptVersion, t.APIKeyID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (r *Repository) ClaimDue(ctx context.Context, now time.Time, limit int) ([]Task, error) {
	rows, err := r.db.QueryContext(ctx,
		`UPDATE plus_iq_task t SET next_run_at = $1 + make_interval(mins => t.interval_minutes)
		 WHERE t.id IN (SELECT id FROM plus_iq_task WHERE enabled AND next_run_at <= $1
		                ORDER BY next_run_at LIMIT $2 FOR UPDATE SKIP LOCKED)
		 RETURNING `+prefixCols("t.", taskCols), now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func prefixCols(p, cols string) string {
	parts := strings.Split(cols, ",")
	for i, c := range parts {
		parts[i] = p + strings.TrimSpace(c)
	}
	return strings.Join(parts, ", ")
}

func (r *Repository) StartQueued(ctx context.Context, limit int) ([]Run, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`UPDATE plus_iq_run SET status = 'running', started_at = NOW()
		 WHERE id IN (SELECT id FROM plus_iq_run WHERE status = 'queued' ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED)
		 RETURNING `+runCols, limit)
	if err != nil {
		return nil, err
	}
	return collectRuns(rows)
}

func (r *Repository) CountRunning(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM plus_iq_run WHERE status = 'running'`).Scan(&n)
	return n, err
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *Repository) Finish(ctx context.Context, runID int64, apiKeyID *int64, out RunOutcome) error {
	var refs any
	if len(out.ExternalRefs) > 0 {
		b, _ := json.Marshal(out.ExternalRefs)
		refs = b
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE plus_iq_run SET status = $2, api_key_id = COALESCE($3, api_key_id), client_request_id = $4, finished_at = NOW(),
		   duration_ms = $5, input_tokens = $6, output_tokens = $7, html = $8, external_refs = $9, error = $10, raw_excerpt = $11
		 WHERE id = $1`,
		runID, out.Status, apiKeyID, out.ClientRequestID, out.DurationMs, out.InputTokens, out.OutputTokens,
		nullStr(out.HTML), refs, out.Error, out.RawExcerpt)
	return err
}

func (r *Repository) SaveAttribution(ctx context.Context, runID int64, hops []AccountHop) error {
	if hops == nil {
		hops = []AccountHop{}
	}
	b, err := json.Marshal(hops)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE plus_iq_run SET account_chain = $2, attributed_at = NOW() WHERE id = $1`, runID, b)
	return err
}

func (r *Repository) Prune(ctx context.Context, taskID int64, keep int) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM plus_iq_run WHERE id IN (
		   SELECT id FROM (
		     SELECT id, ROW_NUMBER() OVER (PARTITION BY status ORDER BY id DESC) AS rn
		     FROM plus_iq_run WHERE task_id = $1 AND status IN ('success', 'failed')
		   ) x WHERE rn > $2)`, taskID, keep)
	return err
}

func (r *Repository) FailStale(ctx context.Context, startedBefore time.Time, message string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE plus_iq_run SET status = 'failed', finished_at = NOW(), error = $2
		 WHERE status = 'running' AND started_at < $1`, startedBefore, message)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ---------- 检测记录：查询 ----------

const runCols = `id, task_id, group_id, model, trigger, triggered_by, status, prompt_version, api_key_id, client_request_id,
	queued_at, started_at, finished_at, duration_ms, input_tokens, output_tokens, html IS NOT NULL, external_refs,
	error, raw_excerpt, account_chain, attributed_at`

func scanRun(s scanner) (Run, error) {
	var (
		run                           Run
		by, keyID                     sql.NullInt64
		started, finished, attributed sql.NullTime
		dur, in, out                  sql.NullInt64
		refs, chain                   []byte
	)
	err := s.Scan(&run.ID, &run.TaskID, &run.GroupID, &run.Model, &run.Trigger, &by, &run.Status, &run.PromptVersion, &keyID,
		&run.ClientRequestID, &run.QueuedAt, &started, &finished, &dur, &in, &out, &run.HasHTML, &refs,
		&run.Error, &run.RawExcerpt, &chain, &attributed)
	if err != nil {
		return run, err
	}
	if by.Valid {
		run.TriggeredBy = &by.Int64
	}
	if keyID.Valid {
		run.APIKeyID = &keyID.Int64
	}
	if started.Valid {
		run.StartedAt = &started.Time
	}
	if finished.Valid {
		run.FinishedAt = &finished.Time
	}
	if attributed.Valid {
		run.AttributedAt = &attributed.Time
	}
	if dur.Valid {
		run.DurationMs = intp(int(dur.Int64))
	}
	if in.Valid {
		run.InputTokens = intp(int(in.Int64))
	}
	if out.Valid {
		run.OutputTokens = intp(int(out.Int64))
	}
	if len(refs) > 0 {
		_ = json.Unmarshal(refs, &run.ExternalRefs)
	}
	if len(chain) > 0 {
		_ = json.Unmarshal(chain, &run.AccountChain)
	}
	if run.AccountChain == nil {
		run.AccountChain = []AccountHop{}
	}
	return run, nil
}

func collectRuns(rows *sql.Rows) ([]Run, error) {
	defer func() { _ = rows.Close() }()
	out := []Run{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (r *Repository) GetRun(ctx context.Context, id int64) (*Run, error) {
	run, err := scanRun(r.db.QueryRowContext(ctx, `SELECT `+runCols+` FROM plus_iq_run WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *Repository) ListRuns(ctx context.Context, f RunFilter) ([]Run, int64, error) {
	var (
		conds []string
		args  []any
	)
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if f.GroupID > 0 {
		add("group_id = $%d", f.GroupID)
	}
	if f.TaskID > 0 {
		add("task_id = $%d", f.TaskID)
	}
	if f.Status != "" {
		add("status = $%d", string(f.Status))
	}
	if f.Trigger != "" {
		add("trigger = $%d", string(f.Trigger))
	}
	if f.AccountID > 0 {
		add("account_chain @> jsonb_build_array(jsonb_build_object('account_id', $%d::bigint))", f.AccountID)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM plus_iq_run`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if f.PageSize <= 0 || f.PageSize > 100 {
		f.PageSize = 20
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT %s FROM plus_iq_run%s ORDER BY id DESC LIMIT $%d OFFSET $%d`, runCols, where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	runs, err := collectRuns(rows)
	return runs, total, err
}

func (r *Repository) RecentRuns(ctx context.Context, taskIDs []int64, limit int, successOnly bool) (map[int64][]Run, error) {
	out := make(map[int64][]Run, len(taskIDs))
	if len(taskIDs) == 0 || limit <= 0 {
		return out, nil
	}
	statuses := []string{"success", "failed"}
	if successOnly {
		statuses = []string{"success"}
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+runCols+` FROM (
		   SELECT *, ROW_NUMBER() OVER (PARTITION BY task_id ORDER BY id DESC) AS rn
		   FROM plus_iq_run WHERE task_id = ANY($1) AND status = ANY($2)
		 ) x WHERE rn <= $3 ORDER BY task_id, id DESC`, pq.Array(taskIDs), pq.Array(statuses), limit)
	if err != nil {
		return nil, err
	}
	runs, err := collectRuns(rows)
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		out[run.TaskID] = append(out[run.TaskID], run)
	}
	return out, nil
}

func (r *Repository) RunHTML(ctx context.Context, id int64) (string, RunStatus, int64, error) {
	var (
		html    sql.NullString
		status  RunStatus
		groupID int64
	)
	err := r.db.QueryRowContext(ctx, `SELECT html, status, group_id FROM plus_iq_run WHERE id = $1`, id).Scan(&html, &status, &groupID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !html.Valid) {
		return "", "", 0, ErrNotFound
	}
	return html.String, status, groupID, err
}

func (r *Repository) Stats(ctx context.Context, since time.Time) (RunStats, error) {
	var s RunStats
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FILTER (WHERE status IN ('success','failed') AND finished_at >= $1),
		        COUNT(*) FILTER (WHERE status = 'success' AND finished_at >= $1),
		        COUNT(*) FILTER (WHERE status = 'running'),
		        COUNT(*) FILTER (WHERE status = 'queued')
		 FROM plus_iq_run`, since).Scan(&s.Finished, &s.Success, &s.Running, &s.Queued)
	return s, err
}

func (r *Repository) ActiveByTask(ctx context.Context) (map[int64]RunStatus, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT task_id, status FROM plus_iq_run WHERE status IN ('queued', 'running')`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[int64]RunStatus{}
	for rows.Next() {
		var (
			id int64
			st RunStatus
		)
		if err := rows.Scan(&id, &st); err != nil {
			return nil, err
		}
		if out[id] != RunRunning {
			out[id] = st
		}
	}
	return out, rows.Err()
}

// ---------- 上游只读 ----------

func (r *Repository) ListGroups(ctx context.Context) ([]GroupInfo, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, COALESCE(name, ''), lower(COALESCE(NULLIF(TRIM(platform), ''), 'unknown'))
		 FROM groups WHERE deleted_at IS NULL AND status = 'active' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []GroupInfo{}
	for rows.Next() {
		var g GroupInfo
		if err := rows.Scan(&g.ID, &g.Name, &g.Platform); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *Repository) AccountNames(ctx context.Context, ids []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, COALESCE(name, '') FROM accounts WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			id   int64
			name string
		)
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

func (r *Repository) AccountCounts(ctx context.Context) (map[int64]int, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT ag.group_id, COUNT(*) FROM account_groups ag JOIN accounts a ON a.id = ag.account_id
		 WHERE a.deleted_at IS NULL GROUP BY ag.group_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[int64]int{}
	for rows.Next() {
		var (
			id int64
			n  int
		)
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

const activeKeyCond = `k.deleted_at IS NULL AND k.status = 'active' AND k.group_id IS NOT NULL AND (k.expires_at IS NULL OR k.expires_at > NOW())`

func (r *Repository) ResolveKey(ctx context.Context, keyID int64) (string, int64, error) {
	var (
		key     string
		groupID int64
	)
	err := r.db.QueryRowContext(ctx, `SELECT k.key, k.group_id FROM api_keys k WHERE k.id = $1 AND `+activeKeyCond, keyID).Scan(&key, &groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrKeyUnavailable
	}
	return key, groupID, err
}

func (r *Repository) ListGroupKeys(ctx context.Context, groupID int64) ([]KeyOption, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT k.id, k.name, k.key, k.user_id, COALESCE(u.email, '')
		 FROM api_keys k LEFT JOIN users u ON u.id = k.user_id
		 WHERE k.group_id = $1 AND `+activeKeyCond+` ORDER BY k.id`, groupID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []KeyOption{}
	for rows.Next() {
		var (
			k   KeyOption
			raw string
		)
		if err := rows.Scan(&k.ID, &k.Name, &raw, &k.UserID, &k.UserEmail); err != nil {
			return nil, err
		}
		k.Masked = maskKey(raw)
		k.Active = true
		out = append(out, k)
	}
	return out, rows.Err()
}

func (r *Repository) KeyInfos(ctx context.Context, ids []int64) (map[int64]KeyOption, error) {
	out := make(map[int64]KeyOption, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT k.id, k.name, k.key, k.user_id, COALESCE(u.email, ''), (`+activeKeyCond+`)
		 FROM api_keys k LEFT JOIN users u ON u.id = k.user_id WHERE k.id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			k   KeyOption
			raw string
		)
		if err := rows.Scan(&k.ID, &k.Name, &raw, &k.UserID, &k.UserEmail, &k.Active); err != nil {
			return nil, err
		}
		k.Masked = maskKey(raw)
		out[k.ID] = k
	}
	return out, rows.Err()
}

func maskKey(k string) string {
	if len(k) <= 10 {
		return "****"
	}
	return k[:6] + "****" + k[len(k)-4:]
}
