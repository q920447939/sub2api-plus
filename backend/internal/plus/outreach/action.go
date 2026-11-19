// Package outreach — 行动（Action）：邮件里唯一的按钮 {{cta_url}} 点了之后去哪。
//
// 邮件里的 {{cta_url}} 永远是跟踪链接 /r/:token；点击时由解析器决定真正的去向：
//   - 基础解析器由目的提供：verifyAction（一次性验证页）、landingAction（活动落地页）
//   - 附带权益时套一层 claimAction 装饰器：先到领取页，领完再去基础解析器给出的地址
//
// 这样写邮件的人只需要一个变量，不会再出现「回来看看」「领取权益」两个按钮。
package outreach

import (
	"errors"
	"net/url"
	"strings"
)

// ActionTarget 解析去向所需的信息
type ActionTarget struct {
	Base        string // 链接域名（不含结尾斜杠）
	Landing     string // 活动落地页（已规整；空 = 站点首页）
	Token       string // 收件人凭证
	InviteToken string // 验证邮箱：一次性验证 token
}

// ActionResolver 解析主按钮的最终去向
type ActionResolver interface {
	Resolve(t ActionTarget) (string, error)
}

type landingAction struct{}

func (landingAction) Resolve(t ActionTarget) (string, error) {
	return landingTarget(t.Base, t.Landing), nil
}

type verifyAction struct{}

var errNoInviteToken = errors.New("outreach: invite token missing")

func (verifyAction) Resolve(t ActionTarget) (string, error) {
	if t.InviteToken == "" {
		return "", errNoInviteToken
	}
	return t.Base + "/verify-email?token=" + url.QueryEscape(t.InviteToken), nil
}

// claimAction 附带权益时的按钮去向：领取页。
// 领完之后去哪不放在 URL 里（避免被改成外站跳板），由领取页调用 ClaimInfo，服务端用目的的基础解析器（actionFor(p, false)）算出 next。
// 领取与跳转是两步：发放在 POST /claim 的事务里完成（行锁 + claimed_at），落地页出错不影响、也不会触发重复领取。
type claimAction struct{}

func (claimAction) Resolve(t ActionTarget) (string, error) {
	return t.Base + "/outreach/claim?t=" + url.QueryEscape(t.Token), nil
}

// actionFor 组装某个目的 + 是否带权益的解析链
func actionFor(p Purpose, hasBenefits bool) ActionResolver {
	if hasBenefits {
		return claimAction{}
	}
	return p.Action()
}

// landingTarget 落地页：路径拼在链接域名后；完整地址原样使用（保存时已校验过域名）
func landingTarget(base, landing string) string {
	switch {
	case landing == "":
		return base + "/"
	case strings.HasPrefix(landing, "/"):
		return base + landing
	}
	return landing
}

// normalizeLanding 校验落地页：只允许本站路径，或域名属于 allowedBases 之一的 http(s) 地址，防止邮件链接被当成外站跳板
func normalizeLanding(raw string, allowedBases []string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	if len(s) > 500 {
		return "", badInput("落地页地址不超过 500 字符")
	}
	if strings.HasPrefix(s, "/") {
		if strings.HasPrefix(s, "//") || strings.ContainsAny(s, " \\") {
			return "", badInput("落地页路径格式不正确，例如 /pricing")
		}
		return s, nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", badInput("落地页填写本站路径（如 /pricing），或 http(s):// 开头的本站地址")
	}
	for _, b := range allowedBases {
		if bu, err := url.Parse(b); err == nil && bu.Host != "" && strings.EqualFold(bu.Host, u.Host) {
			return s, nil
		}
	}
	return "", badInput("落地页只能是本站或「触达链接域名」下的地址")
}
