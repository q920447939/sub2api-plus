package dispatch

import (
	"math"
	"sort"
)

// 权重的业务模型：
//   流量在「分组里的一个模型」（一条线路）上分，候选 = 这个分组里调度开着、支持这个模型的账号。
//   规则 = { 账号, 分组, 模型（空 = 这个分组里它的全部模型）, share, hard }，同一个分组里模型上的规则优先。
//   没有规则 = 正常：按原来的规则分；share = 0 暂停；hard = 连只有它能跑的线路也停。
//
// 预计占比（Lanes）从「现在线上的规则」(base) 和近 24 小时实际占比推：
//   规则没变 → 就是实际占比；
//   变了 → 设了比例的拿走各自的比例，没调过的按原来的比例分剩下的；只剩设了比例的就按比例分全部；
//         只剩被暂停的：没勾「一起停掉」的保留，否则这条线路没有账号可用。

// Lanes 预计用的只读数据（一次请求内复用）
type Lanes struct {
	cat  *Catalog
	tr   *TrafficSet
	base []Rule

	models map[int64][]laneModel
	shares map[laneKey]laneShare
}

type laneModel struct {
	Model string
	N     int64
}

type laneKey struct {
	Group int64
	Model string
}

type laneShare struct {
	pct   map[int64]float64
	total int64
}

// NewLanes base = 现在线上生效的规则
func NewLanes(cat *Catalog, tr *TrafficSet, base []Rule) *Lanes {
	return &Lanes{cat: cat, tr: tr, base: base, models: map[int64][]laneModel{}, shares: map[laneKey]laneShare{}}
}

// Serving 分组里调度开着的账号（按 id）
func (l *Lanes) Serving(groupID int64) []*CatAccount {
	g := l.cat.Groups[groupID]
	if g == nil {
		return nil
	}
	var out []*CatAccount
	for _, id := range g.Accounts {
		if a := l.cat.Accounts[id]; a != nil && a.Serving() {
			out = append(out, a)
		}
	}
	return out
}

func (l *Lanes) candidates(groupID int64, model string) []*CatAccount {
	var out []*CatAccount
	for _, a := range l.Serving(groupID) {
		if a.Supports(model) {
			out = append(out, a)
		}
	}
	return out
}

// GroupModels 分组近 24 小时有流量的模型（按请求数从多到少）
func (l *Lanes) GroupModels(groupID int64) []laneModel {
	if ms, ok := l.models[groupID]; ok {
		return ms
	}
	per := map[string]int64{}
	l.tr.Each(TFilter{Group: groupID, From: l.tr.NowBucket() - 23*3600}, func(r *TRow) {
		if a := l.cat.Accounts[r.Account]; a != nil && a.Serving() {
			per[r.Model] += r.Total()
		}
	})
	ms := make([]laneModel, 0, len(per))
	for m, n := range per {
		if n > 0 {
			ms = append(ms, laneModel{m, n})
		}
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].N > ms[j].N || (ms[i].N == ms[j].N && ms[i].Model < ms[j].Model) })
	l.models[groupID] = ms
	return ms
}

// observed 线路上各账号近 24 小时的实际占比
func (l *Lanes) observed(groupID int64, model string) laneShare {
	k := laneKey{groupID, model}
	if s, ok := l.shares[k]; ok {
		return s
	}
	per := map[int64]int64{}
	var sum int64
	l.tr.Each(TFilter{Group: groupID, Model: model, From: l.tr.NowBucket() - 23*3600}, func(r *TRow) {
		if a := l.cat.Accounts[r.Account]; a != nil && a.Serving() {
			per[r.Account] += r.Total()
			sum += r.Total()
		}
	})
	s := laneShare{pct: map[int64]float64{}, total: sum}
	for id, n := range per {
		if sum > 0 {
			s.pct[id] = float64(n) * 100 / float64(sum)
		}
	}
	l.shares[k] = s
	return s
}

// ruleOf 同一个分组里模型上的规则优先于「全部模型」
func ruleOf(rules []Rule, groupID, accountID int64, model string) *Rule {
	var all *Rule
	for i := range rules {
		r := &rules[i]
		if r.AccountID != accountID || r.GroupID != groupID {
			continue
		}
		if r.Model == model && model != "" {
			return r
		}
		if r.Model == "" {
			all = r
		}
	}
	return all
}

// RuleChange 改一条规则；Share = nil 表示去掉（恢复正常）
type RuleChange struct {
	AccountID int64
	GroupID   int64
	Model     string
	Share     *float64
	Hard      bool
}

func setRules(rules []Rule, set []RuleChange) []Rule {
	hit := func(r Rule) bool {
		for _, c := range set {
			if c.AccountID == r.AccountID && c.GroupID == r.GroupID && c.Model == r.Model {
				return true
			}
		}
		return false
	}
	out := make([]Rule, 0, len(rules)+len(set))
	for _, r := range rules {
		if !hit(r) {
			out = append(out, r)
		}
	}
	for _, c := range set {
		if c.Share != nil {
			out = append(out, Rule{AccountID: c.AccountID, GroupID: c.GroupID, Model: c.Model, Share: *c.Share, Hard: c.Hard})
		}
	}
	return out
}

// setShare 在一个分组里把某个账号设成 x：同组里别的设过比例的账号按比例让出（或补上）空间，没调过的账号接走剩下的
func (l *Lanes) setShare(rules []Rule, accountID int64, model string, groupID int64, x float64) []Rule {
	out := setRules(rules, []RuleChange{{AccountID: accountID, GroupID: groupID, Model: model, Share: &x}})
	if x <= 0 {
		return out
	}
	mine := func(r Rule) bool { return r.GroupID == groupID && r.Model == model }
	var others []Rule
	sum := 0.0
	set := map[int64]bool{}
	for _, r := range out {
		if mine(r) {
			set[r.AccountID] = true
			if r.AccountID != accountID && r.Share > 0 {
				others = append(others, r)
				sum += r.Share
			}
		}
	}
	free := 0
	for _, a := range l.Serving(groupID) {
		if a.ID != accountID && !set[a.ID] {
			free++
		}
	}
	room := 1 - x
	if len(others) == 0 || (sum <= room && free > 0) {
		return out
	}
	var ch []RuleChange
	for _, r := range others {
		c := RuleChange{AccountID: r.AccountID, GroupID: groupID, Model: model}
		if room > 0.005 {
			v := room * r.Share / sum
			c.Share = &v
		}
		ch = append(ch, c)
	}
	return setRules(out, ch)
}

// LaneRow 一个账号在线路上的预计占比
type LaneRow struct {
	ID  int64   `json:"account_id"`
	Pct float64 `json:"pct"`
}

type laneSplit struct {
	rows  []LaneRow
	total int64
	only  bool // 没有别的账号：设了比例的分全部
	kept  bool // 只剩被暂停的，没勾一起停掉的保留
	dead  bool // 这条线路没有账号可用
}

func sameRule(x, y *Rule) bool {
	return (x == nil && y == nil) || (x != nil && y != nil && math.Abs(x.Share-y.Share) < 1e-9 && x.Hard == y.Hard)
}

// split 一条线路在给定规则下各账号预计占多少
func (l *Lanes) split(groupID int64, model string, rules []Rule) laneSplit {
	cand := l.candidates(groupID, model)
	cur := l.observed(groupID, model)
	unchanged := true
	for _, a := range cand {
		if !sameRule(ruleOf(rules, groupID, a.ID, model), ruleOf(l.base, groupID, a.ID, model)) {
			unchanged = false
			break
		}
	}
	if unchanged {
		rows := make([]LaneRow, 0, len(cand))
		for _, a := range cand {
			rows = append(rows, LaneRow{a.ID, cur.pct[a.ID]})
		}
		return laneSplit{rows: rows, total: cur.total}
	}
	type rr struct {
		id   int64
		nat  float64 // 还原到没有规则时的占比
		rule *Rule
		pct  float64
	}
	rs := make([]*rr, 0, len(cand))
	natSum := 0.0
	for _, a := range cand {
		s := cur.pct[a.ID]
		if b := ruleOf(l.base, groupID, a.ID, model); b != nil && b.Share > 0 {
			s /= b.Share
		}
		rs = append(rs, &rr{id: a.ID, nat: s, rule: ruleOf(rules, groupID, a.ID, model)})
		natSum += s
	}
	for _, r := range rs {
		if natSum > 0 {
			r.nat = r.nat * 100 / natSum
		} else {
			r.nat = 100 / float64(len(rs))
		}
	}
	var T, P, U []*rr
	want := 0.0
	for _, r := range rs {
		switch {
		case r.rule == nil:
			U = append(U, r)
		case r.rule.Share > 0:
			T = append(T, r)
			want += r.rule.Share
		default:
			P = append(P, r)
		}
	}
	got := 0.0
	for _, r := range T {
		r.pct = r.rule.Share / math.Max(want, 1) * 100
		got += r.pct
	}
	out := laneSplit{total: cur.total}
	spread := func(list []*rr, total float64) {
		base := 0.0
		for _, r := range list {
			base += r.nat
		}
		for _, r := range list {
			if base > 0 {
				r.pct = total * r.nat / base
			} else {
				r.pct = total / float64(len(list))
			}
		}
	}
	switch {
	case len(U) > 0:
		spread(U, 100-got)
	case len(T) > 0:
		for _, r := range T {
			r.pct = r.pct * 100 / got
		}
		out.only = true
	default:
		var soft []*rr
		for _, r := range P {
			if !r.rule.Hard {
				soft = append(soft, r)
			}
		}
		if len(soft) > 0 {
			spread(soft, 100)
			out.kept = true
		} else {
			out.dead = true
		}
	}
	for _, r := range rs {
		out.rows = append(out.rows, LaneRow{r.id, r.pct})
	}
	return out
}

// GroupView 账号在一个分组里的情况（占比按这个分组的全部流量算，同组各账号加起来是 100%）
type GroupView struct {
	GroupID int64     `json:"group_id"`
	Total   int64     `json:"total"`  // 它能接的线路近 24 小时的请求数
	Pct     float64   `json:"pct"`    // 它承接多少
	Locked  float64   `json:"locked"` // 其中只有它能接的（挪不走）
	Reach   float64   `json:"reach"`  // 它最多能接多少（有的模型它不提供）
	Alone   []string  `json:"alone"`  // 只有它在跑的模型
	Dead    []string  `json:"dead"`   // 这样设以后没有账号可用的模型
	Kept    []string  `json:"kept"`   // 暂停了但只有它能跑、照常保留的模型
	Rows    []LaneRow `json:"rows"`   // 同组各账号
}

// GroupView model 为空 = 这个分组里的全部模型
func (l *Lanes) GroupView(accountID int64, model string, groupID int64, rules []Rule) GroupView {
	per := map[int64]float64{}
	for _, a := range l.Serving(groupID) {
		per[a.ID] = 0
	}
	v := GroupView{GroupID: groupID, Alone: []string{}, Dead: []string{}, Kept: []string{}}
	var sum, locked, reach float64
	for _, m := range l.GroupModels(groupID) {
		if model != "" && m.Model != model {
			continue
		}
		sp := l.split(groupID, m.Model, rules)
		n := float64(m.N)
		for _, r := range sp.rows {
			if r.ID == accountID {
				reach += n
				v.Total += m.N
				if len(sp.rows) == 1 {
					locked += n
					v.Alone = append(v.Alone, m.Model)
				}
				if sp.dead {
					v.Dead = append(v.Dead, m.Model)
				}
				if sp.kept {
					v.Kept = append(v.Kept, m.Model)
				}
			}
			per[r.ID] += r.Pct * n / 100
		}
		sum += n
	}
	p := func(x float64) float64 {
		if sum <= 0 {
			return 0
		}
		return x * 100 / sum
	}
	v.Pct, v.Locked, v.Reach = p(per[accountID]), p(locked), p(reach)
	ids := make([]int64, 0, len(per))
	for id := range per {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		v.Rows = append(v.Rows, LaneRow{id, p(per[id])})
	}
	return v
}
