package dispatch

import "fmt"

// 档位
const (
	ModeOff     = "off"     // 关闭：不检测、不判定，钩子原样放行
	ModeObserve = "observe" // 只观察：检测 + 判定 + 调度建议；调整保存但不生效
	ModeManual  = "manual"  // 人工调整：调度台里的调整立即生效
)

// Config 全局配置（单行 + 乐观锁）
type Config struct {
	Mode        string  `json:"mode"`
	Pref        string  `json:"pref"`         // 挪出来的流量优先给：quality | cost
	BadFactor   float64 `json:"bad_factor"`   // 参与调度的维度异常时份额 ×
	WarnFactor  float64 `json:"warn_factor"`  // 可疑时份额 ×
	MinShare    float64 `json:"min_share"`    // 被降的账号至少保留 %
	MaxMove     float64 `json:"max_move"`     // 每小时最多挪走全组 %
	Concurrency int     `json:"concurrency"`  // 主动检测的全局并发
	GapShare    float64 `json:"gap_share"`    // 模型占平台流量超过 % 却没被计划覆盖时提醒
	FaultRuns   int     `json:"fault_runs"`   // 检测连续出错几次提醒
	RestoreHint int     `json:"restore_hint"` // 调整依据消失满几个小时提示「建议恢复」
}

func DefaultConfig() Config {
	return Config{Mode: ModeObserve, Pref: "quality", BadFactor: 0.3, WarnFactor: 0.7, MinShare: 3, MaxMove: 30,
		Concurrency: 4, GapShare: 5, FaultRuns: 3, RestoreHint: 3}
}

func (c *Config) normalize() error {
	d := DefaultConfig()
	switch c.Mode {
	case ModeOff, ModeObserve, ModeManual:
	case "":
		c.Mode = d.Mode
	default:
		return fmt.Errorf("%w：档位只能是 off / observe / manual", ErrInvalid)
	}
	if c.Pref != "cost" {
		c.Pref = "quality"
	}
	if c.BadFactor <= 0 || c.BadFactor >= 1 {
		c.BadFactor = d.BadFactor
	}
	if c.WarnFactor <= 0 || c.WarnFactor >= 1 {
		c.WarnFactor = d.WarnFactor
	}
	if c.BadFactor > c.WarnFactor {
		return fmt.Errorf("%w：异常系数应小于等于可疑系数", ErrInvalid)
	}
	if c.MinShare < 0 || c.MinShare > 50 {
		c.MinShare = d.MinShare
	}
	if c.MaxMove <= 0 || c.MaxMove > 100 {
		c.MaxMove = d.MaxMove
	}
	if c.Concurrency <= 0 || c.Concurrency > 32 {
		c.Concurrency = d.Concurrency
	}
	if c.GapShare <= 0 || c.GapShare > 100 {
		c.GapShare = d.GapShare
	}
	if c.FaultRuns <= 0 {
		c.FaultRuns = d.FaultRuns
	}
	if c.RestoreHint <= 0 {
		c.RestoreHint = d.RestoreHint
	}
	return nil
}
