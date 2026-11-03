// Package checkin — [plus] 签到业务逻辑
package checkin

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/plus/points"
	"github.com/lib/pq"
)

var (
	ErrAlreadyCheckedIn = errors.New("checkin: already checked in today")
	ErrIPLimitExceeded  = errors.New("checkin: ip daily limit exceeded")
	ErrActivityDisabled = errors.New("checkin: activity is disabled")
)

// Service 签到业务层
type Service struct {
	repo      *Repository
	pointsSvc *points.Service

	cfgMu  sync.RWMutex
	cfg    Config
	cfgVer int64
	cfgAt  time.Time
}

const cfgTTL = 10 * time.Second

func NewService(repo *Repository, pointsSvc *points.Service) *Service {
	return &Service{repo: repo, pointsSvc: pointsSvc}
}

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
	ver, err := s.repo.SaveConfig(ctx, cfg, expectVer)
	if err != nil {
		return 0, err
	}
	s.cfgMu.Lock()
	s.cfgAt = time.Time{}
	s.cfgMu.Unlock()
	return ver, nil
}

// CheckinResult 签到结果
type CheckinResult struct {
	Streak        int       `json:"streak"`         // 真实连续签到天数
	PointsGranted int64     `json:"points_granted"` // 本次获得积分
	Balance       int64     `json:"balance"`        // 签到后积分余额
	CreatedAt     time.Time `json:"created_at"`
}

// Checkin 执行签到。clientIP 用于 IP 限流（空串=不限）。
func (s *Service) Checkin(ctx context.Context, userID int64, clientIP string) (*CheckinResult, error) {
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, ErrActivityDisabled
	}

	// IP 限流（在事务外做，避免持锁时做网络/慢查询）
	if clientIP != "" && cfg.IPDailyMax > 0 {
		since := time.Now().Add(-24 * time.Hour)
		count, err := s.repo.IPCheckinCount(ctx, clientIP, since)
		if err != nil {
			return nil, fmt.Errorf("ip check: %w", err)
		}
		if count >= cfg.IPDailyMax {
			return nil, ErrIPLimitExceeded
		}
	}

	// 今日是否已签
	if log, err := s.repo.TodayLog(ctx, userID); err != nil {
		return nil, err
	} else if log != nil {
		return nil, ErrAlreadyCheckedIn
	}

	// 计算 streak：昨日有记录则 +1，否则从1开始（不再循环，真实连签天数）
	yesterday, err := s.repo.YesterdayStreak(ctx, userID)
	if err != nil {
		return nil, err
	}
	streak := 1
	if yesterday > 0 {
		streak = yesterday + 1
	}

	pts := cfg.PointsForDayN(streak)
	today := timezone.StartOfDay(time.Now().In(timezone.Location())).Format("2006-01-02")
	note := fmt.Sprintf("签到第%d天", streak)

	var result CheckinResult
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// 在事务内发放积分（Credit 接受 DBTX）
	entry, err := points.Credit(ctx, tx, userID, pts, points.ReasonCheckin, 0, note)
	if err != nil {
		return nil, fmt.Errorf("credit points: %w", err)
	}

	// 写签到日志
	logID, err := s.repo.InsertLog(ctx, tx, userID, today, streak, pts, entry.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrAlreadyCheckedIn
		}
		return nil, fmt.Errorf("insert checkin log: %w", err)
	}

	// 记录 IP
	if clientIP != "" {
		if err = s.repo.InsertIPLog(ctx, tx, userID, clientIP); err != nil {
			return nil, fmt.Errorf("insert ip log: %w", err)
		}
	}

	// 更新流水的 ref_id 为 checkin log ID（INSERT 时还不知道 log ID，事后 UPDATE）
	if _, err = tx.ExecContext(ctx,
		`UPDATE plus_points_ledger SET ref_id = $1 WHERE id = $2`,
		logID, entry.ID); err != nil {
		return nil, fmt.Errorf("update ledger ref_id: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	result = CheckinResult{
		Streak:        streak,
		PointsGranted: pts,
		Balance:       entry.Balance,
		CreatedAt:     entry.CreatedAt,
	}
	return &result, nil
}

// StatusResult 签到状态
type StatusResult struct {
	TodayChecked bool     `json:"today_checked"`
	Streak       int      `json:"streak"` // 当前连签天数（含今日）
	Balance      int64    `json:"balance"`
	MonthChecked []string `json:"month_checked"` // 已签日期列表 "2006-01-02"
	TodayPoints  int64    `json:"today_points"`  // 今日可获积分（未签时展示）
	Today        string   `json:"today"`         // 服务端东八区日期，前端日历以此为准
	Month        string   `json:"month"`         // month_checked 所属月份 "2006-01"
	Rewards      [7]int64 `json:"rewards"`       // 7 天奖励循环
	WeekBonus    int64    `json:"week_bonus"`    // 第 7 天额外奖励
	Enabled      bool     `json:"enabled"`
	PopupEnabled bool     `json:"popup_enabled"` // 登录后未签到弹窗提醒
}

// Status 查询签到状态 + 指定月份日历；month 为 "2006-01"，空或非法时取当月
func (s *Service) Status(ctx context.Context, userID int64, month string) (*StatusResult, error) {
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().In(timezone.Location())
	calMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, timezone.Location())
	if m, perr := time.ParseInLocation("2006-01", month, timezone.Location()); perr == nil && !m.After(calMonth) {
		calMonth = m
	}

	todayLog, err := s.repo.TodayLog(ctx, userID)
	if err != nil {
		return nil, err
	}

	streak := 0
	if todayLog != nil {
		streak = todayLog.Streak
	} else {
		// 今日未签，连签天数看昨日
		streak, err = s.repo.YesterdayStreak(ctx, userID)
		if err != nil {
			return nil, err
		}
	}

	// 今日可获积分（未签时展示）
	todayPts := int64(0)
	if todayLog != nil {
		todayPts = todayLog.PointsGranted
	} else {
		todayPts = cfg.PointsForDayN(streak + 1)
	}

	// 本月已签日期
	dates, err := s.repo.MonthLogs(ctx, userID, calMonth.Year(), int(calMonth.Month()))
	if err != nil {
		return nil, err
	}
	checked := make([]string, 0, len(dates))
	for _, d := range dates {
		checked = append(checked, d.Format("2006-01-02"))
	}

	// 积分余额
	pOv, err := s.pointsSvc.Overview(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &StatusResult{
		TodayChecked: todayLog != nil,
		Streak:       streak,
		Balance:      pOv.Balance,
		MonthChecked: checked,
		TodayPoints:  todayPts,
		Today:        now.Format("2006-01-02"),
		Month:        calMonth.Format("2006-01"),
		Rewards:      cfg.Rewards,
		WeekBonus:    cfg.WeekBonus,
		Enabled:      cfg.Enabled,
		PopupEnabled: cfg.Enabled && cfg.PopupEnabled,
	}, nil
}

// ─── 工具 ──────────────────────────────────────────────────────────────

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

// StreakBoostConfig 供幸运九宫格读取连签加成；签到整体关闭时加成也不生效
func (s *Service) StreakBoostConfig(ctx context.Context) (StreakBoost, error) {
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return StreakBoost{}, err
	}
	if !cfg.Enabled {
		return StreakBoost{}, nil
	}
	return cfg.StreakBoost, nil
}
