// Package plusconfig — [plus] 按模块隔离的运行时设置存储。
//
// 每个模块定义自己的配置类型并持有自己的 Store（按 key 分行存放在 plus_settings），
// 模块之间不共享配置结构：邮箱验证不知道发送限速，触达中心不知道验证奖励。
// 需要别的模块的某个值时，通过对方暴露的接口读取，而不是读对方的配置。
//
// 存储：plus_settings(key, version, data JSONB) + 乐观锁；读取带 10 秒缓存，保存后立即失效。
package plusconfig

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrVersionMismatch 保存时版本号不是最新（别人先改过）
var ErrVersionMismatch = errors.New("plusconfig: version mismatch")

const cacheTTL = 10 * time.Second

// Store 一个模块的配置。T 是值类型；def 给出缺省值，validate 校验并可规整（如去空白）。
type Store[T any] struct {
	db       *sql.DB
	key      string
	def      func() T
	validate func(*T) error

	mu  sync.RWMutex
	cfg T
	ver int64
	at  time.Time
}

func New[T any](db *sql.DB, key string, def func() T, validate func(*T) error) *Store[T] {
	return &Store[T]{db: db, key: key, def: def, validate: validate}
}

// Get 返回配置与版本号；读库失败时返回缺省配置并带上错误
func (s *Store[T]) Get(ctx context.Context) (T, int64, error) {
	s.mu.RLock()
	if !s.at.IsZero() && time.Since(s.at) < cacheTTL {
		cfg, ver := s.cfg, s.ver
		s.mu.RUnlock()
		return cfg, ver, nil
	}
	s.mu.RUnlock()

	cfg, ver, err := s.load(ctx)
	if err != nil {
		return s.def(), 0, err
	}
	s.mu.Lock()
	s.cfg, s.ver, s.at = cfg, ver, time.Now()
	s.mu.Unlock()
	return cfg, ver, nil
}

// Cfg 只要配置，出错用缺省值（热路径使用）
func (s *Store[T]) Cfg(ctx context.Context) T {
	cfg, _, _ := s.Get(ctx)
	return cfg
}

func (s *Store[T]) load(ctx context.Context) (T, int64, error) {
	var (
		ver  int64
		data []byte
	)
	cfg := s.def() // 缺失字段沿用缺省值
	err := s.db.QueryRowContext(ctx, `SELECT version, data FROM plus_settings WHERE key = $1`, s.key).Scan(&ver, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return cfg, 0, nil
	}
	if err != nil {
		return cfg, 0, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, 0, fmt.Errorf("plusconfig %s: decode: %w", s.key, err)
	}
	return cfg, ver, nil
}

// Save 以 expectVer 做 CAS 保存，expectVer=0 表示首次写入
func (s *Store[T]) Save(ctx context.Context, cfg T, expectVer int64) (int64, error) {
	if err := s.validate(&cfg); err != nil {
		return 0, err
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return 0, err
	}
	var newVer int64
	if expectVer == 0 {
		err = s.db.QueryRowContext(ctx, `
			INSERT INTO plus_settings (key, version, data, updated_at) VALUES ($1, 1, $2, NOW())
			ON CONFLICT (key) DO NOTHING RETURNING version`, s.key, data).Scan(&newVer)
	} else {
		err = s.db.QueryRowContext(ctx, `
			UPDATE plus_settings SET version = version + 1, data = $2, updated_at = NOW()
			WHERE key = $1 AND version = $3 RETURNING version`, s.key, data, expectVer).Scan(&newVer)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrVersionMismatch
	}
	if err != nil {
		return 0, err
	}
	s.invalidate()
	return newVer, nil
}

func (s *Store[T]) invalidate() {
	s.mu.Lock()
	s.at = time.Time{}
	s.mu.Unlock()
}
