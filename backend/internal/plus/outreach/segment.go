// Package outreach — 人群圈选（规约组合）：系统规则 ∧ 目的规则 ∧ 运营条件，翻译成 SQL 条件片段。
//
// 片段（frag）自带参数，占位符统一写成 `$?`；查询组装器（query）在绑定时按出现顺序编号为 $1..$n。
// 每条查询只绑定自己用到的片段，因此不会出现「参数表里有、SQL 里没用到」的错位。
//
// 资格分三层，语义不要混：
//   - 目标人群 target：运营条件（Segment）∩ 目的要求的邮箱状态 ∩ 邮箱有效 ∩ 未退订 ∩ 非管理员
//   - 批次可选 pickable：target 再排除「正在其他未结束批次里排队」，以及（scope=fresh 时）「本活动已发送过」
//   - 发送时校验：频控（recent）只在 worker 发信前判断，命中则跳过并写明原因，不影响名单生成
package outreach

import (
	"fmt"
	"strconv"
	"strings"
)

// frag 一段带参数的 SQL 条件
type frag struct {
	sql  string
	args []any
}

func sqlf(sql string, args ...any) frag { return frag{sql: sql, args: args} }

var (
	sqlTrue  = sqlf("TRUE")
	sqlFalse = sqlf("FALSE")
)

func and(fs ...frag) frag {
	parts := make([]string, 0, len(fs))
	var args []any
	for _, f := range fs {
		parts = append(parts, "("+f.sql+")")
		args = append(args, f.args...)
	}
	return frag{sql: strings.Join(parts, " AND "), args: args}
}

func or(fs ...frag) frag {
	parts := make([]string, 0, len(fs))
	var args []any
	for _, f := range fs {
		parts = append(parts, "("+f.sql+")")
		args = append(args, f.args...)
	}
	return frag{sql: strings.Join(parts, " OR "), args: args}
}

func not(f frag) frag { return frag{sql: "NOT (" + f.sql + ")", args: f.args} }

// query 组装一条 SQL 的参数表
type query struct{ args []any }

// bind 把片段写入 SQL：按顺序把 `$?` 替换成真实编号，并追加对应参数
func (q *query) bind(f frag) string {
	parts := strings.Split(f.sql, "$?")
	if len(parts)-1 != len(f.args) {
		panic(fmt.Sprintf("outreach: frag has %d placeholders but %d args: %s", len(parts)-1, len(f.args), f.sql))
	}
	var b strings.Builder
	for i, p := range parts {
		b.WriteString(p)
		if i < len(f.args) {
			q.args = append(q.args, f.args[i])
			fmt.Fprintf(&b, "$%d", len(q.args))
		}
	}
	return b.String()
}

// arg 追加一个独立参数
func (q *query) arg(v any) string {
	q.args = append(q.args, v)
	return fmt.Sprintf("$%d", len(q.args))
}

// 批次选人范围
const (
	ScopeFresh = "fresh" // 只发给本活动还没发送过的人（默认，适合灰度放量）
	ScopeAgain = "again" // 再发一轮：已发送过的人也纳入（仍受发送时频控约束）
)

func validScope(s string) bool { return s == ScopeFresh || s == ScopeAgain }

const activityExpr = "GREATEST(u.last_active_at, u.last_login_at, u.created_at)"

// 邮箱验证状态只读 plus_email_status 视图（邮箱验证模块的对外契约），不碰它的表
const audienceFrom = `FROM users u LEFT JOIN plus_email_status es ON es.user_id = u.id`

// audience 圈选用到的全部条件片段
type audience struct {
	base    frag // 正常用户 + 非管理员 + Segment 条件
	valid   frag // 邮箱未标无效
	state   frag // 目的要求的邮箱状态
	unsub   frag // 已退订该目的
	recent  frag // 频控窗口内收过邮件（发送时校验）
	reached frag // 本活动已发送过（读台账，批次清理后依然有效）
	queued  frag // 正在本活动未结束的批次里排队
}

// target 目标人群
func (a audience) target() frag { return and(a.base, a.valid, a.state, not(a.unsub)) }

// pickable 生成批次时可进入名单的人
func (a audience) pickable(scope string) frag {
	f := and(a.target(), not(a.queued))
	if scope != ScopeAgain {
		f = and(f, not(a.reached))
	}
	return f
}

// segmentConds 运营条件：每一条同时给出 SQL 和可读描述，保证「显示的规则」就是「执行的规则」
func segmentConds(sg Segment) ([]frag, []string) {
	var fs []frag
	var txt []string
	switch sg.Recharged {
	case "yes":
		fs, txt = append(fs, sqlf("u.total_recharged > 0")), append(txt, "充值过")
	case "no":
		fs, txt = append(fs, sqlf("u.total_recharged = 0")), append(txt, "没充值过")
	}
	if sg.InactiveDays > 0 {
		fs = append(fs, sqlf(activityExpr+" < NOW() - make_interval(days => $?)", sg.InactiveDays))
		txt = append(txt, fmt.Sprintf("%d 天以上没有登录或调用", sg.InactiveDays))
	}
	if sg.MinBalance > 0 {
		fs = append(fs, sqlf("u.balance > $?", sg.MinBalance))
		txt = append(txt, "余额大于 "+strconv.FormatFloat(sg.MinBalance, 'f', -1, 64))
	}
	if sg.RegisteredDays > 0 {
		fs = append(fs, sqlf("u.created_at <= NOW() - make_interval(days => $?)", sg.RegisteredDays))
		txt = append(txt, fmt.Sprintf("注册满 %d 天", sg.RegisteredDays))
	}
	return fs, txt
}

// describeAudience 完整人群规则（给运营看）：系统规则 + 目的规则 + 运营条件 + 退订
func describeAudience(p Purpose, sg Segment) []string {
	_, txt := segmentConds(sg)
	out := []string{"普通用户（不含管理员），账号正常，邮箱未被标记无效", p.State().Text}
	if len(txt) == 0 {
		out = append(out, "运营条件：不限")
	} else {
		out = append(out, "运营条件："+strings.Join(txt, "，"))
	}
	if p.Policy().Marketing() {
		out = append(out, fmt.Sprintf("没有退订「%s」类邮件", p.Label()))
	}
	return out
}

// buildAudience campaignID=0 时 reached / queued 恒为 FALSE（新建活动尚无历史）
func buildAudience(p Purpose, sg Segment, cfg DeliveryConfig, campaignID int64) audience {
	conds, _ := segmentConds(sg)
	conds = append([]frag{sqlf("u.deleted_at IS NULL"), sqlf("u.status = 'active'"), sqlf("u.role <> 'admin'")}, conds...)

	a := audience{
		base: and(conds...), valid: sqlf("es.invalid_at IS NULL"), state: p.State().frag(),
		unsub: sqlFalse, reached: sqlFalse, queued: sqlFalse,
	}
	if p.Policy().Marketing() {
		a.unsub = sqlf(`EXISTS (SELECT 1 FROM plus_outreach_unsubscribes x WHERE x.user_id = u.id AND x.purpose = $?)`, p.Key())
	}
	a.recent = recentFrag(p, cfg)

	if campaignID > 0 {
		a.reached = sqlf(`EXISTS (SELECT 1 FROM plus_outreach_ledger l WHERE l.campaign_id = $? AND l.user_id = u.id)`, campaignID)
		a.queued = sqlf(`EXISTS (SELECT 1 FROM plus_outreach_recipients r JOIN plus_outreach_batches b ON b.id = r.batch_id
			WHERE r.campaign_id = $? AND r.user_id = u.id
			  AND r.status IN ('queued','sending') AND b.deleted_at IS NULL AND b.status IN ('pending','running','paused'))`, campaignID)
	}
	return a
}

// recentFrag 频控：同目的间隔（每个目的可配置）OR 营销类之间的统一间隔
func recentFrag(p Purpose, cfg DeliveryConfig) frag {
	var parts []frag
	if d := gapDays(p, cfg.PurposeGapDays); d > 0 {
		parts = append(parts, sqlf(`EXISTS (SELECT 1 FROM plus_outreach_ledger l
			WHERE l.user_id = u.id AND l.purpose = $? AND l.sent_at > NOW() - make_interval(days => $?))`, p.Key(), d))
	}
	if p.Policy().Marketing() && cfg.MarketingGapDays > 0 {
		parts = append(parts, sqlf(`EXISTS (SELECT 1 FROM plus_outreach_ledger l
			WHERE l.user_id = u.id AND l.category = 'marketing' AND l.sent_at > NOW() - make_interval(days => $?))`, cfg.MarketingGapDays))
	}
	switch len(parts) {
	case 0:
		return sqlFalse
	case 1:
		return parts[0]
	}
	return or(parts...)
}
