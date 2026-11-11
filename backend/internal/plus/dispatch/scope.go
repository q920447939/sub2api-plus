package dispatch

import (
	"encoding/json"
	"fmt"
	"math"
)

// PlanScope 计划「测哪些账号」里选中的一条规则。计划可以选多条，账号命中任意一条就测。
type PlanScope struct {
	Type   string             `json:"type"`
	Params map[string]float64 `json:"params,omitempty"`
	IDs    []int64            `json:"account_ids,omitempty"`
}

// ScopeField 规则的一个参数（页面按它渲染输入框，不写死）
type ScopeField struct {
	Key     string  `json:"key"`
	Kind    string  `json:"kind"` // number | accounts
	Prefix  string  `json:"prefix,omitempty"`
	Suffix  string  `json:"suffix,omitempty"`
	Min     float64 `json:"min,omitempty"`
	Max     float64 `json:"max,omitempty"`
	Default float64 `json:"default,omitempty"`
}

// ScopeSpec 规则的声明
type ScopeSpec struct {
	Key    string       `json:"key"`
	Name   string       `json:"name"`
	Fields []ScopeField `json:"fields"`
}

// ScopeTarget 判断的对象：账号 × 模型
type ScopeTarget struct {
	Account  *CatAccount
	Model    string
	MaxShare float64 // 在所在各分组里最高的近 24 小时流量占比（%）
	Traffic  *TrafficSet
}

// ScopeRule 一种「测哪些账号」的规则（Strategy）。新增规则只需实现它并在 scopeRules 里登记，页面不用改。
type ScopeRule interface {
	Spec() ScopeSpec
	// Match 命中返回 true；没命中时返回原因
	Match(t ScopeTarget, s PlanScope) (bool, string)
}

var scopeRules = []ScopeRule{trafficScope{}, newScope{}, manualScope{}}

func scopeRule(key string) ScopeRule {
	for _, r := range scopeRules {
		if r.Spec().Key == key {
			return r
		}
	}
	return nil
}

// ScopeSpecs 所有规则的声明（给页面）
func ScopeSpecs() []ScopeSpec {
	out := make([]ScopeSpec, 0, len(scopeRules))
	for _, r := range scopeRules {
		out = append(out, r.Spec())
	}
	return out
}

// normalizeScopes 校验并补默认值；去重，同一种规则只留一条
func normalizeScopes(in []PlanScope) ([]PlanScope, error) {
	seen := map[string]bool{}
	out := make([]PlanScope, 0, len(in))
	for _, s := range in {
		r := scopeRule(s.Type)
		if r == nil {
			return nil, fmt.Errorf("%w：不认识的账号范围「%s」", ErrInvalid, s.Type)
		}
		if seen[s.Type] {
			continue
		}
		seen[s.Type] = true
		spec := r.Spec()
		ns := PlanScope{Type: s.Type}
		for _, f := range spec.Fields {
			switch f.Kind {
			case "number":
				v, ok := s.Params[f.Key]
				if !ok {
					v = f.Default
				}
				if v < f.Min || v > f.Max {
					return nil, fmt.Errorf("%w：「%s」要在 %g 到 %g 之间", ErrInvalid, spec.Name, f.Min, f.Max)
				}
				if ns.Params == nil {
					ns.Params = map[string]float64{}
				}
				ns.Params[f.Key] = v
			case "accounts":
				ns.IDs = uniqueIDs(s.IDs)
				if len(ns.IDs) == 0 {
					return nil, fmt.Errorf("%w：「%s」至少选一个账号", ErrInvalid, spec.Name)
				}
			}
		}
		out = append(out, ns)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w：至少选一种账号范围", ErrInvalid)
	}
	return out, nil
}

// scopeParam 计划里某条规则的参数（没选这条规则时 ok=false）
func scopeParam(scopes []PlanScope, typ, key string) (float64, bool) {
	for _, s := range scopes {
		if s.Type == typ {
			v, ok := s.Params[key]
			return v, ok
		}
	}
	return 0, false
}

// ---------- 按流量 ----------

type trafficScope struct{}

func (trafficScope) Spec() ScopeSpec {
	return ScopeSpec{Key: "traffic", Name: "有流量的账号", Fields: []ScopeField{
		{Key: "min_share", Kind: "number", Prefix: "流量占比 ≥", Suffix: "%", Min: 0, Max: 100, Default: 5},
	}}
}

func (trafficScope) Match(t ScopeTarget, s PlanScope) (bool, string) {
	a, min := t.Account, s.Params["min_share"]
	switch {
	case len(a.Groups) == 0:
		return false, "没有加入任何分组"
	case a.Status != "active":
		return false, "账号已停用"
	case !a.Serving():
		return false, "调度已关闭"
	case t.MaxShare < min:
		return false, fmt.Sprintf("流量占比 %.1f%%，低于 %g%%", t.MaxShare, min)
	case !accountModelTraffic(t.Traffic, a, t.Model):
		return false, "近 24 小时没有这个模型的流量"
	}
	return true, ""
}

// ---------- 新接入 ----------

type newScope struct{}

func (newScope) Spec() ScopeSpec {
	return ScopeSpec{Key: "new", Name: "新接入的账号", Fields: []ScopeField{
		{Key: "days", Kind: "number", Prefix: "创建", Suffix: "天内", Min: 1, Max: 30, Default: 3},
	}}
}

func (newScope) Match(t ScopeTarget, s PlanScope) (bool, string) {
	a, days := t.Account, int(math.Round(s.Params["days"]))
	switch {
	case a.Status != "active":
		return false, "账号已停用"
	case !a.IsNew(days, t.Traffic.NowAt):
		return false, fmt.Sprintf("创建已超过 %d 天", days)
	}
	return true, ""
}

// ---------- 指定账号 ----------

type manualScope struct{}

func (manualScope) Spec() ScopeSpec {
	return ScopeSpec{Key: "manual", Name: "指定账号", Fields: []ScopeField{{Key: "account_ids", Kind: "accounts"}}}
}

func (manualScope) Match(t ScopeTarget, s PlanScope) (bool, string) {
	for _, id := range s.IDs {
		if id == t.Account.ID {
			return true, ""
		}
	}
	return false, "不在指定账号里"
}

// scopesJSON 落库用
func scopesJSON(s []PlanScope) []byte {
	b, _ := json.Marshal(s)
	return b
}
