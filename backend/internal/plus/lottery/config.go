// Package lottery — [plus] 兑换刮刮乐活动
package lottery

import (
	"errors"
	"fmt"
	"time"
)

// 抽奖机会状态
const (
	ChanceStatusUnused = "unused"
	ChanceStatusUsed   = "used"
)

// 奖金降级原因（degraded 字段）
const (
	DegradedNone           = ""
	DegradedDailyBudget    = "daily_budget"     // 触及当日预算上限
	DegradedUserDaily      = "user_daily"       // 触及单用户当日上限
	DegradedSingleOrderPct = "single_order_pct" // 触及单笔比例上限
	DegradedPoolFiltered   = "pool_filtered"    // 单笔比例规则筛掉全部奖档后兜底降级
)

// Config 是完整配置的顶层容器，存储在 plus_lottery_config.data
type Config struct {
	Enabled          bool         `json:"enabled"`
	EnabledAt        *time.Time   `json:"enabled_at,omitempty"` // 开关首次打开时间，之前兑换的卡密不补机会
	PromoMaxAmount   float64      `json:"promo_max_amount"`     // 宣传的"最高可得"数字
	ChanceTiers      []ChanceTier `json:"chance_tiers"`
	ChanceValidHours int          `json:"chance_valid_hours"` // 刮刮卡有效期（小时），从兑换时刻起算
	NotesKeyword     string       `json:"notes_keyword"`      // 非空时只计入备注包含该关键词的卡密
	PrizePoolReal    PrizePool    `json:"prize_pool_real"`    // 奖池 A：真实用户抽奖
	PrizePoolVirtual PrizePool    `json:"prize_pool_virtual"` // 奖池 B：虚拟记录专用
	RiskControl      RiskControl  `json:"risk_control"`
	VirtualFeed      VirtualFeed  `json:"virtual_feed"`
}

// ChanceTier 单张卡密面值满 Threshold 得 Chances 张刮刮卡
type ChanceTier struct {
	Threshold float64 `json:"threshold"`
	Chances   int     `json:"chances"`
}

// PrizePool 奖金分档，按权重随机命中某档后在 [Min, Max] 内均匀取值
type PrizePool struct {
	Tiers []PrizeTier `json:"tiers"`
}

type PrizeTier struct {
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Weight int     `json:"weight"`
}

// RiskControl 风控兜底，叠加在奖池之上
type RiskControl struct {
	FloorAmount    float64 `json:"floor_amount"`     // 保底金额（每次必中），默认 0.1
	SingleOrderPct float64 `json:"single_order_pct"` // 单张刮刮卡奖金上限 = 卡密面值 × 比例，(0, 1]
	DailyBudget    float64 `json:"daily_budget"`     // 每日发奖总额上限，0 = 不限
	UserDailyMax   float64 `json:"user_daily_max"`   // 单用户每日中奖上限，0 = 不限
}

// VirtualFeed 虚拟动态与氛围数字
type VirtualFeed struct {
	Enabled               bool    `json:"enabled"`
	DailyTarget           int     `json:"daily_target"`            // 每天约生成多少条虚拟记录
	BaseTotalParticipants int64   `json:"base_total_participants"` // 参与人数基数
	BaseTotalAwarded      float64 `json:"base_total_awarded"`      // 累计送出金额基数
	LeaderboardEnabled    bool    `json:"leaderboard_enabled"`
}

var (
	ErrInvalidChanceTiers  = errors.New("chance tiers must be ascending with positive thresholds and chances; validity 1h~365d")
	ErrInvalidPrizePool    = errors.New("prize pool tiers need 0 <= min <= max, weight >= 0, and total weight > 0")
	ErrPoolExceedsPromo    = errors.New("virtual pool max exceeds promo_max_amount")
	ErrInvalidRiskControl  = errors.New("risk control values out of range")
	ErrInvalidVirtualFeed  = errors.New("virtual feed values out of range")
	ErrInvalidNotesKeyword = errors.New("notes keyword must be at most 64 chars")
)

// DefaultConfig 新部署时的初始配置（关闭状态）
func DefaultConfig() Config {
	return Config{
		Enabled:          false,
		PromoMaxAmount:   88,
		ChanceValidHours: 24,
		ChanceTiers: []ChanceTier{
			{Threshold: 10, Chances: 1},
			{Threshold: 50, Chances: 3},
			{Threshold: 100, Chances: 6},
		},
		PrizePoolReal: PrizePool{Tiers: []PrizeTier{
			{Min: 0.1, Max: 0.2, Weight: 70},
			{Min: 0.5, Max: 1, Weight: 25},
			{Min: 5, Max: 10, Weight: 5},
		}},
		PrizePoolVirtual: PrizePool{Tiers: []PrizeTier{
			{Min: 0.5, Max: 2, Weight: 50},
			{Min: 5, Max: 20, Weight: 35},
			{Min: 30, Max: 88, Weight: 15},
		}},
		RiskControl: RiskControl{FloorAmount: 0.1, SingleOrderPct: 0.5},
		VirtualFeed: VirtualFeed{
			Enabled:               true,
			DailyTarget:           120,
			BaseTotalParticipants: 3000,
			BaseTotalAwarded:      12000,
			LeaderboardEnabled:    true,
		},
	}
}

// Validate 校验配置的合法性。关闭状态的配置同样要求合法，便于管理员预先配好再开启。
func (c *Config) Validate() error {
	if len(c.ChanceTiers) == 0 {
		return ErrInvalidChanceTiers
	}
	for i, t := range c.ChanceTiers {
		if t.Threshold <= 0 || t.Chances <= 0 || t.Chances > 100 {
			return ErrInvalidChanceTiers
		}
		if i > 0 && t.Threshold <= c.ChanceTiers[i-1].Threshold {
			return ErrInvalidChanceTiers
		}
	}
	if c.ChanceValidHours < 1 || c.ChanceValidHours > 24*365 {
		return ErrInvalidChanceTiers
	}
	if len([]rune(c.NotesKeyword)) > 64 {
		return ErrInvalidNotesKeyword
	}
	if err := c.PrizePoolReal.Validate(); err != nil {
		return fmt.Errorf("prize_pool_real: %w", err)
	}
	if err := c.PrizePoolVirtual.Validate(); err != nil {
		return fmt.Errorf("prize_pool_virtual: %w", err)
	}
	if c.PromoMaxAmount < 0 {
		return ErrInvalidRiskControl
	}
	if c.PromoMaxAmount > 0 && c.PrizePoolVirtual.MaxAmount() > c.PromoMaxAmount {
		return ErrPoolExceedsPromo
	}
	rc := c.RiskControl
	if rc.FloorAmount <= 0 || rc.SingleOrderPct <= 0 || rc.SingleOrderPct > 1 ||
		rc.DailyBudget < 0 || rc.UserDailyMax < 0 {
		return ErrInvalidRiskControl
	}
	vf := c.VirtualFeed
	if vf.DailyTarget < 0 || vf.DailyTarget > 1440 || vf.BaseTotalParticipants < 0 || vf.BaseTotalAwarded < 0 {
		return ErrInvalidVirtualFeed
	}
	return nil
}

// ChancesForAmount 返回卡密面值对应的机会数（命中最高档），不满最低档返回 0
func (c *Config) ChancesForAmount(amount float64) int {
	best := 0
	for _, t := range c.ChanceTiers {
		if amount >= t.Threshold {
			best = t.Chances
		}
	}
	return best
}

// Validate 校验奖池
func (p *PrizePool) Validate() error {
	if len(p.Tiers) == 0 {
		return ErrInvalidPrizePool
	}
	total := 0
	for _, t := range p.Tiers {
		if t.Min < 0 || t.Max < t.Min || t.Weight < 0 {
			return ErrInvalidPrizePool
		}
		total += t.Weight
	}
	if total == 0 {
		return ErrInvalidPrizePool
	}
	return nil
}

// MaxAmount 奖池可能抽到的最高金额（权重为 0 的档不会被抽中，不计入）
func (p *PrizePool) MaxAmount() float64 {
	m := 0.0
	for _, t := range p.Tiers {
		if t.Weight > 0 && t.Max > m {
			m = t.Max
		}
	}
	return m
}
