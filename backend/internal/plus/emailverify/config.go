// Package emailverify — 本模块自己的设置（存 plus_settings，key = email_verify）。
package emailverify

import (
	"database/sql"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/plus/plusconfig"
)

// Config 邮箱验证的业务规则，后台「邮箱验证 → 设置」修改，即时生效
type Config struct {
	RewardPoints  int64 `json:"reward_points"`   // 充值过的用户验证成功奖励积分，0 = 关闭；每人一次
	InviteTTLDays int   `json:"invite_ttl_days"` // 一次性验证链接有效期
}

func DefaultConfig() Config {
	return Config{RewardPoints: 200, InviteTTLDays: 7}
}

func (c *Config) Validate() error {
	switch {
	case c.RewardPoints < 0 || c.RewardPoints > 1_000_000:
		return errors.New("验证奖励积分需在 0 到 1000000 之间")
	case c.InviteTTLDays < 1 || c.InviteTTLDays > 90:
		return errors.New("验证链接有效期需在 1 到 90 天之间")
	}
	return nil
}

// ConfigStore 本模块的设置存储
type ConfigStore = plusconfig.Store[Config]

func NewConfigStore(db *sql.DB) *ConfigStore {
	return plusconfig.New(db, "email_verify", DefaultConfig, (*Config).Validate)
}
