// Package outreach — 权益（Benefit）：随邮件附带、用户点击领取链接并登录后才发放的东西。
//
// 策略模式：每种权益实现 Benefit 接口并注册到 BenefitRegistry。活动 / 批次里只存
// [{kind, config}] 列表（可以挂多个），领取流程、批次、统计都不认识具体权益。
// 新增一种权益（余额、优惠码……）只需新增一个实现并在 module.go 注册，不改其他代码。
package outreach

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// BenefitSpec 活动 / 批次里保存的一条权益配置
type BenefitSpec struct {
	Kind   string          `json:"kind"`
	Config json.RawMessage `json:"config"`
}

// BenefitField 权益配置项的描述，前端据此通用渲染表单，不需要为每种权益写页面
type BenefitField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Type  string `json:"type"` // 目前只有 int
	Min   int    `json:"min"`
	Max   int    `json:"max"`
	Hint  string `json:"hint,omitempty"`
}

// GrantRequest 领取时的上下文
type GrantRequest struct {
	UserID     int64
	CampaignID int64
	BatchID    int64
}

// Benefit 一种权益的实现。Grant 在领取事务内调用，必须只通过 tx 写库，
// 这样「标记已领取」与「所有权益入账」同时成功或同时失败。
type Benefit interface {
	Kind() string
	Label() string
	Fields() []BenefitField
	Variables() []VariableDoc
	ValidateConfig(raw json.RawMessage) error
	// TemplateVars 返回该配置提供给邮件模板的变量值
	TemplateVars(raw json.RawMessage) map[string]string
	// Describe 给用户和管理员看的一句话描述，例如「200 积分」
	Describe(raw json.RawMessage) string
	Grant(ctx context.Context, tx DBTX, req GrantRequest, raw json.RawMessage) error
}

// BenefitRegistry 按 kind 查找权益实现；保持注册顺序以便前端稳定展示
type BenefitRegistry struct {
	order []string
	m     map[string]Benefit
}

func NewBenefitRegistry() *BenefitRegistry {
	return &BenefitRegistry{m: map[string]Benefit{}}
}

// Register 重复注册同一 kind 会 panic（属于装配期编程错误）
func (r *BenefitRegistry) Register(b Benefit) {
	if _, dup := r.m[b.Kind()]; dup {
		panic("outreach: duplicate benefit kind " + b.Kind())
	}
	r.m[b.Kind()] = b
	r.order = append(r.order, b.Kind())
}

func (r *BenefitRegistry) Get(kind string) (Benefit, bool) {
	b, ok := r.m[kind]
	return b, ok
}

// All 按注册顺序返回
func (r *BenefitRegistry) All() []Benefit {
	out := make([]Benefit, 0, len(r.order))
	for _, k := range r.order {
		out = append(out, r.m[k])
	}
	return out
}

// Validate 校验权益列表：kind 已注册、不重复、配置合法。顺便把 nil 配置规整为 {}。
func (r *BenefitRegistry) Validate(specs []BenefitSpec) error {
	seen := map[string]bool{}
	for i := range specs {
		s := &specs[i]
		b, ok := r.m[s.Kind]
		if !ok {
			return badInput(fmt.Sprintf("权益类型「%s」不可用", s.Kind))
		}
		if seen[s.Kind] {
			return badInput(fmt.Sprintf("权益「%s」重复添加", b.Label()))
		}
		seen[s.Kind] = true
		if len(s.Config) == 0 || string(s.Config) == "null" {
			s.Config = json.RawMessage(`{}`)
		}
		if err := b.ValidateConfig(s.Config); err != nil {
			return badInput(fmt.Sprintf("%s：%s", b.Label(), err.Error()))
		}
	}
	return nil
}

// TemplateVars 汇总所有权益提供的模板变量
func (r *BenefitRegistry) TemplateVars(specs []BenefitSpec) map[string]string {
	out := map[string]string{}
	for _, s := range specs {
		if b, ok := r.m[s.Kind]; ok {
			for k, v := range b.TemplateVars(s.Config) {
				out[k] = v
			}
		}
	}
	return out
}

// Describe 每条权益一句话描述
func (r *BenefitRegistry) Describe(specs []BenefitSpec) []string {
	out := make([]string, 0, len(specs))
	for _, s := range specs {
		if b, ok := r.m[s.Kind]; ok {
			out = append(out, b.Describe(s.Config))
		} else {
			out = append(out, s.Kind)
		}
	}
	return out
}

// GrantAll 在同一事务内依次发放所有权益；任何一条失败都返回错误，由调用方回滚
func (r *BenefitRegistry) GrantAll(ctx context.Context, tx DBTX, req GrantRequest, specs []BenefitSpec) ([]string, error) {
	granted := make([]string, 0, len(specs))
	for _, s := range specs {
		b, ok := r.m[s.Kind]
		if !ok {
			return nil, fmt.Errorf("outreach: benefit %q is not available", s.Kind)
		}
		if err := b.Grant(ctx, tx, req, s.Config); err != nil {
			return nil, fmt.Errorf("outreach: grant %s: %w", s.Kind, err)
		}
		granted = append(granted, b.Describe(s.Config))
	}
	return granted, nil
}

// MissingKinds 返回 specs 里当前未注册的权益 kind（模块不可用时用来拒绝执行批次）
func (r *BenefitRegistry) MissingKinds(specs []BenefitSpec) []string {
	var out []string
	for _, s := range specs {
		if _, ok := r.m[s.Kind]; !ok {
			out = append(out, s.Kind)
		}
	}
	sort.Strings(out)
	return out
}

// ─── 积分权益 ────────────────────────────────────────────────────────────────

// KindPointsGrant 领取积分
const KindPointsGrant = "points_grant"

// PointsGranter 把积分记入账户，必须使用传入的 tx
type PointsGranter interface {
	GrantPoints(ctx context.Context, tx DBTX, userID, campaignID, points int64) error
}

type pointsBenefit struct{ granter PointsGranter }

// NewPointsBenefit 创建积分权益
func NewPointsBenefit(g PointsGranter) Benefit { return &pointsBenefit{granter: g} }

type pointsConfig struct {
	Points int `json:"points"`
}

func (pointsBenefit) Kind() string  { return KindPointsGrant }
func (pointsBenefit) Label() string { return "赠送积分" }
func (pointsBenefit) Fields() []BenefitField {
	return []BenefitField{{Key: "points", Label: "积分数", Type: "int", Min: 1, Max: 1_000_000, Hint: "用户点击邮件按钮并登录后领取到账；每人每批次限领一次，「再发一轮」批次里已领过的人可以再领一次"}}
}
func (pointsBenefit) Variables() []VariableDoc {
	return []VariableDoc{{Name: "reward_points", Desc: "赠送的积分数"}}
}

func parsePoints(raw json.RawMessage) (pointsConfig, error) {
	var c pointsConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("配置格式错误")
	}
	return c, nil
}

func (pointsBenefit) ValidateConfig(raw json.RawMessage) error {
	c, err := parsePoints(raw)
	if err != nil {
		return err
	}
	if c.Points < 1 || c.Points > 1_000_000 {
		return fmt.Errorf("积分数需在 1 到 1000000 之间")
	}
	return nil
}

func (pointsBenefit) TemplateVars(raw json.RawMessage) map[string]string {
	c, _ := parsePoints(raw)
	return map[string]string{"reward_points": fmt.Sprint(c.Points)}
}

func (pointsBenefit) Describe(raw json.RawMessage) string {
	c, _ := parsePoints(raw)
	return fmt.Sprintf("%d 积分", c.Points)
}

func (p pointsBenefit) Grant(ctx context.Context, tx DBTX, req GrantRequest, raw json.RawMessage) error {
	c, err := parsePoints(raw)
	if err != nil {
		return err
	}
	return p.granter.GrantPoints(ctx, tx, req.UserID, req.CampaignID, int64(c.Points))
}

// ─── 工具 ────────────────────────────────────────────────────────────────────

func marshalBenefits(specs []BenefitSpec) []byte {
	if specs == nil {
		specs = []BenefitSpec{}
	}
	return marshalJSON(specs)
}

func parseBenefits(raw []byte) []BenefitSpec {
	out := []BenefitSpec{}
	_ = json.Unmarshal(raw, &out)
	return out
}
