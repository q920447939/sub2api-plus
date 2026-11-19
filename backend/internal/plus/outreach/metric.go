// Package outreach — 效果指标。
//
// 指标属于「目的」：每个目的声明自己关心哪些指标、口径是什么（如召回看「7 日内回访」，活动看「14 日内付费」），
// 第一个可用的是核心指标。口径写死在指标定义里，不放进任何设置：口径一变，前后活动就没法比较；
// 真要换口径就新增一个指标（key 不同），旧数据仍按旧口径解读。
//
// 效果只在「活动」层面看。批次是执行单元，只看投递结果（发送 / 失败 / 跳过 / 退信），见 DeliveryStats。
// 这些都是发送之后发生的行为，没有对照组，只能说明相关性。
package outreach

import (
	"fmt"
	"strings"
)

// 指标 key 前缀（带窗口的指标 key 形如 returned_7d）
const (
	MetricClicked  = "clicked"
	MetricVerified = "verified"
	MetricClaimed  = "claimed"
)

// MetricSpec 一个效果指标：口径固定
type MetricSpec struct {
	Key          string
	Label        string
	Desc         string
	NeedsCTA     bool // 邮件没有按钮时无意义
	NeedsBenefit bool // 没有附带权益时无意义
	// Cond 统计条件，可引用 r（收件人）、u（用户）、es（邮箱状态视图）
	Cond frag
}

// MetricView 给页面的一个指标
type MetricView struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Desc    string `json:"desc"`
	Value   int    `json:"value"`
	Primary bool   `json:"primary"`
}

// MetricRegistry 全部目的声明过的指标（按 key 去重），统计时一次算出所有列
type MetricRegistry struct {
	order []string
	m     map[string]MetricSpec
}

// NewMetricRegistry 收集所有目的的指标 + 权益领取
func NewMetricRegistry(purposes *PurposeRegistry) *MetricRegistry {
	r := &MetricRegistry{m: map[string]MetricSpec{}}
	for _, p := range purposes.All() {
		for _, s := range p.Metrics() {
			r.add(s)
		}
	}
	r.add(ClaimedMetric())
	return r
}

func (r *MetricRegistry) add(s MetricSpec) {
	if old, ok := r.m[s.Key]; ok {
		if old.Cond.sql != s.Cond.sql {
			panic("outreach: metric " + s.Key + " declared twice with different definitions")
		}
		return
	}
	r.m[s.Key] = s
	r.order = append(r.order, s.Key)
}

// Applicable 某封邮件（目的 + 是否带按钮 + 是否带权益）该看的指标，第一个是核心指标
func (r *MetricRegistry) Applicable(p Purpose, hasCTA, hasBenefits bool) []string {
	var keys []string
	for _, s := range p.Metrics() {
		keys = append(keys, s.Key)
	}
	if hasBenefits {
		keys = append(keys, MetricClaimed)
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		s, ok := r.m[k]
		if !ok || (s.NeedsCTA && !hasCTA) || (s.NeedsBenefit && !hasBenefits) {
			continue
		}
		out = append(out, k)
	}
	return out
}

// Views 把统计结果按 keys 的顺序转成页面指标
func (r *MetricRegistry) Views(keys []string, values map[string]int) []MetricView {
	out := make([]MetricView, 0, len(keys))
	for i, k := range keys {
		s := r.m[k]
		out = append(out, MetricView{Key: k, Label: s.Label, Desc: s.Desc, Value: values[k], Primary: i == 0})
	}
	return out
}

// selectCols 统计列：投递 7 列 + 每个指标一列，返回 SQL 片段和指标 key 顺序（与列顺序一致）
func (r *MetricRegistry) selectCols(q *query) (string, []string) {
	cols := []string{deliveryCols}
	keys := make([]string, 0, len(r.order))
	for _, k := range r.order {
		cols = append(cols, fmt.Sprintf("COUNT(*) FILTER (WHERE %s)", q.bind(r.m[k].Cond)))
		keys = append(keys, k)
	}
	return strings.Join(cols, ",\n\t\t"), keys
}

// deliveryCols 投递统计（活动与批次共用）
const deliveryCols = `COUNT(*),
		COUNT(*) FILTER (WHERE r.status IN ('queued','sending')),
		COUNT(*) FILTER (WHERE r.status = 'sent'),
		COUNT(*) FILTER (WHERE r.status = 'failed'),
		COUNT(*) FILTER (WHERE r.status = 'skipped'),
		COUNT(*) FILTER (WHERE r.bounced),
		COUNT(*) FILTER (WHERE r.unsub_at IS NOT NULL)`

const statsFrom = `FROM plus_outreach_recipients r
	LEFT JOIN users u ON u.id = r.user_id
	LEFT JOIN plus_email_status es ON es.user_id = r.user_id`

// DeliveryStats 投递结果（批次只看这个）
type DeliveryStats struct {
	Total   int `json:"total"`
	Queued  int `json:"queued"`
	Sent    int `json:"sent"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
	Bounced int `json:"bounced"`
	Unsub   int `json:"unsub"`
}

func (b *DeliveryStats) dest() []any {
	return []any{&b.Total, &b.Queued, &b.Sent, &b.Failed, &b.Skipped, &b.Bounced, &b.Unsub}
}

func toMap(keys []string, vals []int) map[string]int {
	m := make(map[string]int, len(keys))
	for i, k := range keys {
		m[k] = vals[i]
	}
	return m
}

// StatsView 活动效果：投递结果 + 该目的的效果指标
type StatsView struct {
	DeliveryStats
	Metrics []MetricView `json:"metrics"`
}

// ─── 指标定义 ────────────────────────────────────────────────────────────────

const lastSeenExpr = "GREATEST(u.last_login_at, u.last_active_at)"

// VerifiedMetric 发送后完成了邮箱验证（当前邮箱）
func VerifiedMetric() MetricSpec {
	return MetricSpec{Key: MetricVerified, Label: "完成邮箱验证", Desc: "发送后完成了邮箱验证",
		Cond: sqlf("r.status = 'sent' AND es.verified_at >= r.sent_at")}
}

// ClickedMetric 点过邮件里的按钮
func ClickedMetric() MetricSpec {
	return MetricSpec{Key: MetricClicked, Label: "点击按钮", Desc: "点过邮件里的按钮", NeedsCTA: true,
		Cond: sqlf("r.clicked_at IS NOT NULL")}
}

// ClaimedMetric 领取了附带权益
func ClaimedMetric() MetricSpec {
	return MetricSpec{Key: MetricClaimed, Label: "领取权益", Desc: "点按钮并登录后领取了附带权益", NeedsCTA: true, NeedsBenefit: true,
		Cond: sqlf("r.claimed_at IS NOT NULL")}
}

// ReturnedWithin 发送后 days 天内有登录或调用
func ReturnedWithin(days int) MetricSpec {
	return MetricSpec{
		Key: fmt.Sprintf("returned_%dd", days), Label: fmt.Sprintf("%d 日内回访", days),
		Desc: fmt.Sprintf("发送后 %d 天内有登录或调用", days),
		Cond: sqlf(fmt.Sprintf("r.status = 'sent' AND %[1]s > r.sent_at AND %[1]s <= r.sent_at + INTERVAL '%[2]d days'", lastSeenExpr, days)),
	}
}

// PaidWithin 发送后 days 天内有在线支付完成或兑换余额码
func PaidWithin(days int) MetricSpec {
	return MetricSpec{
		Key: fmt.Sprintf("paid_%dd", days), Label: fmt.Sprintf("%d 日内付费", days),
		Desc: fmt.Sprintf("发送后 %d 天内有在线支付完成或兑换余额码", days),
		Cond: sqlf(fmt.Sprintf(`r.status = 'sent' AND (
			EXISTS (SELECT 1 FROM payment_orders po WHERE po.user_id = r.user_id AND po.status = 'COMPLETED'
			        AND po.completed_at > r.sent_at AND po.completed_at <= r.sent_at + INTERVAL '%[1]d days')
			OR EXISTS (SELECT 1 FROM redeem_codes rc WHERE rc.used_by = r.user_id AND rc.type = 'balance' AND rc.value > 0
			        AND rc.used_at > r.sent_at AND rc.used_at <= r.sent_at + INTERVAL '%[1]d days'))`, days)),
	}
}
