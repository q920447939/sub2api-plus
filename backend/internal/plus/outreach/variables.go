// Package outreach — 变量（组合模式）：每个来源只负责自己那几个变量。
//
// 编辑器的变量列表、渲染时的取值、保存时的「未知变量」校验，都由同一组 VariableSource 合并得出，
// 不会出现「列表里有、渲染不认」或「渲染认、校验报错」的不一致。
package outreach

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// VariableDoc 变量说明
type VariableDoc struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// RenderContext 渲染一封邮件所需的全部输入
type RenderContext struct {
	Purpose  Purpose
	Benefits []BenefitSpec
	Email    string
	User     RecipientVars
	SiteName string
	CTAURL   string
	UnsubURL string
}

// VariableSource 一组变量的提供者。Docs 依赖目的和权益：例如退订链接只对营销类可用。
type VariableSource interface {
	Docs(p Purpose, benefits []BenefitSpec) []VariableDoc
	Resolve(ctx context.Context, rc *RenderContext) map[string]string
}

// Variables 组合多个来源
type Variables []VariableSource

func (vs Variables) Docs(p Purpose, benefits []BenefitSpec) []VariableDoc {
	out := []VariableDoc{}
	for _, v := range vs {
		out = append(out, v.Docs(p, benefits)...)
	}
	return out
}

func (vs Variables) Resolve(ctx context.Context, rc *RenderContext) map[string]string {
	out := map[string]string{}
	for _, v := range vs {
		for k, val := range v.Resolve(ctx, rc) {
			out[k] = val
		}
	}
	return out
}

// Unknown 返回 texts 里出现、但当前目的 + 权益不提供的变量名（去重、排序）
func (vs Variables) Unknown(p Purpose, benefits []BenefitSpec, texts ...string) []string {
	known := map[string]bool{}
	for _, d := range vs.Docs(p, benefits) {
		known[d.Name] = true
	}
	return unknownIn(known, texts...)
}

func unknownIn(known map[string]bool, texts ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range texts {
		for _, m := range placeholderRe.FindAllStringSubmatch(t, -1) {
			if name := m[1]; !known[name] && !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	sort.Strings(out)
	return out
}

func wrapVars(names []string) string {
	w := make([]string, len(names))
	for i, n := range names {
		w[i] = "{{" + n + "}}"
	}
	return strings.Join(w, "、")
}

// ─── 内置来源 ────────────────────────────────────────────────────────────────

type userVars struct{}

func (userVars) Docs(Purpose, []BenefitSpec) []VariableDoc {
	return []VariableDoc{
		{"username", "用户名（没有则取邮箱前缀）"},
		{"email", "用户邮箱"},
		{"balance", "当前余额"},
		{"last_active_days", "距上次活跃的天数"},
	}
}

func (userVars) Resolve(_ context.Context, rc *RenderContext) map[string]string {
	name := strings.TrimSpace(rc.User.Username)
	if name == "" {
		name = localPart(rc.Email)
	}
	return map[string]string{
		"username": name, "email": rc.Email, "balance": rc.User.Balance,
		"last_active_days": fmt.Sprint(rc.User.LastActiveDays),
	}
}

// siteVars 外壳页脚也只能用这一组
type siteVars struct{}

func (siteVars) Docs(Purpose, []BenefitSpec) []VariableDoc {
	return []VariableDoc{{"site_name", "站点名称"}}
}

func (siteVars) Resolve(_ context.Context, rc *RenderContext) map[string]string {
	return map[string]string{"site_name": rc.SiteName}
}

// actionVars 主按钮与退订链接
type actionVars struct{}

func (actionVars) Docs(p Purpose, _ []BenefitSpec) []VariableDoc {
	out := []VariableDoc{{"cta_url", "主按钮链接：系统按目的自动决定去向（验证页 / 领取权益后到落地页 / 落地页）"}}
	if p != nil && p.Policy().Marketing() {
		out = append(out, VariableDoc{"unsubscribe_url", "退订链接（页脚已自动附带，一般不需要再写）"})
	}
	return out
}

func (actionVars) Resolve(_ context.Context, rc *RenderContext) map[string]string {
	return map[string]string{"cta_url": rc.CTAURL, "unsubscribe_url": rc.UnsubURL}
}

// purposeVars 委托给目的自己的变量
type purposeVars struct{}

func (purposeVars) Docs(p Purpose, b []BenefitSpec) []VariableDoc {
	if p == nil || p.Variables() == nil {
		return nil
	}
	return p.Variables().Docs(p, b)
}

func (purposeVars) Resolve(ctx context.Context, rc *RenderContext) map[string]string {
	if rc.Purpose == nil || rc.Purpose.Variables() == nil {
		return nil
	}
	return rc.Purpose.Variables().Resolve(ctx, rc)
}

// benefitVars 委托给已选权益
type benefitVars struct{ reg *BenefitRegistry }

func (b benefitVars) Docs(_ Purpose, specs []BenefitSpec) []VariableDoc {
	var out []VariableDoc
	for _, s := range specs {
		if impl, ok := b.reg.Get(s.Kind); ok {
			out = append(out, impl.Variables()...)
		}
	}
	return out
}

func (b benefitVars) Resolve(_ context.Context, rc *RenderContext) map[string]string {
	return b.reg.TemplateVars(rc.Benefits)
}

// verifyRewardVars 验证邮箱目的专属：奖励积分由邮箱验证模块提供
type verifyRewardVars struct{ v Verification }

func (verifyRewardVars) Docs(Purpose, []BenefitSpec) []VariableDoc {
	return []VariableDoc{{"verify_reward_points", "验证奖励积分数（在「邮箱验证」页配置，只发给充值过的用户）"}}
}

func (v verifyRewardVars) Resolve(ctx context.Context, _ *RenderContext) map[string]string {
	if v.v == nil {
		return map[string]string{"verify_reward_points": "0"}
	}
	return map[string]string{"verify_reward_points": fmt.Sprint(v.v.RewardPoints(ctx))}
}

// NewVariables 内置变量来源的组合
func NewVariables(benefits *BenefitRegistry) Variables {
	return Variables{userVars{}, siteVars{}, actionVars{}, purposeVars{}, benefitVars{reg: benefits}}
}
