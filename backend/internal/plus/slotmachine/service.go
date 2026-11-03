// Package slotmachine — [plus] 幸运九宫格业务编排
//
// 一次摇奖（同一事务）：参与条件 → 扣积分 → 注入奖金罐 → 修正器调概率 → 排除罐内余额不足的奖档
// （概率转给谢谢惠顾）→ 抽奖 → 扣罐 → 发奖 → 写中奖记录。前端只按返回的 tier_index 播放动画。
package slotmachine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/plus/points"
)

// CacheInvalidator 余额变动后失效上游缓存，由桥接层注入
type CacheInvalidator interface {
	InvalidateUserBalance(ctx context.Context, userID int64)
}

type Service struct {
	repo        *Repository
	pointsSvc   *points.Service
	invalidator CacheInvalidator
	modifiers   []Modifier
	rng         RNG
	now         func() time.Time

	cacheMu sync.RWMutex
	cache   *runtimeCfg
}

// runtimeCfg 配置 + 构造好的规则链，短 TTL 缓存
type runtimeCfg struct {
	cfg   Config
	ver   int64
	rules []Rule
	at    time.Time
}

const cfgTTL = 10 * time.Second

// NewService boost 可为 nil（不启用连签加成）
func NewService(repo *Repository, pointsSvc *points.Service, invalidator CacheInvalidator, boost StreakBoostSource) *Service {
	s := &Service{repo: repo, pointsSvc: pointsSvc, invalidator: invalidator, rng: DefaultRNG, now: time.Now}
	if boost != nil {
		s.modifiers = append(s.modifiers, streakBoostMod{src: boost})
	}
	return s
}

// SetRNG 替换随机源，仅用于测试
func (s *Service) SetRNG(rng RNG) { s.rng = rng }

// SetClock 替换时钟，仅用于测试
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) runtime(ctx context.Context) (*runtimeCfg, error) {
	s.cacheMu.RLock()
	rc := s.cache
	s.cacheMu.RUnlock()
	if rc != nil && time.Since(rc.at) < cfgTTL {
		return rc, nil
	}
	cfg, ver, err := s.repo.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	rules, err := BuildRules(cfg.Rules)
	if err != nil {
		return nil, fmt.Errorf("build rules: %w", err)
	}
	rc = &runtimeCfg{cfg: cfg, ver: ver, rules: rules, at: time.Now()}
	s.cacheMu.Lock()
	s.cache = rc
	s.cacheMu.Unlock()
	return rc, nil
}

func (s *Service) invalidate() {
	s.cacheMu.Lock()
	s.cache = nil
	s.cacheMu.Unlock()
}

// Config 当前配置（带缓存）
func (s *Service) Config(ctx context.Context) (Config, int64, error) {
	rc, err := s.runtime(ctx)
	if err != nil {
		return Config{}, 0, err
	}
	return rc.cfg, rc.ver, nil
}

func (s *Service) dayStart() time.Time {
	return timezone.StartOfDay(s.now().In(timezone.Location()))
}

// ─── 用户事实（懒加载 + 单次请求内缓存） ─────────────────────────────────

type userFacts struct {
	s      *Service
	userID int64
	now    time.Time

	streak    *int
	todaySpin *int
}

func (s *Service) facts(userID int64) *userFacts {
	return &userFacts{s: s, userID: userID, now: s.now()}
}

func (f *userFacts) Now() time.Time { return f.now }

func (f *userFacts) RecentRecharge(ctx context.Context, since time.Time) (float64, error) {
	return f.s.repo.RecentRecharge(ctx, f.userID, since)
}

func (f *userFacts) TodaySpinCount(ctx context.Context) (int, error) {
	if f.todaySpin != nil {
		return *f.todaySpin, nil
	}
	n, err := f.s.repo.TodaySpinCount(ctx, f.userID, timezone.StartOfDay(f.now.In(timezone.Location())))
	if err != nil {
		return 0, err
	}
	f.todaySpin = &n
	return n, nil
}

func (f *userFacts) LastSpinAt(ctx context.Context) (time.Time, bool, error) {
	t, err := f.s.repo.LastSpinTime(ctx, f.userID)
	if err != nil {
		return time.Time{}, false, err
	}
	return t.Time, t.Valid, nil
}

func (f *userFacts) CheckinStreak(ctx context.Context) (int, error) {
	if f.streak != nil {
		return *f.streak, nil
	}
	today := timezone.StartOfDay(f.now.In(timezone.Location())).Format("2006-01-02")
	n, err := f.s.repo.CheckinStreak(ctx, f.userID, today)
	if err != nil {
		return 0, err
	}
	f.streak = &n
	return n, nil
}

// ─── 用户端 ──────────────────────────────────────────────────────────────

// PublicTier 用户端可见的奖档。不下发任何概率数字，只告诉用户这一格当前有加成
type PublicTier struct {
	Label     string `json:"label"`
	Desc      string `json:"desc,omitempty"`
	Emoji     string `json:"emoji,omitempty"`
	Badge     string `json:"badge,omitempty"`
	PrizeType string `json:"prize_type"`
	Boosted   bool   `json:"boosted,omitempty"`
}

// PublicBoost 用户端可见的加成说明（不含各档增加的概率）
type PublicBoost struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// RuleStatus 参与条件当前状态
type RuleStatus struct {
	Type   string `json:"type"`
	Text   string `json:"text"`
	Passed bool   `json:"passed"`
}

type Overview struct {
	Enabled     bool          `json:"enabled"`
	CostPerSpin int64         `json:"cost_per_spin"`
	Balance     int64         `json:"balance"`
	Streak      int           `json:"streak"`
	Tiers       []PublicTier  `json:"tiers"`
	Rules       []RuleStatus  `json:"rules"`
	Boosts      []PublicBoost `json:"boosts"`
	// EarnWays 当前可用的积分获取方式，积分不足时前端据此引导
	EarnWays []points.EarnWay `json:"earn_ways"`
	CanSpin  bool             `json:"can_spin"`
	// Blocker 首个未满足的条件：disabled / insufficient_points / 规则 Code
	Blocker    string `json:"blocker,omitempty"`
	Reason     string `json:"reason,omitempty"`
	RetryAfter int    `json:"retry_after,omitempty"` // 秒
	SpinsToday int    `json:"spins_today"`
}

func (s *Service) Overview(ctx context.Context, userID int64) (*Overview, error) {
	rc, err := s.runtime(ctx)
	if err != nil {
		return nil, err
	}
	cfg := rc.cfg
	ov := &Overview{Enabled: cfg.Enabled, CostPerSpin: cfg.CostPerSpin, Tiers: []PublicTier{}, Rules: []RuleStatus{}, Boosts: []PublicBoost{}}
	for _, t := range cfg.Tiers {
		ov.Tiers = append(ov.Tiers, PublicTier{Label: t.Label, Desc: t.Desc, Emoji: t.Emoji, Badge: t.Badge,
			PrizeType: t.PrizeType})
	}
	ov.EarnWays = s.pointsSvc.EarnWays(ctx, userID)
	pOv, err := s.pointsSvc.Overview(ctx, userID)
	if err != nil {
		return nil, err
	}
	ov.Balance = pOv.Balance

	f := s.facts(userID)
	if ov.Streak, err = f.CheckinStreak(ctx); err != nil {
		return nil, err
	}
	if ov.SpinsToday, err = f.TodaySpinCount(ctx); err != nil {
		return nil, err
	}
	_, boosts, err := ApplyModifiers(ctx, s.modifiers, f, &cfg)
	if err != nil {
		return nil, err
	}
	for _, b := range boosts {
		ov.Boosts = append(ov.Boosts, PublicBoost{Type: b.Type, Text: b.Text})
		for i, add := range b.Tiers {
			if i >= 0 && i < len(ov.Tiers) && add > 0 {
				ov.Tiers[i].Boosted = true
			}
		}
	}
	if !cfg.Enabled {
		ov.Blocker, ov.Reason = "disabled", "活动暂未开放"
		return ov, nil
	}
	for _, r := range rc.rules {
		st := RuleStatus{Type: r.Type(), Text: r.Describe(), Passed: true}
		if err := r.Check(ctx, f); err != nil {
			re, ok := AsRuleError(err)
			if !ok {
				return nil, err
			}
			st.Passed = false
			if ov.Blocker == "" {
				ov.Blocker, ov.Reason = re.Code, re.Message
				ov.RetryAfter = int(re.RetryAfter.Seconds() + 0.999)
			}
		}
		ov.Rules = append(ov.Rules, st)
	}
	if ov.Blocker == "" && ov.Balance < cfg.CostPerSpin {
		ov.Blocker = "insufficient_points"
		ov.Reason = fmt.Sprintf("积分不足，还差 %d 积分", cfg.CostPerSpin-ov.Balance)
	}
	ov.CanSpin = ov.Blocker == ""
	return ov, nil
}

type SpinResult struct {
	SpinID       int64     `json:"spin_id"`
	TierIndex    int       `json:"tier_index"`
	PrizeType    string    `json:"prize_type"`
	PrizePoints  int64     `json:"prize_points,omitempty"`
	PrizeBalance float64   `json:"prize_balance,omitempty"`
	Label        string    `json:"label"`
	Desc         string    `json:"desc,omitempty"`
	Emoji        string    `json:"emoji,omitempty"`
	RedeemCode   string    `json:"redeem_code,omitempty"`
	Boosted      bool      `json:"boosted"`
	Cost         int64     `json:"cost"`
	Balance      int64     `json:"balance"`
	CreatedAt    time.Time `json:"created_at"`
	Replayed     bool      `json:"replayed"`
}

// ExcludeByPots 罐内余额（含本次注入）付不起的奖档概率转给谢谢惠顾，返回新概率与被排除的下标
func ExcludeByPots(cfg *Config, probs []int, avail map[string]int64) ([]int, []int) {
	none := cfg.NoneIndex()
	out := append([]int(nil), probs...)
	var excluded []int
	for i, t := range cfg.Tiers {
		if i == none || out[i] == 0 {
			continue
		}
		if cfg.ValuePoints(t) > avail[t.Pot] {
			out[none] += out[i]
			out[i] = 0
			excluded = append(excluded, i)
		}
	}
	return out, excluded
}

func (s *Service) Spin(ctx context.Context, userID int64, idempotencyKey string) (*SpinResult, error) {
	if !validIdempotencyKey(idempotencyKey) {
		return nil, ErrIdempotencyKey
	}
	rc, err := s.runtime(ctx)
	if err != nil {
		return nil, err
	}
	cfg := rc.cfg
	if !cfg.Enabled {
		return nil, ErrActivityDisabled
	}

	var (
		result         *SpinResult
		balanceChanged bool
	)
	err = s.repo.withTx(ctx, func(tx *sql.Tx) error {
		// 锁用户行：串行化同一用户的摇奖，让参与条件看到的次数/冷却与本次写入一致
		var email string
		if err := tx.QueryRowContext(ctx,
			`SELECT email FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&email); err != nil {
			return fmt.Errorf("lock user: %w", err)
		}
		existing, err := s.repo.FindSpinByKey(ctx, tx, userID, idempotencyKey)
		if err != nil {
			return err
		}
		if existing != nil {
			result = s.replay(cfg, existing)
			return nil
		}

		f := s.facts(userID)
		if err := CheckRules(ctx, rc.rules, f); err != nil {
			return err
		}
		probs, boosts, err := ApplyModifiers(ctx, s.modifiers, f, &cfg)
		if err != nil {
			return err
		}

		ledger, err := points.Debit(ctx, tx, userID, cfg.CostPerSpin, points.ReasonSlotConsume, 0,
			fmt.Sprintf("幸运九宫格 -%d 积分", cfg.CostPerSpin))
		if err != nil {
			if errors.Is(err, points.ErrInsufficientPoints) {
				return ErrInsufficientPoints
			}
			return fmt.Errorf("debit points: %w", err)
		}

		pots, err := s.repo.LockPots(ctx, tx)
		if err != nil {
			return err
		}
		inSmall, inJackpot := cfg.Pot.Inflow(cfg.CostPerSpin)
		avail := map[string]int64{PotSmall: pots[PotSmall] + inSmall, PotJackpot: pots[PotJackpot] + inJackpot}
		probs, excluded := ExcludeByPots(&cfg, probs, avail)

		idx := PickBP(probs, s.rng)
		tier := cfg.Tiers[idx]
		payout := cfg.ValuePoints(tier)

		spinID, createdAt, err := s.repo.InsertSpin(ctx, tx, spinInsert{
			UserID: userID, Key: idempotencyKey, Cost: cfg.CostPerSpin, TierIndex: idx, Tier: tier,
			LedgerID: ledger.ID, Excluded: excluded, Boosted: boostedTier(boosts, idx), PotPayout: payout,
		})
		if err != nil {
			if isUniqueViolation(err) {
				return ErrIdempotencyKey
			}
			return fmt.Errorf("insert spin: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE plus_points_ledger SET ref_id = $1 WHERE id = $2`, spinID, ledger.ID); err != nil {
			return fmt.Errorf("update ledger ref: %w", err)
		}

		for pot, in := range map[string]int64{PotSmall: inSmall, PotJackpot: inJackpot} {
			if in > 0 {
				if _, err := s.repo.MovePot(ctx, tx, pot, in, PotReasonInflow, spinID, 0, ""); err != nil {
					return fmt.Errorf("pot inflow: %w", err)
				}
			}
		}
		if payout > 0 {
			if _, err := s.repo.MovePot(ctx, tx, tier.Pot, -payout, PotReasonPayout, spinID, 0, tier.Label); err != nil {
				return fmt.Errorf("pot payout: %w", err)
			}
		}

		kind, _ := PrizeKindOf(tier.PrizeType)
		award, err := kind.Award(ctx, AwardInput{Tx: tx, UserID: userID, SpinID: spinID, Tier: tier})
		if err != nil {
			return err
		}
		balanceChanged = award.BalanceChanged
		balanceAfter := ledger.Balance
		if award.PointsBalance != nil {
			balanceAfter = *award.PointsBalance
		}
		if award.WinLedgerID.Valid {
			if _, err := tx.ExecContext(ctx, `UPDATE plus_slot_spins SET win_ledger_id = $1 WHERE id = $2`,
				award.WinLedgerID.Int64, spinID); err != nil {
				return fmt.Errorf("update win ledger: %w", err)
			}
		}

		if tier.PrizeType != PrizeTypeNone {
			if err := s.repo.InsertFeed(ctx, tx, FeedItem{DisplayName: MaskEmail(email),
				TierIndex: idx, Label: tier.Label, Emoji: tier.Emoji, CreatedAt: createdAt}, 0); err != nil {
				return fmt.Errorf("insert feed: %w", err)
			}
		}

		result = &SpinResult{
			SpinID: spinID, TierIndex: idx, PrizeType: tier.PrizeType,
			PrizePoints: tier.PrizePoints, PrizeBalance: tier.PrizeBalance,
			Label: tier.Label, Desc: tier.Desc, Emoji: tier.Emoji, RedeemCode: award.RedeemCode,
			Boosted: boostedTier(boosts, idx), Cost: cfg.CostPerSpin,
			Balance: balanceAfter, CreatedAt: createdAt,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result.Replayed {
		if pOv, err := s.pointsSvc.Overview(ctx, userID); err == nil {
			result.Balance = pOv.Balance
		}
	}
	if balanceChanged && s.invalidator != nil {
		s.invalidator.InvalidateUserBalance(ctx, userID)
	}
	return result, nil
}

func (s *Service) replay(cfg Config, rec *SpinRecord) *SpinResult {
	r := &SpinResult{SpinID: rec.ID, TierIndex: rec.TierIndex, PrizeType: rec.PrizeType,
		PrizePoints: rec.PrizePoints, PrizeBalance: rec.PrizeBalance, Cost: rec.Cost,
		CreatedAt: rec.CreatedAt, Replayed: true}
	if rec.TierIndex >= 0 && rec.TierIndex < len(cfg.Tiers) {
		t := cfg.Tiers[rec.TierIndex]
		r.Label, r.Desc, r.Emoji = t.Label, t.Desc, t.Emoji
	}
	r.RedeemCode = spinRedeemCode(*rec)
	return r
}

// HistoryItem 抽奖记录（标签取当前配置，与九宫格展示一致）
type HistoryItem struct {
	SpinRecord
	Emoji      string `json:"emoji,omitempty"`
	RedeemCode string `json:"redeem_code,omitempty"`
}

// History 用户抽奖记录分页；winOnly 只看中奖
func (s *Service) History(ctx context.Context, userID int64, winOnly bool, page, size int) ([]HistoryItem, int64, int, int, error) {
	page, size = normPage(page, size)
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	recs, total, err := s.repo.ListHistory(ctx, userID, winOnly, page, size)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	out := make([]HistoryItem, 0, len(recs))
	for _, r := range recs {
		it := HistoryItem{SpinRecord: r}
		if r.TierIndex >= 0 && r.TierIndex < len(cfg.Tiers) {
			it.Label, it.Emoji = cfg.Tiers[r.TierIndex].Label, cfg.Tiers[r.TierIndex].Emoji
		}
		it.RedeemCode = spinRedeemCode(r)
		out = append(out, it)
	}
	return out, total, page, size, nil
}

// Feed 跑马灯：今日最新 MarqueeSize 条中奖记录（真实与虚拟混排）
func (s *Service) Feed(ctx context.Context) ([]FeedItem, error) {
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled || !cfg.Atmosphere.MarqueeEnabled {
		return []FeedItem{}, nil
	}
	return s.repo.ListFeed(ctx, s.dayStart(), cfg.Atmosphere.MarqueeSize)
}

// ─── 管理端 ──────────────────────────────────────────────────────────────

// PotPreview 每次摇奖两个罐的注入与期望支出（积分）
type PotPreview struct {
	SmallInflow     int64   `json:"small_inflow"`
	JackpotInflow   int64   `json:"jackpot_inflow"`
	SmallExpected   float64 `json:"small_expected"`
	JackpotExpected float64 `json:"jackpot_expected"`
}

// AdminState 管理端一次性拉取的全部状态
type AdminState struct {
	Config      Config          `json:"config"`
	Version     int64           `json:"version"`
	RuleCatalog []RuleMeta      `json:"rule_catalog"`
	PrizeKinds  []PrizeKindMeta `json:"prize_kinds"`
	Base        PotPreview      `json:"base"`
	Boosted     PotPreview      `json:"boosted"` // 连签加成全部生效时
	Pots        []PotStat       `json:"pots"`
	Spins       SpinStat        `json:"spins"`
}

func (s *Service) potPreview(cfg *Config, probs []int) PotPreview {
	p := PotPreview{}
	p.SmallInflow, p.JackpotInflow = cfg.Pot.Inflow(cfg.CostPerSpin)
	p.SmallExpected, p.JackpotExpected = cfg.PotExpectation(probs)
	return p
}

func (s *Service) AdminState(ctx context.Context) (*AdminState, error) {
	cfg, ver, err := s.repo.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	st := &AdminState{Config: cfg, Version: ver, RuleCatalog: RuleCatalog(), PrizeKinds: PrizeKindCatalog()}
	if cfg.Validate() == nil {
		st.Base = s.potPreview(&cfg, cfg.Probs())
		boosted, _, err := ApplyModifiers(ctx, s.modifiers, previewFacts{}, &cfg)
		if err != nil {
			return nil, err
		}
		st.Boosted = s.potPreview(&cfg, boosted)
	}
	st.Pots, st.Spins, err = s.repo.PotStats(ctx, s.dayStart())
	return st, err
}

func (s *Service) AdminSaveConfig(ctx context.Context, cfg Config, expectVer int64) (int64, error) {
	if err := cfg.Validate(); err != nil {
		return 0, err
	}
	ver, err := s.repo.SaveConfig(ctx, cfg, expectVer)
	if err != nil {
		return 0, err
	}
	s.invalidate()
	return ver, nil
}

// AdminAdjustPot 管理员加水（delta>0）或抽水（delta<0），必须填写备注
func (s *Service) AdminAdjustPot(ctx context.Context, pot string, delta, operatorID int64, note string) (int64, error) {
	if pot != PotSmall && pot != PotJackpot {
		return 0, invalid("未知的奖金罐: %s", pot)
	}
	if delta == 0 || delta > 100_000_000 || delta < -100_000_000 {
		return 0, invalid("调整数量需为非零且绝对值不超过 1 亿")
	}
	if note == "" || len([]rune(note)) > 200 {
		return 0, invalid("请填写 1-200 字的调整说明")
	}
	reason := PotReasonAdminAdd
	if delta < 0 {
		reason = PotReasonAdminSub
	}
	var bal int64
	err := s.repo.withTx(ctx, func(tx *sql.Tx) error {
		var err error
		bal, err = s.repo.MovePot(ctx, tx, pot, delta, reason, 0, operatorID, note)
		return err
	})
	return bal, err
}

func (s *Service) AdminPotLedger(ctx context.Context, manualOnly bool, limit int) ([]PotLedgerItem, error) {
	return s.repo.ListPotLedger(ctx, manualOnly, clampLimit(limit, 200))
}

func (s *Service) AdminSpins(ctx context.Context, q SpinQuery) ([]AdminSpin, int64, int, int, error) {
	q.Page, q.PageSize = normPage(q.Page, q.PageSize)
	list, total, err := s.repo.ListAdminSpins(ctx, q)
	return list, total, q.Page, q.PageSize, err
}

func (s *Service) AdminFeed(ctx context.Context, limit int) ([]AdminFeedItem, error) {
	return s.repo.ListAdminFeed(ctx, clampLimit(limit, 200))
}

// ─── 工具 ──────────────────────────────────────────────────────────────

func normPage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	return page, size
}

func clampLimit(v, max int) int {
	if v <= 0 {
		return 50
	}
	if v > max {
		return max
	}
	return v
}

func boostedTier(boosts []Boost, idx int) bool {
	for _, b := range boosts {
		if b.Tiers[idx] > 0 {
			return true
		}
	}
	return false
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
