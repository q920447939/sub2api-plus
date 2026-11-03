// Package points — [plus] 积分账户与流水
package points

import (
	"errors"
	"fmt"
	"time"
)

// Reason 积分流水来源/消耗类型。新增来源只加常量，不改表结构。
const (
	ReasonCheckin     = "checkin"      // 签到
	ReasonRedeemBonus = "redeem_bonus" // 兑换卡密送积分
	ReasonSlotConsume = "slot_consume" // 幸运九宫格消耗
	ReasonSlotWin     = "slot_win"     // 幸运九宫格积分奖励
	ReasonExpire      = "expire"       // 过期清零
	ReasonAdminAdjust = "admin_adjust" // 管理员手动调整
)

// RedeemBonusTier 卡密面值满 Threshold（美元）送 Points 积分
type RedeemBonusTier struct {
	Threshold float64 `json:"threshold"`
	Points    int64   `json:"points"`
}

// RedeemBonusConfig 兑换卡密送积分
type RedeemBonusConfig struct {
	Enabled bool `json:"enabled"`
	// EnabledAt 开关最近一次打开的时间，之前兑换的卡密不补发；由服务端维护
	EnabledAt *time.Time        `json:"enabled_at,omitempty"`
	Tiers     []RedeemBonusTier `json:"tiers"`
}

// PointsFor 按面值取最高命中档位，未命中返回 0
func (c *RedeemBonusConfig) PointsFor(value float64) int64 {
	var pts int64
	for _, t := range c.Tiers {
		if value+1e-9 >= t.Threshold {
			pts = t.Points
		}
	}
	return pts
}

// Config 积分全局配置
type Config struct {
	// ExpireDays 账户最后一次积分收支后多少天无变动则清零，0=不过期
	ExpireDays int `json:"expire_days"`
	// WarnDays 清零前多少天展示预警，0=不展示
	WarnDays    int               `json:"warn_days"`
	RedeemBonus RedeemBonusConfig `json:"redeem_bonus"`
}

var ErrInvalidConfig = errors.New("points: invalid config")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidConfig, fmt.Sprintf(format, args...))
}

func (c *Config) Validate() error {
	if c.ExpireDays < 0 || c.WarnDays < 0 {
		return invalid("过期天数与预警天数不能为负")
	}
	if c.ExpireDays > 0 && c.WarnDays >= c.ExpireDays {
		return invalid("预警天数必须小于过期天数")
	}
	if len(c.RedeemBonus.Tiers) > 20 {
		return invalid("兑换送积分档位最多 20 个")
	}
	for i, t := range c.RedeemBonus.Tiers {
		if t.Threshold <= 0 || t.Points <= 0 || t.Points > 10_000_000 {
			return invalid("第 %d 个档位的面值和积分必须大于 0", i+1)
		}
		if i > 0 && t.Threshold <= c.RedeemBonus.Tiers[i-1].Threshold {
			return invalid("兑换送积分档位需按面值从小到大排列且不重复")
		}
	}
	if c.RedeemBonus.Enabled && len(c.RedeemBonus.Tiers) == 0 {
		return invalid("开启兑换送积分时至少配置一个档位")
	}
	return nil
}

func DefaultConfig() Config {
	return Config{
		ExpireDays: 365,
		WarnDays:   30,
		RedeemBonus: RedeemBonusConfig{
			Enabled: false,
			Tiers: []RedeemBonusTier{
				{Threshold: 10, Points: 100},
				{Threshold: 50, Points: 600},
				{Threshold: 100, Points: 1500},
			},
		},
	}
}

// ExpiresAt 返回账户基于 lastActiveAt 的过期时间；若不过期返回 nil
func (c *Config) ExpiresAt(lastActiveAt time.Time) *time.Time {
	if c.ExpireDays <= 0 {
		return nil
	}
	t := lastActiveAt.AddDate(0, 0, c.ExpireDays)
	return &t
}

// WarnAt 返回预警开始时间；nil 表示不预警
func (c *Config) WarnAt(lastActiveAt time.Time) *time.Time {
	if c.ExpireDays <= 0 || c.WarnDays <= 0 {
		return nil
	}
	t := lastActiveAt.AddDate(0, 0, c.ExpireDays-c.WarnDays)
	return &t
}
