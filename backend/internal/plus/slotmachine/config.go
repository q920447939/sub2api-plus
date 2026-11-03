// Package slotmachine — [plus] 积分乐园 · 幸运九宫格配置
//
// 概率统一用万分比整数（ProbBP，10000 = 100%）存储，避免浮点误差。
package slotmachine

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// TierCount 九宫格外圈固定 8 格，下标 0..7 按顺时针从左上角开始
const TierCount = 8

// FullBP 100% 对应的万分比
const FullBP = 10000

// 奖金罐
const (
	PotSmall   = "small"   // 小奖罐：常规奖档
	PotJackpot = "jackpot" // 大奖罐：标记为大奖的奖档
)

// SlotTier 一个奖档
type SlotTier struct {
	ProbBP       int     `json:"prob_bp"`
	PrizeType    string  `json:"prize_type"`
	PrizePoints  int64   `json:"prize_points,omitempty"`
	PrizeBalance float64 `json:"prize_balance,omitempty"`
	// Pot 从哪个奖金罐出奖；谢谢惠顾为空
	Pot   string `json:"pot,omitempty"`
	Label string `json:"label"`           // 格子主标题，如"500 积分"
	Desc  string `json:"desc,omitempty"`  // 副标题
	Emoji string `json:"emoji,omitempty"` // 格子图标
	Badge string `json:"badge,omitempty"` // 左上角角标
}

// PotConfig 每次消耗积分按百分比注入两个罐，剩余部分回收（从系统中消失）
type PotConfig struct {
	SmallPct   int `json:"small_pct"`
	JackpotPct int `json:"jackpot_pct"`
}

// BurnPct 回收比例
func (p PotConfig) BurnPct() int { return 100 - p.SmallPct - p.JackpotPct }

// Inflow 一次消耗 cost 积分时注入两个罐的数量
func (p PotConfig) Inflow(cost int64) (small, jackpot int64) {
	return cost * int64(p.SmallPct) / 100, cost * int64(p.JackpotPct) / 100
}

// Atmosphere 人气氛围：跑马灯 + 虚拟中奖记录
type Atmosphere struct {
	MarqueeEnabled bool `json:"marquee_enabled"`
	// MarqueeSize 跑马灯滚动"今日最新 X 条"中奖记录
	MarqueeSize    int  `json:"marquee_size"`
	VirtualEnabled bool `json:"virtual_enabled"`
	// VirtualDailyTarget 每日虚拟记录目标条数（按小时曲线分布，每日 0.7~1.3 倍浮动）
	VirtualDailyTarget int `json:"virtual_daily_target"`
	// VirtualProbs 虚拟记录在各奖档上的分布（万分比），谢谢惠顾档必须为 0
	VirtualProbs []int `json:"virtual_probs"`
}

// RuleConfig 一条参与条件，Type 对应 rules.go 注册表
type RuleConfig struct {
	Type    string         `json:"type"`
	Enabled bool           `json:"enabled"`
	Params  map[string]any `json:"params"`
}

// Config 幸运九宫格完整配置（plus_slot_config 单行 JSONB）
type Config struct {
	Enabled     bool  `json:"enabled"`
	CostPerSpin int64 `json:"cost_per_spin"`
	// PointsPerYuan 积分估值：N 积分 = $1，余额奖按此折算后从奖金罐扣除
	PointsPerYuan int64        `json:"points_per_yuan"`
	Tiers         []SlotTier   `json:"tiers"`
	Pot           PotConfig    `json:"pot"`
	Rules         []RuleConfig `json:"rules"`
	Atmosphere    Atmosphere   `json:"atmosphere"`
}

var (
	ErrInvalidConfig    = errors.New("slotmachine: invalid config")
	ErrActivityDisabled = errors.New("slotmachine: activity is disabled")
	// ErrInsufficientPoints 积分不足是摇奖的固有条件，不做成可配置规则
	ErrInsufficientPoints = errors.New("slotmachine: insufficient points")
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidConfig, fmt.Sprintf(format, args...))
}

// NoneIndex 谢谢惠顾档下标；配置校验保证恰好一个
func (c *Config) NoneIndex() int {
	for i, t := range c.Tiers {
		if t.PrizeType == PrizeTypeNone {
			return i
		}
	}
	return -1
}

// Validate 校验并规整配置；奖品字段交给各奖品类型校验
func (c *Config) Validate() error {
	if c.CostPerSpin <= 0 || c.CostPerSpin > 1_000_000 {
		return invalid("每次消耗积分需在 1-1000000 之间")
	}
	if c.PointsPerYuan <= 0 {
		return invalid("积分估值必须大于 0")
	}
	if len(c.Tiers) != TierCount {
		return invalid("奖池必须正好 %d 档", TierCount)
	}
	total, none := 0, 0
	for i := range c.Tiers {
		t := &c.Tiers[i]
		t.Label = strings.TrimSpace(t.Label)
		if t.ProbBP < 0 || t.ProbBP > FullBP {
			return invalid("第 %d 档概率需在 0-100%% 之间", i+1)
		}
		if t.Label == "" || utf8.RuneCountInString(t.Label) > 16 {
			return invalid("第 %d 档标题需为 1-16 个字符", i+1)
		}
		if utf8.RuneCountInString(t.Desc) > 24 || utf8.RuneCountInString(t.Badge) > 8 || utf8.RuneCountInString(t.Emoji) > 8 {
			return invalid("第 %d 档副标题/角标/图标过长", i+1)
		}
		kind, ok := PrizeKindOf(t.PrizeType)
		if !ok {
			return invalid("第 %d 档奖品类型不支持: %s", i+1, t.PrizeType)
		}
		if err := kind.Normalize(t); err != nil {
			return invalid("第 %d 档%s", i+1, err.Error())
		}
		if t.PrizeType == PrizeTypeNone {
			none++
			t.Pot = ""
		} else if t.Pot != PotSmall && t.Pot != PotJackpot {
			return invalid("第 %d 档需指定从小奖罐还是大奖罐出奖", i+1)
		}
		total += t.ProbBP
	}
	if none != 1 {
		return invalid("奖池需要且只能有 1 个「谢谢惠顾」档")
	}
	if total != FullBP {
		return invalid("各档概率合计必须为 100%%（当前 %.2f%%）", float64(total)/100)
	}
	if c.Pot.SmallPct < 0 || c.Pot.JackpotPct < 0 || c.Pot.SmallPct+c.Pot.JackpotPct > 100 {
		return invalid("奖金罐注入比例需为非负数且合计不超过 100%%")
	}
	if err := c.Atmosphere.validate(c.NoneIndex()); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, rc := range c.Rules {
		if seen[rc.Type] {
			return invalid("参与条件 %s 重复", rc.Type)
		}
		seen[rc.Type] = true
		if _, err := BuildRule(rc); err != nil {
			return err
		}
	}
	return nil
}

func (a *Atmosphere) validate(noneIdx int) error {
	if a.MarqueeSize < 1 || a.MarqueeSize > 100 {
		return invalid("跑马灯滚动条数需在 1-100 之间")
	}
	if a.VirtualDailyTarget < 0 || a.VirtualDailyTarget > 5000 {
		return invalid("每日虚拟记录条数需在 0-5000 之间")
	}
	if len(a.VirtualProbs) != TierCount {
		return invalid("虚拟记录奖品分布必须正好 %d 档", TierCount)
	}
	total := 0
	for i, p := range a.VirtualProbs {
		if p < 0 {
			return invalid("虚拟记录第 %d 档概率不能为负", i+1)
		}
		if i == noneIdx && p != 0 {
			return invalid("虚拟记录不能落在「谢谢惠顾」档")
		}
		total += p
	}
	if total != FullBP {
		return invalid("虚拟记录奖品分布合计必须为 100%%（当前 %.2f%%）", float64(total)/100)
	}
	return nil
}

// ValuePoints 奖档价值（积分口径），谢谢惠顾为 0
func (c *Config) ValuePoints(t SlotTier) int64 {
	if kind, ok := PrizeKindOf(t.PrizeType); ok {
		return kind.ValuePoints(c, t)
	}
	return 0
}

// Probs 基础概率（万分比）
func (c *Config) Probs() []int {
	p := make([]int, len(c.Tiers))
	for i, t := range c.Tiers {
		p[i] = t.ProbBP
	}
	return p
}

// PotExpectation 给定概率下，每次摇奖两个罐的期望支出（积分）
func (c *Config) PotExpectation(probs []int) (small, jackpot float64) {
	for i, t := range c.Tiers {
		v := float64(probs[i]) / FullBP * float64(c.ValuePoints(t))
		switch t.Pot {
		case PotSmall:
			small += v
		case PotJackpot:
			jackpot += v
		}
	}
	return
}

func DefaultConfig() Config {
	return Config{
		Enabled:       false,
		CostPerSpin:   100,
		PointsPerYuan: 100,
		Tiers: []SlotTier{
			{ProbBP: 25, PrizeType: PrizeTypeBalance, PrizeBalance: 20, Pot: PotJackpot, Label: "$20 余额", Desc: "超级大奖", Emoji: "🧧", Badge: "HOT"},
			{ProbBP: 2500, PrizeType: PrizeTypePoints, PrizePoints: 20, Pot: PotSmall, Label: "20 积分", Desc: "小有收获", Emoji: "🪙"},
			{ProbBP: 1200, PrizeType: PrizeTypePoints, PrizePoints: 100, Pot: PotSmall, Label: "100 积分", Desc: "原地回本", Emoji: "⚡", Badge: "回本"},
			{ProbBP: 2255, PrizeType: PrizeTypeNone, Label: "谢谢惠顾", Desc: "下次一定中", Emoji: "🍀"},
			{ProbBP: 120, PrizeType: PrizeTypePoints, PrizePoints: 500, Pot: PotSmall, Label: "500 积分", Desc: "暴击大奖", Emoji: "💎", Badge: "超人气"},
			{ProbBP: 3000, PrizeType: PrizeTypePoints, PrizePoints: 50, Pot: PotSmall, Label: "50 积分", Desc: "小试牛刀", Emoji: "🎈"},
			{ProbBP: 400, PrizeType: PrizeTypeBalance, PrizeBalance: 1, Pot: PotSmall, Label: "$1 余额", Desc: "即时到账", Emoji: "💵", Badge: "余额"},
			{ProbBP: 500, PrizeType: PrizeTypePoints, PrizePoints: 200, Pot: PotSmall, Label: "200 积分", Desc: "手气不错", Emoji: "🎁"},
		},
		Pot: PotConfig{SmallPct: 60, JackpotPct: 5},
		Rules: []RuleConfig{
			{Type: RuleRecharge, Enabled: true, Params: map[string]any{"window_days": 30, "min_amount": 1}},
			{Type: RuleDailyLimit, Enabled: true, Params: map[string]any{"max": 5}},
			{Type: RuleCooldown, Enabled: true, Params: map[string]any{"seconds": 3}},
		},
		Atmosphere: Atmosphere{
			MarqueeEnabled:     true,
			MarqueeSize:        20,
			VirtualEnabled:     true,
			VirtualDailyTarget: 120,
			VirtualProbs:       []int{300, 1500, 1800, 0, 1200, 1500, 2200, 1500},
		},
	}
}
