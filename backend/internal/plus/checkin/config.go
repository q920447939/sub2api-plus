// Package checkin — [plus] 签到配置
package checkin

import (
	"errors"
	"fmt"
)

// Config 签到配置
type Config struct {
	Enabled bool `json:"enabled"`

	// Rewards 7天连签奖励（索引0=第1天…索引6=第7天），单位：积分
	Rewards [7]int64 `json:"rewards"`

	// WeekBonus 第7天额外里程碑奖励（叠加在 Rewards[6] 之上）
	WeekBonus int64 `json:"week_bonus"`

	// IPDailyMax 同一 IP 24小时内最多签到账号数，0=不限
	IPDailyMax int `json:"ip_daily_max"`

	// PopupEnabled 登录后今日未签到时弹出签到卡片
	PopupEnabled bool `json:"popup_enabled"`

	// StreakBoost 连签加成：配置属于签到体系，作用于幸运九宫格的中奖概率
	StreakBoost StreakBoost `json:"streak_boost"`
}

// GridTierCount 幸运九宫格外圈奖档数（与 slotmachine.TierCount 一致）
const GridTierCount = 8

// TierBoost 某个奖档的概率加成，单位：万分比（100 = 1 个百分点）
type TierBoost struct {
	Tier  int `json:"tier"`
	AddBP int `json:"add_bp"`
}

// StreakBoost 连签满 MinStreak 天时，指定奖档概率各加 AddBP，增加的部分从"谢谢惠顾"里扣
type StreakBoost struct {
	Enabled   bool        `json:"enabled"`
	MinStreak int         `json:"min_streak"`
	Tiers     []TierBoost `json:"tiers"`
}

var ErrInvalidConfig = errors.New("checkin: invalid config")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidConfig, fmt.Sprintf(format, args...))
}

func (c *Config) Validate() error {
	for _, v := range c.Rewards {
		if v < 0 {
			return invalid("7 天奖励不能为负")
		}
	}
	if c.WeekBonus < 0 {
		return invalid("第 7 天额外奖励不能为负")
	}
	if c.IPDailyMax < 0 {
		return invalid("同 IP 上限不能为负")
	}
	return c.StreakBoost.Validate()
}

func (b *StreakBoost) Validate() error {
	if b.MinStreak < 1 || b.MinStreak > 365 {
		return invalid("连签加成生效天数需在 1-365 之间")
	}
	seen := map[int]bool{}
	for _, t := range b.Tiers {
		if t.Tier < 0 || t.Tier >= GridTierCount {
			return invalid("连签加成奖档需在 1-%d 之间", GridTierCount)
		}
		if seen[t.Tier] {
			return invalid("连签加成奖档重复")
		}
		seen[t.Tier] = true
		if t.AddBP <= 0 || t.AddBP > 5000 {
			return invalid("连签加成每档需在 0.01%%-50%% 之间")
		}
	}
	if b.Enabled && len(b.Tiers) == 0 {
		return invalid("开启连签加成时至少选择一个奖档")
	}
	return nil
}

func DefaultConfig() Config {
	return Config{
		Enabled:      true,
		Rewards:      [7]int64{10, 20, 30, 40, 50, 60, 100},
		WeekBonus:    50,
		IPDailyMax:   5,
		PopupEnabled: true,
		// 下标对应幸运九宫格默认奖池：4=500 积分、6=$1 余额、7=200 积分
		StreakBoost: StreakBoost{Enabled: true, MinStreak: 3, Tiers: []TierBoost{
			{Tier: 4, AddBP: 100}, {Tier: 6, AddBP: 100}, {Tier: 7, AddBP: 100},
		}},
	}
}

// PointsForDayN 返回第 N 天签到的奖励（7天奖励循环）
func (c *Config) PointsForDayN(n int) int64 {
	if n < 1 {
		n = 1
	}
	idx := (n - 1) % 7
	pts := c.Rewards[idx]
	if idx == 6 { // 每7天加一次满签里程碑
		pts += c.WeekBonus
	}
	return pts
}
