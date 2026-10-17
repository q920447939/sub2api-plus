// Package lottery — [plus] 兑换刮刮乐活动
package lottery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/lib/pq"
)

// Repository 封装刮刮乐模块的全部数据访问。按职责拆分在 repo_*.go 中。
type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

var ErrVersionMismatch = errors.New("lottery: config version mismatch")

// 上游 redeem_codes 中计入活动的类型与状态
const (
	redeemTypeBalance = "balance"
	redeemStatusUsed  = "used"
)

// ─── 配置（单行 + 乐观锁） ───────────────────────────────────────────────

// LoadConfig 读取配置；不存在时返回默认配置与 version=0
func (r *Repository) LoadConfig(ctx context.Context) (Config, int64, error) {
	var (
		ver  int64
		data []byte
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT version, data FROM plus_lottery_config WHERE id = 1`).Scan(&ver, &data)
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
	return cfg, ver, nil
}

// SaveConfig 以 expectVer 做 CAS 保存，expectVer=0 表示首次写入
func (r *Repository) SaveConfig(ctx context.Context, cfg Config, expectVer int64) (int64, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return 0, err
	}
	var newVer int64
	if expectVer == 0 {
		err = r.db.QueryRowContext(ctx,
			`INSERT INTO plus_lottery_config (id, version, data, updated_at)
			 VALUES (1, 1, $1, NOW())
			 ON CONFLICT (id) DO NOTHING
			 RETURNING version`, data).Scan(&newVer)
	} else {
		err = r.db.QueryRowContext(ctx,
			`UPDATE plus_lottery_config
			 SET version = version + 1, data = $1, updated_at = NOW()
			 WHERE id = 1 AND version = $2
			 RETURNING version`, data, expectVer).Scan(&newVer)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrVersionMismatch
	}
	return newVer, err
}

// ─── 工具 ──────────────────────────────────────────────────────────────

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

// withTx 执行事务，fn 返回错误时回滚
func (r *Repository) withTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// DayRange 表示一个按系统时区切分的自然日 [Start, End)
type DayRange struct {
	Key   string // YYYY-MM-DD，对应 plus_lottery_daily_budget.day
	Start time.Time
	End   time.Time
}
