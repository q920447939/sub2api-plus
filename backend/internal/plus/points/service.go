// Package points — [plus] 积分业务编排：配置缓存、过期扫描、流水查询
package points

import (
	"context"
	"sync"
	"time"
)

// Service 积分服务
type Service struct {
	repo *Repository

	cfgMu  sync.RWMutex
	cfg    Config
	cfgVer int64
	cfgAt  time.Time

	earn earnWayReg
}

const cfgTTL = 10 * time.Second

func NewService(repo *Repository) *Service {
	s := &Service{repo: repo}
	s.RegisterEarnWay(s.redeemBonusEarnWay)
	return s
}

func (s *Service) Repo() *Repository { return s.repo }

// Config 返回当前配置（带短 TTL 缓存）
func (s *Service) Config(ctx context.Context) (Config, int64, error) {
	s.cfgMu.RLock()
	if !s.cfgAt.IsZero() && time.Since(s.cfgAt) < cfgTTL {
		cfg, ver := s.cfg, s.cfgVer
		s.cfgMu.RUnlock()
		return cfg, ver, nil
	}
	s.cfgMu.RUnlock()

	cfg, ver, err := s.repo.LoadConfig(ctx)
	if err != nil {
		return Config{}, 0, err
	}
	s.cfgMu.Lock()
	s.cfg, s.cfgVer, s.cfgAt = cfg, ver, time.Now()
	s.cfgMu.Unlock()
	return cfg, ver, nil
}

// SaveConfig 管理端保存
func (s *Service) SaveConfig(ctx context.Context, cfg Config, expectVer int64) (int64, error) {
	if err := cfg.Validate(); err != nil {
		return 0, err
	}
	// EnabledAt 由服务端维护：关→开时记为当前时间，保证只发放开启之后的兑换
	prev, _, err := s.repo.LoadConfig(ctx)
	if err != nil {
		return 0, err
	}
	switch {
	case !cfg.RedeemBonus.Enabled:
		cfg.RedeemBonus.EnabledAt = prev.RedeemBonus.EnabledAt
	case prev.RedeemBonus.Enabled && prev.RedeemBonus.EnabledAt != nil:
		cfg.RedeemBonus.EnabledAt = prev.RedeemBonus.EnabledAt
	default:
		t := time.Now()
		cfg.RedeemBonus.EnabledAt = &t
	}
	ver, err := s.repo.SaveConfig(ctx, cfg, expectVer)
	if err != nil {
		return 0, err
	}
	s.cfgMu.Lock()
	s.cfgAt = time.Time{}
	s.cfgMu.Unlock()
	return ver, nil
}

// Overview 用户端积分概览
type Overview struct {
	Balance  int64      `json:"balance"`
	ExpireAt *time.Time `json:"expire_at,omitempty"`
	WarnDays int        `json:"warn_days,omitempty"` // 距离清零剩余天数，0=不预警
}

func (s *Service) Overview(ctx context.Context, userID int64) (*Overview, error) {
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	acc, err := s.repo.Account(ctx, userID)
	if err != nil {
		return nil, err
	}
	ov := &Overview{Balance: acc.Balance}
	if acc.Balance > 0 && cfg.ExpireDays > 0 {
		expires := cfg.ExpiresAt(acc.LastActiveAt)
		ov.ExpireAt = expires
		if cfg.WarnDays > 0 {
			warnAt := cfg.WarnAt(acc.LastActiveAt)
			if warnAt != nil && time.Now().After(*warnAt) {
				days := int(time.Until(*expires).Hours()/24) + 1
				if days < 0 {
					days = 0
				}
				ov.WarnDays = days
			}
		}
	}
	return ov, nil
}

// Ledger 用户端流水分页
func (s *Service) Ledger(ctx context.Context, userID int64, page, pageSize int, reason string) ([]LedgerItem, int, error) {
	return s.repo.ListLedger(ctx, userID, page, pageSize, reason)
}

// ExpireStale 扫描并清零所有过期账户，分批处理。由 Worker 调用。
func (s *Service) ExpireStale(ctx context.Context) (ExpireResult, error) {
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return ExpireResult{}, err
	}
	if cfg.ExpireDays <= 0 {
		return ExpireResult{}, nil
	}
	threshold := time.Now().AddDate(0, 0, -cfg.ExpireDays)
	var total ExpireResult
	for {
		res, more, err := s.repo.ExpireInactive(ctx, threshold, 200)
		if err != nil {
			return total, err
		}
		total.ExpiredAccounts += res.ExpiredAccounts
		total.ExpiredPoints += res.ExpiredPoints
		if !more {
			break
		}
	}
	return total, nil
}

// ─── 管理端 ──────────────────────────────────────────────────────────────

func (s *Service) AdminGetConfig(ctx context.Context) (Config, int64, error) {
	return s.repo.LoadConfig(ctx)
}

func (s *Service) AdminSaveConfig(ctx context.Context, cfg Config, expectVer int64) (int64, error) {
	return s.SaveConfig(ctx, cfg, expectVer)
}
