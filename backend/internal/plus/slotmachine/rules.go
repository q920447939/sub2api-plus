// Package slotmachine — [plus] 摇机前置条件：规则注册表 + 责任链
//
// 新增一种前置条件只需：实现 Rule，写一个 RuleFactory，在 init 里 RegisterRule。
// 规则的启用与参数存在 Config.Rules（JSONB），管理端可直接增删改，无需改业务代码。
package slotmachine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// 内置规则类型
const (
	RuleRecharge   = "recharge"    // 近 N 天充值 ≥ M
	RuleDailyLimit = "daily_limit" // 每日摇机次数上限
	RuleCooldown   = "cooldown"    // 两次摇机最短间隔
)

// Facts 规则判断所需的用户事实，由 Service 按需懒加载；规则不直接依赖数据库
type Facts interface {
	Now() time.Time
	RecentRecharge(ctx context.Context, since time.Time) (float64, error)
	TodaySpinCount(ctx context.Context) (int, error)
	LastSpinAt(ctx context.Context) (time.Time, bool, error)
	CheckinStreak(ctx context.Context) (int, error)
}

// RuleError 规则未通过：Code 供前端/HTTP 层分流，Message 直接展示给用户
type RuleError struct {
	Rule       string
	Code       string
	Message    string
	RetryAfter time.Duration // 仅冷却类规则使用
}

func (e *RuleError) Error() string { return "slotmachine: rule " + e.Rule + " not met: " + e.Message }

// AsRuleError 便于调用方识别规则拒绝
func AsRuleError(err error) (*RuleError, bool) {
	var re *RuleError
	ok := errors.As(err, &re)
	return re, ok
}

// Rule 一条前置条件
type Rule interface {
	Type() string
	// Describe 面向用户的规则文案，如"近 30 天充值 ≥ $1"
	Describe() string
	// Check 通过返回 nil；不通过返回 *RuleError；其他 error 视为系统错误
	Check(ctx context.Context, f Facts) error
}

// RuleFactory 从配置参数构造规则，参数非法时返回 ErrInvalidConfig 包装的错误
type RuleFactory func(params map[string]any) (Rule, error)

// RuleMeta 注册信息，供管理端渲染可选规则与参数表单
type RuleMeta struct {
	Type    string      `json:"type"`
	Name    string      `json:"name"`
	Params  []ParamMeta `json:"params"`
	factory RuleFactory
}

// ParamMeta 参数描述。Kind 为空表示数值；"tier_list" 表示奖档下标数组（由 factory 自行校验）
type ParamMeta struct {
	Key     string  `json:"key"`
	Kind    string  `json:"kind,omitempty"`
	Label   string  `json:"label"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Step    float64 `json:"step"`
	Default float64 `json:"default"`
}

var (
	ruleMu       sync.RWMutex
	ruleRegistry = map[string]RuleMeta{}
)

// RegisterRule 注册规则类型；重复注册会 panic（属于编程错误）
func RegisterRule(meta RuleMeta, f RuleFactory) {
	ruleMu.Lock()
	defer ruleMu.Unlock()
	if _, dup := ruleRegistry[meta.Type]; dup {
		panic("slotmachine: duplicate rule type " + meta.Type)
	}
	meta.factory = f
	ruleRegistry[meta.Type] = meta
}

// RuleCatalog 已注册的规则类型（按类型名排序）
func RuleCatalog() []RuleMeta {
	ruleMu.RLock()
	defer ruleMu.RUnlock()
	list := make([]RuleMeta, 0, len(ruleRegistry))
	for _, m := range ruleRegistry {
		list = append(list, m)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Type < list[j].Type })
	return list
}

// BuildRule 按配置构造规则
func BuildRule(rc RuleConfig) (Rule, error) {
	ruleMu.RLock()
	meta, ok := ruleRegistry[rc.Type]
	ruleMu.RUnlock()
	if !ok {
		return nil, invalid("未知的前置条件类型: %s", rc.Type)
	}
	if err := checkParams(meta.Params, rc.Params); err != nil {
		return nil, err
	}
	return meta.factory(rc.Params)
}

// BuildRules 构造启用中的规则链，保持配置顺序
func BuildRules(cfgs []RuleConfig) ([]Rule, error) {
	chain := make([]Rule, 0, len(cfgs))
	for _, rc := range cfgs {
		if !rc.Enabled {
			continue
		}
		r, err := BuildRule(rc)
		if err != nil {
			return nil, err
		}
		chain = append(chain, r)
	}
	return chain, nil
}

// CheckRules 按顺序执行规则链，遇到第一个不通过的规则即返回
func CheckRules(ctx context.Context, chain []Rule, f Facts) error {
	for _, r := range chain {
		if err := r.Check(ctx, f); err != nil {
			return err
		}
	}
	return nil
}

// checkParams 按注册的参数描述校验范围；缺省值由各 factory 处理
func checkParams(metas []ParamMeta, params map[string]any) error {
	for _, pm := range metas {
		raw, ok := params[pm.Key]
		if !ok || pm.Kind != "" {
			continue
		}
		v, ok := toFloat(raw)
		if !ok || math.IsNaN(v) || v < pm.Min || v > pm.Max {
			return invalid("%s 需在 %g-%g 之间", pm.Label, pm.Min, pm.Max)
		}
	}
	return nil
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// param 读取数值参数，缺省时用 def
func param(params map[string]any, key string, def float64) float64 {
	if v, ok := toFloat(params[key]); ok {
		return v
	}
	return def
}

// ─── 内置规则 ────────────────────────────────────────────────────────────

type rechargeRule struct {
	windowDays int
	minAmount  float64
}

func (r rechargeRule) Type() string { return RuleRecharge }
func (r rechargeRule) Describe() string {
	return fmt.Sprintf("近 %d 天累计充值 ≥ $%g", r.windowDays, r.minAmount)
}
func (r rechargeRule) Check(ctx context.Context, f Facts) error {
	total, err := f.RecentRecharge(ctx, f.Now().AddDate(0, 0, -r.windowDays))
	if err != nil {
		return fmt.Errorf("recent recharge: %w", err)
	}
	if total+1e-9 < r.minAmount {
		return &RuleError{Rule: RuleRecharge, Code: "recharge_required",
			Message: fmt.Sprintf("需近 %d 天累计充值满 $%g 才能参与（当前 $%.2f）", r.windowDays, r.minAmount, total)}
	}
	return nil
}

type dailyLimitRule struct{ max int }

func (r dailyLimitRule) Type() string     { return RuleDailyLimit }
func (r dailyLimitRule) Describe() string { return fmt.Sprintf("每日最多摇 %d 次", r.max) }
func (r dailyLimitRule) Check(ctx context.Context, f Facts) error {
	n, err := f.TodaySpinCount(ctx)
	if err != nil {
		return fmt.Errorf("today spin count: %w", err)
	}
	if n >= r.max {
		return &RuleError{Rule: RuleDailyLimit, Code: "daily_limit",
			Message: fmt.Sprintf("今日 %d 次机会已用完，明天再来", r.max)}
	}
	return nil
}

type cooldownRule struct{ seconds int }

func (r cooldownRule) Type() string { return RuleCooldown }
func (r cooldownRule) Describe() string {
	return fmt.Sprintf("两次摇奖间隔 ≥ %d 秒", r.seconds)
}
func (r cooldownRule) Check(ctx context.Context, f Facts) error {
	last, ok, err := f.LastSpinAt(ctx)
	if err != nil {
		return fmt.Errorf("last spin: %w", err)
	}
	if !ok {
		return nil
	}
	wait := time.Duration(r.seconds)*time.Second - f.Now().Sub(last)
	if wait > 0 {
		return &RuleError{Rule: RuleCooldown, Code: "cooldown", RetryAfter: wait,
			Message: fmt.Sprintf("休息一下，%d 秒后可再摇", int(math.Ceil(wait.Seconds())))}
	}
	return nil
}

func init() {
	RegisterRule(RuleMeta{Type: RuleRecharge, Name: "充值门槛", Params: []ParamMeta{
		{Key: "window_days", Label: "统计天数", Min: 1, Max: 365, Step: 1, Default: 30},
		{Key: "min_amount", Label: "最低充值金额", Min: 0.01, Max: 100000, Step: 0.01, Default: 1},
	}}, func(p map[string]any) (Rule, error) {
		return rechargeRule{windowDays: int(param(p, "window_days", 30)), minAmount: param(p, "min_amount", 1)}, nil
	})
	RegisterRule(RuleMeta{Type: RuleDailyLimit, Name: "每日次数上限", Params: []ParamMeta{
		{Key: "max", Label: "每日次数", Min: 1, Max: 1000, Step: 1, Default: 5},
	}}, func(p map[string]any) (Rule, error) {
		return dailyLimitRule{max: int(param(p, "max", 5))}, nil
	})
	RegisterRule(RuleMeta{Type: RuleCooldown, Name: "摇奖冷却", Params: []ParamMeta{
		{Key: "seconds", Label: "间隔秒数", Min: 1, Max: 86400, Step: 1, Default: 3},
	}}, func(p map[string]any) (Rule, error) {
		return cooldownRule{seconds: int(param(p, "seconds", 3))}, nil
	})
}
