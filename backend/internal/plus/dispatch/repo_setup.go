package dispatch

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/lib/pq"
)

// Repository 本模块的数据访问（自有表 plus_dispatch_*；只读上游表）
type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

var (
	_ ConfigStore = (*Repository)(nil)
	_ SetupStore  = (*Repository)(nil)
	_ RunStore    = (*Repository)(nil)
	_ IgnoreStore = (*Repository)(nil)
	_ RuleStore   = (*Repository)(nil)
	_ AuditStore  = (*Repository)(nil)
)

// ---------- 配置 ----------

func (r *Repository) LoadConfig(ctx context.Context) (Config, int64, error) {
	var (
		ver  int64
		data []byte
	)
	cfg := DefaultConfig()
	err := r.db.QueryRowContext(ctx, `SELECT version, data FROM plus_dispatch_config WHERE id = 1`).Scan(&ver, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return cfg, 0, nil
	}
	if err != nil {
		return cfg, 0, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), ver, nil
	}
	_ = cfg.normalize()
	return cfg, ver, nil
}

func (r *Repository) SaveConfig(ctx context.Context, cfg Config, expectVer int64) (int64, error) {
	var ver int64
	err := r.db.QueryRowContext(ctx, `UPDATE plus_dispatch_config SET version = version + 1, data = $1, updated_at = NOW()
		WHERE id = 1 AND version = $2 RETURNING version`, mustJSON(cfg), expectVer).Scan(&ver)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrVersionMismatch
	}
	return ver, err
}

// ---------- 检测项 ----------

func (r *Repository) ListItems(ctx context.Context) ([]Item, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, type, params, rule, enabled, created_at, updated_at FROM plus_dispatch_item ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Name, &it.Type, &it.Params, &it.Rule, &it.Enabled, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *Repository) SaveItem(ctx context.Context, it Item) (int64, error) {
	if it.ID == 0 {
		err := r.db.QueryRowContext(ctx, `INSERT INTO plus_dispatch_item (name, type, params, rule, enabled) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
			it.Name, it.Type, []byte(it.Params), []byte(it.Rule), it.Enabled).Scan(&it.ID)
		return it.ID, err
	}
	res, err := r.db.ExecContext(ctx, `UPDATE plus_dispatch_item SET name=$2, params=$3, rule=$4, enabled=$5, updated_at=NOW() WHERE id=$1`,
		it.ID, it.Name, []byte(it.Params), []byte(it.Rule), it.Enabled)
	return it.ID, affected(res, err)
}

func (r *Repository) DeleteItem(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM plus_dispatch_item WHERE id = $1`, id)
	return affected(res, err)
}

// ---------- 计划 ----------

const planCols = `id, item_id, platform, models, account_ids, interval_minutes, timeout_seconds, enabled, next_run_at, created_at, updated_at`
const planColsP = `p.id, p.item_id, p.platform, p.models, p.account_ids, p.interval_minutes, p.timeout_seconds, p.enabled, p.next_run_at,
	p.created_at, p.updated_at`

func (r *Repository) ListPlans(ctx context.Context) ([]Plan, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+planCols+` FROM plus_dispatch_plan ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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

type scanner interface{ Scan(dest ...any) error }

func scanPlan(s scanner) (Plan, error) {
	var (
		p      Plan
		models pq.StringArray
		accs   pq.Int64Array
	)
	err := s.Scan(&p.ID, &p.ItemID, &p.Platform, &models, &accs, &p.IntervalMinutes, &p.TimeoutSeconds,
		&p.Enabled, &p.NextRunAt, &p.CreatedAt, &p.UpdatedAt)
	p.Models, p.AccountIDs = []string(models), []int64(accs)
	if p.Models == nil {
		p.Models = []string{}
	}
	if p.AccountIDs == nil {
		p.AccountIDs = []int64{}
	}
	return p, err
}

func (r *Repository) SavePlan(ctx context.Context, p Plan) (int64, error) {
	if p.ID == 0 {
		err := r.db.QueryRowContext(ctx, `INSERT INTO plus_dispatch_plan (item_id, platform, models, account_ids, interval_minutes,
			timeout_seconds, enabled, next_run_at) VALUES ($1,$2,$3,$4,$5,$6,$7,NOW()) RETURNING id`,
			p.ItemID, p.Platform, pq.StringArray(p.Models), pq.Int64Array(p.AccountIDs), p.IntervalMinutes, p.TimeoutSeconds, p.Enabled).Scan(&p.ID)
		return p.ID, err
	}
	res, err := r.db.ExecContext(ctx, `UPDATE plus_dispatch_plan SET platform=$2, models=$3, account_ids=$4, interval_minutes=$5,
		timeout_seconds=$6, enabled=$7, next_run_at = LEAST(next_run_at, NOW() + make_interval(mins => $5)), updated_at=NOW() WHERE id=$1`,
		p.ID, p.Platform, pq.StringArray(p.Models), pq.Int64Array(p.AccountIDs), p.IntervalMinutes, p.TimeoutSeconds, p.Enabled)
	return p.ID, affected(res, err)
}

func (r *Repository) DeletePlan(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM plus_dispatch_plan WHERE id = $1`, id)
	return affected(res, err)
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
