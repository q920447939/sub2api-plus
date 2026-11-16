// Package feedback — [plus] 服务层公共部分：依赖、配置缓存、时间线写入
package feedback

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

// Deps 服务依赖；Rewarder 为 nil 表示积分模块不可用（结案时不能发积分）
type Deps struct {
	Repo     *Repository
	Accounts AccountReader
	Errors   ErrorLogReader
	Blobs    BlobStore
	Rewarder Rewarder
	Now      func() time.Time
}

// Service 反馈中心业务
type Service struct {
	Deps

	cfgMu  sync.RWMutex
	cfg    Config
	cfgVer int64
	cfgAt  time.Time
}

const cfgTTL = 10 * time.Second

func NewService(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{Deps: d}
}

// RewardEnabled 积分模块是否可用
func (s *Service) RewardEnabled() bool { return s.Rewarder != nil }

// Config 当前配置（短 TTL 缓存）
func (s *Service) Config(ctx context.Context) (Config, error) {
	s.cfgMu.RLock()
	if !s.cfgAt.IsZero() && time.Since(s.cfgAt) < cfgTTL {
		cfg := s.cfg
		s.cfgMu.RUnlock()
		return cfg, nil
	}
	s.cfgMu.RUnlock()
	cfg, ver, err := s.Repo.LoadConfig(ctx)
	if err != nil {
		return Config{}, err
	}
	s.cfgMu.Lock()
	s.cfg, s.cfgVer, s.cfgAt = cfg, ver, time.Now()
	s.cfgMu.Unlock()
	return cfg, nil
}

// AdminConfig 管理端读取配置（不走缓存，带版本号）
func (s *Service) AdminConfig(ctx context.Context) (Config, int64, error) {
	return s.Repo.LoadConfig(ctx)
}

// SaveConfig 管理端保存配置
func (s *Service) SaveConfig(ctx context.Context, cfg Config, expectVer int64) (int64, error) {
	if err := cfg.Validate(); err != nil {
		return 0, err
	}
	ver, err := s.Repo.SaveConfig(ctx, cfg, expectVer)
	if err != nil {
		return 0, err
	}
	s.cfgMu.Lock()
	s.cfgAt = time.Time{}
	s.cfgMu.Unlock()
	return ver, nil
}

// mutate 锁住一条反馈后在事务里修改；所有状态变更都走这里
func (s *Service) mutate(ctx context.Context, id int64, fn func(q *queries, tx *sql.Tx, f *Feedback) error) error {
	return s.Repo.WithTx(ctx, func(q *queries, tx *sql.Tx) error {
		f, err := q.Lock(ctx, id)
		if err != nil {
			return err
		}
		return fn(q, tx, f)
	})
}

// event 写一条系统事件
func event(ctx context.Context, q *queries, feedbackID int64, actor *int64, internal bool, ev Event) error {
	author := AuthorSystem
	if actor != nil {
		author = AuthorAdmin
	}
	return q.AddMessage(ctx, &Message{FeedbackID: feedbackID, AuthorType: author, AuthorID: actor, Kind: MsgEvent, Event: &ev, Internal: internal})
}

func runeLen(s string) int { return len([]rune(s)) }

func page(p, size int) (int, int) {
	if p < 1 {
		p = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	return p, size
}
