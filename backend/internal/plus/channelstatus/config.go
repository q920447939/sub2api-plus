// Package channelstatus — [plus] 渠道状态配置
package channelstatus

import (
	"errors"
	"fmt"
	"math"
)

var ErrInvalidConfig = errors.New("channelstatus: invalid config")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidConfig, fmt.Sprintf(format, args...))
}

// GroupLayout 分组的展示编排；切片顺序即展示顺序
type GroupLayout struct {
	GroupID     int64  `json:"group_id"`
	Visible     bool   `json:"visible"`
	DisplayName string `json:"display_name"` // 留空使用分组原名
	Subtitle    string `json:"subtitle"`
}

// PlatformLayout 平台 Tab 的展示编排；切片顺序即 Tab 顺序
type PlatformLayout struct {
	Platform string        `json:"platform"`
	Visible  bool          `json:"visible"`
	TabName  string        `json:"tab_name"` // 留空使用平台默认名
	Groups   []GroupLayout `json:"groups"`
}

// Rules 健康规则。口径沿用上游 V2（错误率 + TTFT P50 + 缓存命中率加权），
// 但在此独立持有，评分实现通过 Scorer 接口隔离，不直接耦合 V2 配置。
type Rules struct {
	UpMin             float64  `json:"up_min"`             // 分数 >= 此值为正常
	WarnMin           float64  `json:"warn_min"`           // 分数 >= 此值为降级，否则异常
	MinSample         int64    `json:"min_sample"`         // 当前状态（判定窗口）的最小样本
	HistoryMinSample  int64    `json:"history_min_sample"` // 历史条每一格的最小样本
	WarningErrorRate  float64  `json:"warning_error_rate"`
	CriticalErrorRate float64  `json:"critical_error_rate"`
	TargetTTFTMs      int64    `json:"target_ttft_ms"`
	CriticalTTFTMs    int64    `json:"critical_ttft_ms"`
	WarningCacheRate  float64  `json:"warning_cache_rate"`
	CriticalCacheRate float64  `json:"critical_cache_rate"`
	ErrorWeight       float64  `json:"error_weight"`
	TTFTWeight        float64  `json:"ttft_weight"`
	CacheWeight       float64  `json:"cache_weight"`
	IgnoredCategories []string `json:"ignored_error_categories"`
}

// Config 渠道状态总配置
type Config struct {
	Enabled            bool             `json:"enabled"`
	AutoShowNewGroups  bool             `json:"auto_show_new_groups"`
	RefreshSeconds     int              `json:"refresh_seconds"`
	JudgeWindowMinutes int              `json:"judge_window_minutes"`
	DefaultGranularity Granularity      `json:"default_granularity"`
	Layout             []PlatformLayout `json:"layout"`
	Rules              Rules            `json:"rules"`
	Probe              Probe            `json:"probe"`
}

// DefaultRules 与上游 V2 默认值一致：最小样本 50、错误率严重 20%、
// TTFT 目标 3s / 严重 10s、权重 0.6/0.2/0.2、缓存默认不扣分（0/0）、分档 80/50
func DefaultRules() Rules {
	return Rules{
		UpMin: 80, WarnMin: 50, MinSample: 50, HistoryMinSample: 10,
		WarningErrorRate: 0.05, CriticalErrorRate: 0.20,
		TargetTTFTMs: 3000, CriticalTTFTMs: 10000,
		ErrorWeight: 0.60, TTFTWeight: 0.20, CacheWeight: 0.20,
	}
}

func DefaultConfig() Config {
	return Config{
		Enabled:            false, // 上线时管理员手动开启，并关闭上游 V1/V2
		AutoShowNewGroups:  false,
		RefreshSeconds:     60,
		JudgeWindowMinutes: 15,
		DefaultGranularity: GranHour,
		Layout:             []PlatformLayout{},
		Rules:              DefaultRules(),
		Probe:              DefaultProbe(),
	}
}

func (c *Config) Validate() error {
	switch c.RefreshSeconds {
	case 30, 60, 120, 300:
	default:
		return invalid("刷新周期只能是 30/60/120/300 秒")
	}
	switch c.JudgeWindowMinutes {
	case 5, 15, 30, 60:
	default:
		return invalid("判定窗口只能是 5/15/30/60 分钟")
	}
	if _, ok := ParseGranularity(string(c.DefaultGranularity)); !ok {
		return invalid("默认粒度只能是 hour/day/week")
	}
	seenPlat := map[string]bool{}
	seenGroup := map[int64]bool{}
	for _, p := range c.Layout {
		if p.Platform == "" {
			return invalid("平台标识不能为空")
		}
		if seenPlat[p.Platform] {
			return invalid("平台重复: %s", p.Platform)
		}
		seenPlat[p.Platform] = true
		for _, g := range p.Groups {
			if g.GroupID <= 0 {
				return invalid("分组 ID 无效")
			}
			if seenGroup[g.GroupID] {
				return invalid("分组重复: %d", g.GroupID)
			}
			seenGroup[g.GroupID] = true
		}
	}
	if err := c.Rules.Validate(); err != nil {
		return err
	}
	return c.Probe.Validate()
}

func (r *Rules) Validate() error {
	if r.WarnMin <= 0 || r.UpMin > 100 || r.WarnMin >= r.UpMin {
		return invalid("分档需满足 0 < 降级线 < 正常线 <= 100")
	}
	if r.MinSample < 1 || r.MinSample > 10000 {
		return invalid("最小样本需在 1-10000 之间")
	}
	if r.HistoryMinSample < 1 || r.HistoryMinSample > 10000 {
		return invalid("历史条最小样本需在 1-10000 之间")
	}
	if r.WarningErrorRate <= 0 || r.CriticalErrorRate < r.WarningErrorRate || r.CriticalErrorRate > 1 {
		return invalid("错误率阈值需满足 0 < 告警 <= 严重 <= 100%%")
	}
	if r.TargetTTFTMs <= 0 || r.CriticalTTFTMs <= r.TargetTTFTMs {
		return invalid("TTFT 需满足 0 < 目标 < 严重")
	}
	if r.WarningCacheRate < 0 || r.CriticalCacheRate < 0 || r.WarningCacheRate > 1 || r.CriticalCacheRate > r.WarningCacheRate {
		return invalid("缓存命中率阈值需满足 0 <= 严重 <= 告警 <= 100%%")
	}
	if r.ErrorWeight < 0 || r.TTFTWeight < 0 || r.CacheWeight < 0 {
		return invalid("权重不能为负")
	}
	if math.Abs(r.ErrorWeight+r.TTFTWeight+r.CacheWeight-1) > 0.001 {
		return invalid("权重之和必须等于 100%%")
	}
	allowed := make(map[string]bool, len(ErrorCategories))
	for _, c := range ErrorCategories {
		allowed[c] = true
	}
	for _, c := range r.IgnoredCategories {
		if !allowed[c] {
			return invalid("未知的错误分类: %s", c)
		}
	}
	return nil
}

// ErrorCategories 与上游 V2 错误分类（taxonomy v1）一致，用于"忽略错误分类"
var ErrorCategories = []string{
	"content_policy", "authentication", "context_limit", "invalid_request",
	"model_unsupported", "group_access", "quota_or_balance", "account_pool_unavailable",
	"rate_or_capacity", "timeout", "transport_or_stream", "upstream_forbidden",
	"not_found", "client_cancelled", "upstream_5xx", "internal", "other",
}

// ForHistory 返回用于历史条单格评分的规则：只把样本门槛换成 HistoryMinSample，其余口径不变
func (r Rules) ForHistory() Rules {
	if r.HistoryMinSample > 0 {
		r.MinSample = r.HistoryMinSample
	}
	return r
}
