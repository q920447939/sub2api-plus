// Package outreach — 目的（Purpose）：这类邮件为什么发、发给什么状态的人、按钮去哪、怎么算成功。
//
// 策略 + 注册表：运营只选「目的」，合规类别、邮箱状态要求、主行动、核心指标、专属变量都由目的声明。
// 人群、渲染、发送、统计、页面都只通过 Purpose 接口取这些信息，不再出现 `if kind == ...`。
// 新增一种目的只需实现 Purpose 并在 module.go 注册。
package outreach

import "fmt"

// 内置目的
const (
	PurposeVerifyEmail = "verify_email"
	PurposeRecall      = "recall"
	PurposeNews        = "news"
	PurposePromo       = "promo"
)

// 合规类别
const (
	CategoryService   = "service"   // 与账号相关的服务邮件：不可退订
	CategoryMarketing = "marketing" // 营销邮件：可退订，受跨目的营销频控
)

// Policy 合规策略
type Policy struct {
	Category string `json:"category"`
}

// Marketing 营销类：页脚强制带退订、受营销频控
func (p Policy) Marketing() bool { return p.Category == CategoryMarketing }

// StateRule 目的对邮箱状态的要求
type StateRule struct {
	Verified bool   // true = 必须已验证；false = 必须未验证
	Text     string // 给运营看的描述
}

func (s StateRule) frag() frag {
	if s.Verified {
		return sqlf("es.verified_at IS NOT NULL")
	}
	return sqlf("es.verified_at IS NULL")
}

// skip 发送时邮箱状态已不满足的原因；满足返回 ""
func (s StateRule) skip(verified bool) string {
	switch {
	case s.Verified && !verified:
		return "邮箱未验证"
	case !s.Verified && verified:
		return "用户已完成验证"
	}
	return ""
}

// Purpose 一种邮件目的
type Purpose interface {
	Key() string
	Label() string
	Description() string
	Policy() Policy
	State() StateRule
	// DefaultGapDays 同一用户两封同目的邮件的默认最小间隔，可在「发送设置」覆盖
	DefaultGapDays() int
	// RequiresCTA 正文是否必须带 {{cta_url}}（附带权益时无论目的都必须带）
	RequiresCTA() bool
	AllowsBenefits() bool
	// AllowsLanding 是否可以配置落地页（验证邮箱的按钮固定去验证页）
	AllowsLanding() bool
	// Action 主按钮的去向
	Action() ActionResolver
	// Metrics 该目的关心的效果指标（含口径），按重要性排序，第一个可用的就是核心指标
	Metrics() []MetricSpec
	// Variables 目的专属变量；没有返回 nil
	Variables() VariableSource
}

// purposeDef 用数据描述一个目的；行为差异由 action / vars 两个策略对象承担
type purposeDef struct {
	key, label, desc string
	policy           Policy
	state            StateRule
	gap              int
	requiresCTA      bool
	benefits         bool
	landing          bool
	action           ActionResolver
	metrics          []MetricSpec
	vars             VariableSource
}

func (p *purposeDef) Key() string               { return p.key }
func (p *purposeDef) Label() string             { return p.label }
func (p *purposeDef) Description() string       { return p.desc }
func (p *purposeDef) Policy() Policy            { return p.policy }
func (p *purposeDef) State() StateRule          { return p.state }
func (p *purposeDef) DefaultGapDays() int       { return p.gap }
func (p *purposeDef) RequiresCTA() bool         { return p.requiresCTA }
func (p *purposeDef) AllowsBenefits() bool      { return p.benefits }
func (p *purposeDef) AllowsLanding() bool       { return p.landing }
func (p *purposeDef) Action() ActionResolver    { return p.action }
func (p *purposeDef) Metrics() []MetricSpec     { return p.metrics }
func (p *purposeDef) Variables() VariableSource { return p.vars }

var (
	verifiedState   = StateRule{Verified: true, Text: "邮箱已验证"}
	unverifiedState = StateRule{Verified: false, Text: "邮箱未验证"}
	marketingPolicy = Policy{Category: CategoryMarketing}
)

// NewVerifyEmailPurpose 验证邮箱：服务类，只发未验证用户，按钮去一次性验证页
func NewVerifyEmailPurpose(vars VariableSource) Purpose {
	return &purposeDef{
		key: PurposeVerifyEmail, label: "验证邮箱",
		desc:   "服务邮件：只发给邮箱未验证的用户，按钮直达一次性验证页；不可退订",
		policy: Policy{Category: CategoryService}, state: unverifiedState, gap: 3,
		requiresCTA: true, action: verifyAction{},
		metrics: []MetricSpec{VerifiedMetric(), ClickedMetric()},
		vars:    vars,
	}
}

// NewRecallPurpose 召回沉默用户
func NewRecallPurpose() Purpose {
	return &purposeDef{
		key: PurposeRecall, label: "召回",
		desc:   "营销邮件：唤回一段时间没来的用户，核心看 7 日内回访",
		policy: marketingPolicy, state: verifiedState, gap: 30,
		requiresCTA: true, benefits: true, landing: true, action: landingAction{},
		metrics: []MetricSpec{ReturnedWithin(7), ClickedMetric(), PaidWithin(14)},
	}
}

// NewNewsPurpose 资讯通知：可以没有按钮
func NewNewsPurpose() Purpose {
	return &purposeDef{
		key: PurposeNews, label: "资讯",
		desc:   "营销邮件：新功能、公告；核心看点击，没有按钮时看 7 日内回访",
		policy: marketingPolicy, state: verifiedState, gap: 0,
		benefits: true, landing: true, action: landingAction{},
		metrics: []MetricSpec{ClickedMetric(), ReturnedWithin(7)},
	}
}

// NewPromoPurpose 促销活动
func NewPromoPurpose() Purpose {
	return &purposeDef{
		key: PurposePromo, label: "活动",
		desc:   "营销邮件：限时优惠、充值活动，核心看 14 日内付费",
		policy: marketingPolicy, state: verifiedState, gap: 0,
		requiresCTA: true, benefits: true, landing: true, action: landingAction{},
		metrics: []MetricSpec{PaidWithin(14), ClickedMetric(), ReturnedWithin(7)},
	}
}

// PurposeRegistry 按 key 查找目的，保持注册顺序
type PurposeRegistry struct {
	order []string
	m     map[string]Purpose
}

func NewPurposeRegistry() *PurposeRegistry { return &PurposeRegistry{m: map[string]Purpose{}} }

func (r *PurposeRegistry) Register(p Purpose) {
	if _, dup := r.m[p.Key()]; dup {
		panic("outreach: duplicate purpose " + p.Key())
	}
	r.m[p.Key()] = p
	r.order = append(r.order, p.Key())
}

func (r *PurposeRegistry) Get(key string) (Purpose, bool) {
	p, ok := r.m[key]
	return p, ok
}

// Lookup 读库得到的目的应当已注册（写入时校验过）；未注册说明代码删了目的而数据还在
func (r *PurposeRegistry) Lookup(key string) (Purpose, error) {
	if p, ok := r.m[key]; ok {
		return p, nil
	}
	return nil, badInput(fmt.Sprintf("邮件目的「%s」不可用", key))
}

func (r *PurposeRegistry) All() []Purpose {
	out := make([]Purpose, 0, len(r.order))
	for _, k := range r.order {
		out = append(out, r.m[k])
	}
	return out
}

// gapDays 同目的最小间隔：设置里覆盖了就用设置，否则用目的默认值
func gapDays(p Purpose, overrides map[string]int) int {
	if d, ok := overrides[p.Key()]; ok {
		return d
	}
	return p.DefaultGapDays()
}
