package dispatch

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/plus/channelstatus"
)

// Config 全局配置（单行 + 乐观锁）
type Config struct {
	Detect      bool `json:"detect"`       // 定时检测
	Apply       bool `json:"apply"`        // 权重设置生效（关掉时设置保留，线上按原来的规则分流量）
	Concurrency int  `json:"concurrency"`  // 主动检测的全局并发
	IdleMinutes int  `json:"idle_minutes"` // 近 24 小时没有流量的备用号，多久测一次（不低于计划频率）
	FaultRuns   int  `json:"fault_runs"`   // 检测请求连续出错几次算「测不了」
	// SameAsChannel 健康分和「渠道状态」用同一套规则（读渠道状态的配置）；关掉时用下面自己的
	SameAsChannel bool                `json:"same_as_channel"`
	Rules         channelstatus.Rules `json:"rules"`
}

func DefaultConfig() Config {
	return Config{Detect: true, Apply: true, Concurrency: 4, IdleMinutes: 360, FaultRuns: 3,
		SameAsChannel: true, Rules: channelstatus.DefaultRules()}
}

func (c *Config) normalize() error {
	d := DefaultConfig()
	if c.Concurrency <= 0 || c.Concurrency > 32 {
		c.Concurrency = d.Concurrency
	}
	if c.IdleMinutes <= 0 {
		c.IdleMinutes = d.IdleMinutes
	}
	if c.IdleMinutes > maxIntervalMinutes {
		c.IdleMinutes = maxIntervalMinutes
	}
	if c.FaultRuns <= 0 || c.FaultRuns > 50 {
		c.FaultRuns = d.FaultRuns
	}
	r := &c.Rules
	if r.UpMin == 0 && r.WarnMin == 0 && r.MinSample == 0 {
		*r = channelstatus.DefaultRules()
	}
	if r.IgnoredCategories == nil {
		r.IgnoredCategories = []string{}
	}
	if err := r.Validate(); err != nil {
		return fmt.Errorf("%w：%v", ErrInvalid, err)
	}
	return nil
}
