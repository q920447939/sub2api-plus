package dispatch

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

// Repository 本模块的数据访问（自有表 plus_dispatch_*；只读上游表）
type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

var (
	_ ConfigStore  = (*Repository)(nil)
	_ SetupStore   = (*Repository)(nil)
	_ RunStore     = (*Repository)(nil)
	_ VerdictStore = (*Repository)(nil)
	_ EventStore   = (*Repository)(nil)
	_ AdjustStore  = (*Repository)(nil)
	_ AuditStore   = (*Repository)(nil)
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

// ---------- 维度 ----------

func (r *Repository) ListDimensions(ctx context.Context) ([]Dimension, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, role, gate, sort, enabled, updated_at
		FROM plus_dispatch_dimension ORDER BY sort, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dimension
	for rows.Next() {
		var (
			d    Dimension
			gate []byte
		)
		if err := rows.Scan(&d.ID, &d.Name, &d.Role, &gate, &d.Sort, &d.Enabled, &d.Updated); err != nil {
			return nil, err
		}
		d.Gate = defaultGate()
		_ = json.Unmarshal(gate, &d.Gate)
		d.Gate.normalize()
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *Repository) SaveDimension(ctx context.Context, d Dimension) (int64, error) {
	if d.ID == 0 {
		err := r.db.QueryRowContext(ctx, `INSERT INTO plus_dispatch_dimension (name, role, gate, sort, enabled)
			VALUES ($1,$2,$3,$4,$5) RETURNING id`, d.Name, d.Role, mustJSON(d.Gate), d.Sort, d.Enabled).Scan(&d.ID)
		return d.ID, err
	}
	res, err := r.db.ExecContext(ctx, `UPDATE plus_dispatch_dimension SET name=$2, role=$3, gate=$4, sort=$5,
		enabled=$6, updated_at=NOW() WHERE id=$1`, d.ID, d.Name, d.Role, mustJSON(d.Gate), d.Sort, d.Enabled)
	return d.ID, affected(res, err)
}

func (r *Repository) DeleteDimension(ctx context.Context, id int64) error {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM plus_dispatch_item WHERE dimension_id = $1`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return errors.Join(ErrInvalid, errors.New("这个维度下还有检测项，先把检测项移走或删除"))
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM plus_dispatch_dimension WHERE id = $1`, id)
	return affected(res, err)
}

// ---------- 检测项 ----------

func (r *Repository) ListItems(ctx context.Context) ([]Item, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, type, dimension_id, display_only, params, rule, platforms, enabled,
		created_at, updated_at FROM plus_dispatch_item ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var (
			it        Item
			platforms pq.StringArray
		)
		if err := rows.Scan(&it.ID, &it.Name, &it.Type, &it.DimensionID, &it.DisplayOnly, &it.Params, &it.Rule, &platforms,
			&it.Enabled, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		it.Platforms = []string(platforms)
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *Repository) SaveItem(ctx context.Context, it Item) (int64, error) {
	if it.Platforms == nil {
		it.Platforms = []string{}
	}
	if it.ID == 0 {
		err := r.db.QueryRowContext(ctx, `INSERT INTO plus_dispatch_item (name, type, dimension_id, display_only, params, rule, platforms, enabled)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, it.Name, it.Type, it.DimensionID, it.DisplayOnly, []byte(it.Params),
			[]byte(it.Rule), pq.StringArray(it.Platforms), it.Enabled).Scan(&it.ID)
		return it.ID, err
	}
	res, err := r.db.ExecContext(ctx, `UPDATE plus_dispatch_item SET name=$2, dimension_id=$3, display_only=$4, params=$5, rule=$6,
		platforms=$7, enabled=$8, updated_at=NOW() WHERE id=$1`, it.ID, it.Name, it.DimensionID, it.DisplayOnly, []byte(it.Params),
		[]byte(it.Rule), pq.StringArray(it.Platforms), it.Enabled)
	return it.ID, affected(res, err)
}

func (r *Repository) DeleteItem(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM plus_dispatch_item WHERE id = $1`, id)
	return affected(res, err)
}

// ---------- 计划 ----------

func (r *Repository) ListPlans(ctx context.Context) ([]Plan, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, item_id, platform, models, scopes, exclude_ids, interval_minutes,
		timeout_seconds, enabled, next_run_at, created_at, updated_at FROM plus_dispatch_plan ORDER BY id`)
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
		exc    pq.Int64Array
		scopes []byte
	)
	err := s.Scan(&p.ID, &p.ItemID, &p.Platform, &models, &scopes, &exc, &p.IntervalMinutes, &p.TimeoutSeconds,
		&p.Enabled, &p.NextRunAt, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	p.Models, p.ExcludeIDs = []string(models), []int64(exc)
	if err := json.Unmarshal(scopes, &p.Scopes); err != nil {
		return p, fmt.Errorf("计划 %d 的账号范围格式有误：%w", p.ID, err)
	}
	return p, nil
}

func (r *Repository) SavePlan(ctx context.Context, p Plan) (int64, error) {
	if p.ID == 0 {
		err := r.db.QueryRowContext(ctx, `INSERT INTO plus_dispatch_plan (item_id, platform, models, scopes, exclude_ids,
			interval_minutes, timeout_seconds, enabled, next_run_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW())
			RETURNING id`, p.ItemID, p.Platform, pq.StringArray(p.Models), scopesJSON(p.Scopes),
			pq.Int64Array(p.ExcludeIDs), p.IntervalMinutes, p.TimeoutSeconds, p.Enabled).Scan(&p.ID)
		return p.ID, err
	}
	res, err := r.db.ExecContext(ctx, `UPDATE plus_dispatch_plan SET models=$2, scopes=$3, exclude_ids=$4,
		interval_minutes=$5, timeout_seconds=$6, enabled=$7,
		next_run_at = LEAST(next_run_at, NOW() + make_interval(mins => $5)), updated_at=NOW() WHERE id=$1`,
		p.ID, pq.StringArray(p.Models), scopesJSON(p.Scopes), pq.Int64Array(p.ExcludeIDs),
		p.IntervalMinutes, p.TimeoutSeconds, p.Enabled)
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
