// service.go — [plus] 兑换刮刮乐业务层：编排配置、策略与数据访问，不拼 SQL。
package lottery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

var (
	ErrActivityDisabled   = errors.New("lottery activity is disabled")
	ErrNoChancesLeft      = errors.New("no chances left")
	ErrChanceNotFound     = errors.New("chance not found")
	ErrChanceAlreadyUsed  = errors.New("chance already used")
	ErrChanceExpired      = errors.New("chance expired")
	ErrInvalidIdempotency = errors.New("idempotency key must be 8-64 chars of [A-Za-z0-9_-]")
)

const (
	drawNotes = "刮刮乐奖励"
	syncBatch = 200
)

// CacheInvalidator 余额变动后失效上游的认证/计费缓存，由桥接层注入
type CacheInvalidator interface {
	InvalidateUserBalance(ctx context.Context, userID int64)
}

// Service 刮刮乐服务
type Service struct {
	repo        *Repository
	invalidator CacheInvalidator
	strategy    PrizeStrategy
	rng         RNG
	now         func() time.Time

	cfgMu  sync.RWMutex
	cfg    Config
	cfgVer int64
	cfgAt  time.Time
}

// cfgTTL 配置本地缓存时间。多实例下其他节点最多延迟该时长生效。
const cfgTTL = 10 * time.Second

func NewService(repo *Repository, invalidator CacheInvalidator) *Service {
	return &Service{
		repo:        repo,
		invalidator: invalidator,
		strategy:    WeightedRangeStrategy{},
		rng:         DefaultRNG,
		now:         time.Now,
	}
}

// ─── 配置 ──────────────────────────────────────────────────────────────

// Config 返回当前配置与版本（带短 TTL 缓存）
func (s *Service) Config(ctx context.Context) (Config, int64, error) {
	s.cfgMu.RLock()
	if !s.cfgAt.IsZero() && s.now().Sub(s.cfgAt) < cfgTTL {
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
	s.cfg, s.cfgVer, s.cfgAt = cfg, ver, s.now()
	s.cfgMu.Unlock()
	return cfg, ver, nil
}

// SaveConfig 管理端保存。首次打开开关时记录 EnabledAt，之前兑换的卡密不补发机会。
func (s *Service) SaveConfig(ctx context.Context, cfg Config, expectVer int64) (int64, error) {
	if err := cfg.Validate(); err != nil {
		return 0, err
	}
	current, _, err := s.repo.LoadConfig(ctx)
	if err != nil {
		return 0, err
	}
	cfg.EnabledAt = current.EnabledAt
	if cfg.Enabled && cfg.EnabledAt == nil {
		t := s.now()
		cfg.EnabledAt = &t
	}
	ver, err := s.repo.SaveConfig(ctx, cfg, expectVer)
	if err != nil {
		return 0, err
	}
	s.cfgMu.Lock()
	s.cfgAt = time.Time{} // 立即失效本地缓存
	s.cfgMu.Unlock()
	return ver, nil
}

// ─── 机会同步 ────────────────────────────────────────────────────────────

// SyncChances 将 EnabledAt 之后兑换、尚未登记的余额卡密发放机会。userID=0 表示全站。
// 幂等：卡密主键冲突即跳过，可在请求路径与后台任务中重复调用。
func (s *Service) SyncChances(ctx context.Context, userID int64) error {
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return err
	}
	if !cfg.Enabled || cfg.EnabledAt == nil {
		return nil
	}
	valid := time.Duration(cfg.ChanceValidHours) * time.Hour
	for {
		list, err := s.repo.listPendingRedeems(ctx, *cfg.EnabledAt, userID, cfg.NotesKeyword, syncBatch)
		if err != nil {
			return fmt.Errorf("list pending redeems: %w", err)
		}
		for _, p := range list {
			if err := s.repo.grantRedeem(ctx, p, cfg.ChancesForAmount(p.Value), valid); err != nil {
				return fmt.Errorf("grant redeem %d: %w", p.RedeemCodeID, err)
			}
		}
		if len(list) < syncBatch {
			return nil
		}
	}
}

// LatestRedeemGrant 兑换成功后查询本次卡密带来的刮刮卡数。先同步一次，保证刚兑换的卡密立即可见。
func (s *Service) LatestRedeemGrant(ctx context.Context, userID int64) (int, error) {
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return 0, err
	}
	if !cfg.Enabled {
		return 0, nil
	}
	if err := s.SyncChances(ctx, userID); err != nil {
		return 0, err
	}
	return s.repo.LatestRedeemGrant(ctx, userID, 5*time.Minute)
}

// ─── 用户端 ──────────────────────────────────────────────────────────────

// Overview 用户端首页数据
type Overview struct {
	Enabled        bool         `json:"enabled"`
	PromoMaxAmount float64      `json:"promo_max_amount"`
	ChanceTiers    []ChanceTier `json:"chance_tiers"`
	Remaining      int          `json:"remaining"`
	NextExpiresAt  *time.Time   `json:"next_expires_at,omitempty"` // 最早一张卡的过期时间
	ValidHours     int          `json:"chance_valid_hours"`
	Participants   int64        `json:"participants"`
	TotalAwarded   float64      `json:"total_awarded"`
	Leaderboard    bool         `json:"leaderboard_enabled"`
}

func (s *Service) Overview(ctx context.Context, userID int64) (*Overview, error) {
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	ov := &Overview{
		Enabled:        cfg.Enabled,
		PromoMaxAmount: cfg.PromoMaxAmount,
		ChanceTiers:    cfg.ChanceTiers,
		Leaderboard:    cfg.VirtualFeed.LeaderboardEnabled,
		ValidHours:     cfg.ChanceValidHours,
	}
	if !cfg.Enabled {
		return ov, nil
	}
	if err := s.SyncChances(ctx, userID); err != nil {
		return nil, err
	}
	if ov.Remaining, ov.NextExpiresAt, err = s.repo.CountUnusedChances(ctx, userID); err != nil {
		return nil, err
	}
	p, a, err := s.repo.AtmosphereTotals(ctx)
	if err != nil {
		return nil, err
	}
	ov.Participants = cfg.VirtualFeed.BaseTotalParticipants + p
	ov.TotalAwarded = roundCents(cfg.VirtualFeed.BaseTotalAwarded + a)
	return ov, nil
}

// Draw 刮开一张卡。同一 idempotencyKey 重复提交返回同一结果。
func (s *Service) Draw(ctx context.Context, userID, chanceID int64, idempotencyKey string) (*DrawResult, error) {
	if !validIdempotencyKey(idempotencyKey) {
		return nil, ErrInvalidIdempotency
	}
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, ErrActivityDisabled
	}
	if err := s.SyncChances(ctx, userID); err != nil {
		return nil, err
	}
	req := DrawRequest{UserID: userID, ChanceID: chanceID, IdempotencyKey: idempotencyKey, Day: s.Today()}
	decide := func(in DrawInput) DrawOutcome { return s.strategy.Draw(&cfg, in, s.rng) }
	res, err := s.repo.Draw(ctx, req, decide, roundCents(cfg.RiskControl.FloorAmount), cfg.RiskControl.DailyBudget, drawNotes)
	if err != nil {
		return nil, err
	}
	if !res.Replayed {
		s.invalidate(ctx, userID)
	}
	if res.Remaining, _, err = s.repo.CountUnusedChances(ctx, userID); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Service) Feed(ctx context.Context, limit int) ([]FeedItem, error) {
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	return s.repo.ListFeed(ctx, clampLimit(limit, 50), cfg.VirtualFeed.Enabled)
}

func (s *Service) Leaderboard(ctx context.Context, limit int) ([]LeaderboardItem, error) {
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.VirtualFeed.LeaderboardEnabled {
		return []LeaderboardItem{}, nil
	}
	return s.repo.Leaderboard(ctx, s.Today(), clampLimit(limit, 20), cfg.VirtualFeed.Enabled)
}

func (s *Service) MyDraws(ctx context.Context, userID int64, limit int) ([]UserDraw, error) {
	return s.repo.ListUserDraws(ctx, userID, clampLimit(limit, 100))
}

// ─── 管理端 ──────────────────────────────────────────────────────────────

func (s *Service) AdminStats(ctx context.Context, day DayRange) (*DailyStats, error) {
	return s.repo.AdminDailyStats(ctx, day)
}

func (s *Service) AdminDraws(ctx context.Context, day DayRange, userID int64, page, pageSize int) ([]AdminDrawRecord, int, error) {
	pageSize = clampLimit(pageSize, 100)
	if page < 1 {
		page = 1
	}
	return s.repo.AdminListDraws(ctx, day, userID, pageSize, (page-1)*pageSize)
}

// ─── 时间 ──────────────────────────────────────────────────────────────

// Today 系统时区下的今天
func (s *Service) Today() DayRange {
	return DayOf(s.now().In(timezone.Location()))
}

// DayOf 返回 t 所在系统时区自然日
func DayOf(t time.Time) DayRange {
	start := timezone.StartOfDay(t)
	return DayRange{Key: start.Format("2006-01-02"), Start: start, End: start.AddDate(0, 0, 1)}
}

// ParseDay 解析 YYYY-MM-DD，空串返回今天
func (s *Service) ParseDay(v string) (DayRange, error) {
	if strings.TrimSpace(v) == "" {
		return s.Today(), nil
	}
	t, err := timezone.ParseInLocation("2006-01-02", v)
	if err != nil {
		return DayRange{}, err
	}
	return DayOf(t), nil
}

// ─── 内部 ──────────────────────────────────────────────────────────────

func (s *Service) invalidate(ctx context.Context, userID int64) {
	if s.invalidator != nil {
		s.invalidator.InvalidateUserBalance(ctx, userID)
	}
}

func validIdempotencyKey(k string) bool {
	if len(k) < 8 || len(k) > 64 {
		return false
	}
	for _, c := range k {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func clampLimit(v, max int) int {
	if v <= 0 || v > max {
		return max
	}
	return v
}
