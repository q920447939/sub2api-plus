// Package channelstatus — [plus] 配置存储（单行 + 乐观锁，沿用 plus_checkin_config 的做法）
package channelstatus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

var ErrVersionMismatch = errors.New("channelstatus: config version mismatch")

// Repository 封装渠道状态模块的数据访问
type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

var (
	_ ConfigStore  = (*Repository)(nil)
	_ GroupCatalog = (*Repository)(nil)
)

// ListGroups 只读上游 groups 表（软删除与停用的分组不展示）
func (r *Repository) ListGroups(ctx context.Context) ([]GroupInfo, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, COALESCE(name, ''), lower(COALESCE(NULLIF(TRIM(platform), ''), 'unknown'))
		 FROM groups WHERE deleted_at IS NULL AND status = 'active' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []GroupInfo
	for rows.Next() {
		var g GroupInfo
		if err := rows.Scan(&g.ID, &g.Name, &g.Platform); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Load 读取配置；表中无记录时返回默认配置和版本 0
func (r *Repository) Load(ctx context.Context) (Config, int64, error) {
	var (
		ver  int64
		data []byte
	)
	err := r.db.QueryRowContext(ctx, `SELECT version, data FROM plus_channel_status_config WHERE id = 1`).Scan(&ver, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultConfig(), 0, nil
	}
	if err != nil {
		return Config{}, 0, err
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, 0, err
	}
	cfg.Rules.normalize()
	return cfg, ver, nil
}

// Save 乐观锁保存；expectVer=0 表示首次插入。版本不一致返回 ErrVersionMismatch
func (r *Repository) Save(ctx context.Context, cfg Config, expectVer int64) (int64, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return 0, err
	}
	var newVer int64
	if expectVer == 0 {
		err = r.db.QueryRowContext(ctx,
			`INSERT INTO plus_channel_status_config (id, version, data, updated_at)
			 VALUES (1, 1, $1, NOW())
			 ON CONFLICT (id) DO NOTHING
			 RETURNING version`, data).Scan(&newVer)
	} else {
		err = r.db.QueryRowContext(ctx,
			`UPDATE plus_channel_status_config
			 SET version = version + 1, data = $1, updated_at = NOW()
			 WHERE id = 1 AND version = $2
			 RETURNING version`, data, expectVer).Scan(&newVer)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrVersionMismatch
	}
	return newVer, err
}
