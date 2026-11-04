// Package channelstatus — [plus] 兜底探测配置
package channelstatus

import (
	"net/url"
	"strings"
)

// ProbeFormat 探测请求格式（走本站网关，网关按 Key 所属分组路由）
type ProbeFormat string

const (
	FormatAuto            ProbeFormat = ""                 // 按分组平台自动选择
	FormatAnthropic       ProbeFormat = "anthropic"        // POST /v1/messages
	FormatOpenAIChat      ProbeFormat = "openai_chat"      // POST /v1/chat/completions
	FormatOpenAIResponses ProbeFormat = "openai_responses" // POST /v1/responses
	FormatGemini          ProbeFormat = "gemini"           // POST /v1beta/models/{model}:generateContent
)

// ResolveFormat 自动格式：按平台选最贴近用户真实调用的格式
func ResolveFormat(f ProbeFormat, platform string) ProbeFormat {
	if f != FormatAuto {
		return f
	}
	switch strings.ToLower(platform) {
	case "anthropic", "antigravity", "kiro":
		return FormatAnthropic
	case "openai":
		return FormatOpenAIResponses
	case "gemini":
		return FormatGemini
	default:
		return FormatOpenAIChat
	}
}

func validFormat(f ProbeFormat) bool {
	switch f {
	case FormatAuto, FormatAnthropic, FormatOpenAIChat, FormatOpenAIResponses, FormatGemini:
		return true
	}
	return false
}

// GroupProbe 单个分组的探测设置
type GroupProbe struct {
	GroupID  int64       `json:"group_id"`
	Enabled  bool        `json:"enabled"`
	APIKeyID int64       `json:"api_key_id"` // 必须是绑定到该分组的 Key（参考 V1「使用我的 Key」）
	Model    string      `json:"model"`
	Format   ProbeFormat `json:"format"`
}

// Probe 兜底探测全局设置。触发规则见 probe_policy.go。
type Probe struct {
	Enabled          bool         `json:"enabled"`
	Endpoint         string       `json:"endpoint"`              // 本站网关地址，如 https://api.example.com
	FreshMinutes     int          `json:"fresh_minutes"`         // 成功结果保质期
	RetryMinutes     int          `json:"retry_minutes"`         // 失败后复测间隔
	BaseCooldownMins int          `json:"base_cooldown_minutes"` // 无流量时冷却起点（指数退避）
	MaxCooldownHours int          `json:"max_cooldown_hours"`    // 冷却上限
	DailyLimit       int          `json:"daily_limit"`           // 每分组每日探测上限
	TimeoutSeconds   int          `json:"timeout_seconds"`
	Concurrency      int          `json:"concurrency"` // 单实例并发上限
	Groups           []GroupProbe `json:"groups"`
}

func DefaultProbe() Probe {
	return Probe{
		Enabled: false, FreshMinutes: 30, RetryMinutes: 5, BaseCooldownMins: 10, MaxCooldownHours: 6,
		DailyLimit: 24, TimeoutSeconds: 30, Concurrency: 2, Groups: []GroupProbe{},
	}
}

// GroupSetting 返回分组的探测设置
func (p *Probe) GroupSetting(id int64) (GroupProbe, bool) {
	for _, g := range p.Groups {
		if g.GroupID == id {
			return g, true
		}
	}
	return GroupProbe{}, false
}

func (p *Probe) Validate() error {
	between := func(v, lo, hi int, name string) error {
		if v < lo || v > hi {
			return invalid("%s需在 %d-%d 之间", name, lo, hi)
		}
		return nil
	}
	for _, chk := range []error{
		between(p.FreshMinutes, 5, 720, "探测结果保质期（分钟）"),
		between(p.RetryMinutes, 1, 120, "失败复测间隔（分钟）"),
		between(p.BaseCooldownMins, 1, 720, "冷却起点（分钟）"),
		between(p.MaxCooldownHours, 1, 48, "冷却上限（小时）"),
		between(p.DailyLimit, 1, 1440, "每日探测上限"),
		between(p.TimeoutSeconds, 5, 120, "单次探测超时（秒）"),
		between(p.Concurrency, 1, 16, "并发上限"),
	} {
		if chk != nil {
			return chk
		}
	}
	if p.BaseCooldownMins > p.MaxCooldownHours*60 {
		return invalid("冷却起点不能大于冷却上限")
	}
	p.Endpoint = strings.TrimRight(strings.TrimSpace(p.Endpoint), "/")
	if p.Endpoint != "" {
		u, err := url.Parse(p.Endpoint)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return invalid("网关地址格式不正确")
		}
	}
	seen := map[int64]bool{}
	anyEnabled := false
	for i := range p.Groups {
		g := &p.Groups[i]
		g.Model = strings.TrimSpace(g.Model)
		if g.GroupID <= 0 || seen[g.GroupID] {
			return invalid("探测分组无效或重复")
		}
		seen[g.GroupID] = true
		if !validFormat(g.Format) {
			return invalid("探测格式无效")
		}
		if g.Enabled {
			anyEnabled = true
			if g.APIKeyID <= 0 || g.Model == "" {
				return invalid("已启用探测的分组 #%d 需选择 Key 和模型", g.GroupID)
			}
		}
	}
	if p.Enabled && anyEnabled && p.Endpoint == "" {
		return invalid("启用探测需填写网关地址")
	}
	return nil
}
