package dispatch

import (
	"math"
	"sort"
)

// 调整效果：被调账号所在的这些分组（这个模型）里的全部流量，调整前 → 调整后，前后窗口一样长。
// 不问调整的原因：每一项都给出前后，变差的排最前。

// EffectItem 一项前后对比
type EffectItem struct {
	Key    string   `json:"key"` // 指标 key，或 probe:<检测项 id>
	Label  string   `json:"label"`
	Unit   string   `json:"unit"`
	Before *float64 `json:"before"`
	After  *float64 `json:"after"`
	Trend  string   `json:"trend"` // worse | better | flat | na
}

// EffectShare 一个账号的占比前后
type EffectShare struct {
	AccountID int64    `json:"account_id"`
	Before    float64  `json:"before"`
	After     *float64 `json:"after"`
}

// EffectPart 按分组 / 按模型的细分
type EffectPart struct {
	GroupID int64                 `json:"group_id,omitempty"`
	Model   string                `json:"model,omitempty"`
	NB      int64                 `json:"nb"`
	NA      int64                 `json:"na"`
	SB      *int                  `json:"sb"`
	SA      *int                  `json:"sa"`
	Cols    map[string]EffectItem `json:"cols"`
	Bars    []Slot                `json:"bars,omitempty"`
}

// DeadPair 调整后某账号在这些模型上全部失败
type DeadPair struct {
	AccountID int64    `json:"account_id"`
	Models    []string `json:"models"`
	N         int64    `json:"n"`
}

// Effect 一次调整的效果
type Effect struct {
	Fresh       bool          `json:"fresh"`  // 刚调整，调整后还没有完整的小时
	Enough      bool          `json:"enough"` // 前后都至少 30 次请求
	Hours       int           `json:"hours"`
	NB          int64         `json:"nb"`
	NA          int64         `json:"na"`
	ScoreBefore *int          `json:"score_before"`
	ScoreAfter  *int          `json:"score_after"`
	Items       []EffectItem  `json:"items"`
	Shares      []EffectShare `json:"shares"`
	Groups      []EffectPart  `json:"groups"`
	Models      []EffectPart  `json:"models"`
	Dead        []DeadPair    `json:"dead"`
	Better      int           `json:"better"`
	Worse       int           `json:"worse"`
}

func trendOf(m Metric, b, a *float64, ok bool) string {
	if !ok || b == nil || a == nil {
		return "na"
	}
	d := *a - *b
	big := false
	if m.Abs > 0 {
		big = math.Abs(d) >= m.Abs
	} else {
		big = math.Abs(d) >= m.Rel*math.Max(math.Abs(*b), 1e-9)
	}
	switch {
	case !big:
		return "flat"
	case (d < 0) == (m.Better == "low"):
		return "better"
	}
	return "worse"
}

func valPtr(m Metric, a Agg) *float64 {
	if v, ok := m.Value(a); ok {
		v = round1(v)
		return &v
	}
	return nil
}

// window: 0 = 调整后至今（最多 24 小时）；否则前后各 window 小时
func (st *state) effect(rec Record, window int) Effect {
	const H = int64(3600)
	nowB := st.tr.NowBucket()
	at := rec.CreatedAt.Unix()
	tb := (at + H - 1) / H * H // 调整后第一个完整小时
	n := int((nowB-tb)/H) + 1
	if n < 0 {
		n = 0
	}
	if window > 0 && n > window {
		n = window
	}
	if n > 24 {
		n = 24
	}
	fresh := n == 0
	span := n
	var bFrom, bTo, aFrom, aTo int64
	if fresh {
		span = 6
		bFrom, bTo = nowB-int64(span-1)*H, nowB+H
	} else {
		bFrom, bTo, aFrom, aTo = tb-int64(n)*H, tb, tb, tb+int64(n)*H
	}
	var models map[string]bool
	if rec.Model != "" {
		models = map[string]bool{rec.Model: true}
	}
	groups := map[int64]bool{}
	var gids []int64
	for _, g := range rec.Groups {
		if !groups[g.GroupID] && st.tr.Sum(TFilter{Group: g.GroupID, Models: models, From: bFrom, To: max64(bTo, aTo)}).Total() > 0 {
			groups[g.GroupID] = true
			gids = append(gids, g.GroupID)
		}
	}
	e := Effect{Fresh: fresh, Hours: span, Items: []EffectItem{}, Shares: []EffectShare{}, Groups: []EffectPart{}, Models: []EffectPart{}, Dead: []DeadPair{}}
	if len(gids) == 0 {
		return e
	}
	scope := TFilter{Groups: groups, Models: models}
	win := func(f TFilter, from, to int64) TFilter { f.From, f.To = from, to; return f }
	ab := st.tr.Sum(win(scope, bFrom, bTo))
	var aa Agg
	if !fresh {
		aa = st.tr.Sum(win(scope, aFrom, aTo))
	}
	e.NB, e.NA = ab.Total(), aa.Total()
	e.Enough = !fresh && e.NB >= 30 && e.NA >= 30
	hm := st.health.Rules.HistoryMinSample
	e.ScoreBefore = st.health.Score(ab, hm)
	if !fresh {
		e.ScoreAfter = st.health.Score(aa, hm)
	}
	for _, m := range metrics {
		it := EffectItem{Key: m.Key, Label: m.Name, Unit: m.Unit, Before: valPtr(m, ab)}
		if !fresh {
			it.After = valPtr(m, aa)
		}
		it.Trend = trendOf(m, it.Before, it.After, e.Enough)
		e.Items = append(e.Items, it)
	}
	// 账号 × 模型 在窗口里的请求数（主动检测类：有多少流量走在没通过的账号上；调整后全部失败的组合）
	type pair struct {
		acc   int64
		model string
		n, ok int64
	}
	pairs := func(from, to int64) []pair {
		m := map[pairKey]*pair{}
		st.tr.Each(win(scope, from, to), func(r *TRow) {
			k := pairKey{r.Account, r.Model}
			p := m[k]
			if p == nil {
				p = &pair{acc: r.Account, model: r.Model}
				m[k] = p
			}
			p.n += r.Total()
			p.ok += r.Requests
		})
		out := make([]pair, 0, len(m))
		for _, p := range m {
			out = append(out, *p)
		}
		sort.Slice(out, func(i, j int) bool {
			return out[i].acc < out[j].acc || (out[i].acc == out[j].acc && out[i].model < out[j].model)
		})
		return out
	}
	pb := pairs(bFrom, bTo)
	var pa []pair
	if !fresh {
		pa = pairs(aFrom, aTo)
	}
	for _, it := range st.det.Judging() {
		failing := func(ps []pair) (*float64, bool) {
			var sum, bad int64
			seen := false
			for _, p := range ps {
				a := st.cat.Accounts[p.acc]
				v := st.det.Verdict(it, a, p.model)
				if v != nil {
					seen = true
				}
				sum += p.n
				if v != nil && v.Level == LevelBad {
					bad += p.n
				}
			}
			if sum == 0 {
				return nil, seen
			}
			x := round1(float64(bad) * 100 / float64(sum))
			return &x, seen
		}
		b, s1 := failing(pb)
		a, s2 := failing(pa)
		if !s1 && !s2 {
			continue
		}
		if fresh {
			a = nil
		}
		item := EffectItem{Key: "probe:" + itoa(it.ID), Label: it.Name + "未通过的账号承接的流量", Unit: "%", Before: b, After: a}
		item.Trend = trendOf(Metric{Abs: 5, Better: "low"}, b, a, e.Enough)
		e.Items = append(e.Items, item)
	}
	order := map[string]int{"worse": 0, "better": 1, "flat": 2, "na": 3}
	sort.SliceStable(e.Items, func(i, j int) bool { return order[e.Items[i].Trend] < order[e.Items[j].Trend] })
	for _, it := range e.Items {
		switch it.Trend {
		case "worse":
			e.Worse++
		case "better":
			e.Better++
		}
	}
	// 各账号的占比
	share := func(ps []pair) (map[int64]float64, int64) {
		per := map[int64]int64{}
		var sum int64
		for _, p := range ps {
			per[p.acc] += p.n
			sum += p.n
		}
		out := map[int64]float64{}
		for id, x := range per {
			if sum > 0 {
				out[id] = float64(x) * 100 / float64(sum)
			}
		}
		return out, sum
	}
	sb, _ := share(pb)
	sa, _ := share(pa)
	ids := map[int64]bool{rec.AccountID: true}
	for id := range sb {
		ids[id] = true
	}
	for id := range sa {
		ids[id] = true
	}
	for id := range ids {
		s := EffectShare{AccountID: id, Before: round1(sb[id])}
		if !fresh {
			v := round1(sa[id])
			s.After = &v
		}
		if s.Before > 0.5 || (s.After != nil && *s.After > 0.5) || id == rec.AccountID {
			e.Shares = append(e.Shares, s)
		}
	}
	sort.Slice(e.Shares, func(i, j int) bool {
		if (e.Shares[i].AccountID == rec.AccountID) != (e.Shares[j].AccountID == rec.AccountID) {
			return e.Shares[i].AccountID == rec.AccountID
		}
		return e.Shares[i].Before > e.Shares[j].Before
	})
	// 调整后全部失败（至少 5 次）的 账号 × 模型
	dead := map[int64]*DeadPair{}
	for _, p := range pa {
		if p.n >= 5 && p.ok == 0 {
			d := dead[p.acc]
			if d == nil {
				d = &DeadPair{AccountID: p.acc}
				dead[p.acc] = d
			}
			d.Models = append(d.Models, p.model)
			d.N += p.n
		}
	}
	for _, d := range dead {
		e.Dead = append(e.Dead, *d)
	}
	sort.Slice(e.Dead, func(i, j int) bool { return e.Dead[i].N > e.Dead[j].N })
	// 细分：按分组（带状态条）、按模型
	part := func(f TFilter) EffectPart {
		xb := st.tr.Sum(win(f, bFrom, bTo))
		var xa Agg
		if !fresh {
			xa = st.tr.Sum(win(f, aFrom, aTo))
		}
		ok := !fresh && xb.Total() >= 10 && xa.Total() >= 10
		p := EffectPart{NB: xb.Total(), NA: xa.Total(), SB: st.health.Score(xb, hm), Cols: map[string]EffectItem{}}
		if !fresh {
			p.SA = st.health.Score(xa, hm)
		}
		for _, m := range metrics {
			it := EffectItem{Key: m.Key, Label: m.Name, Unit: m.Unit, Before: valPtr(m, xb)}
			if !fresh {
				it.After = valPtr(m, xa)
			}
			it.Trend = trendOf(m, it.Before, it.After, ok)
			p.Cols[m.Key] = it
		}
		return p
	}
	for _, g := range gids {
		p := part(TFilter{Group: g, Models: models})
		p.GroupID = g
		p.Bars = st.health.Timeline(st.tr, TFilter{Group: g, Models: models}, GranHour, 48)
		e.Groups = append(e.Groups, p)
	}
	sort.Slice(e.Groups, func(i, j int) bool { return e.Groups[i].NB+e.Groups[i].NA > e.Groups[j].NB+e.Groups[j].NA })
	names := map[string]bool{}
	for _, p := range append(append([]pair{}, pb...), pa...) {
		names[p.model] = true
	}
	for m := range names {
		p := part(TFilter{Groups: groups, Model: m})
		p.Model = m
		e.Models = append(e.Models, p)
	}
	sort.Slice(e.Models, func(i, j int) bool { return e.Models[i].NB+e.Models[i].NA > e.Models[j].NB+e.Models[j].NA })
	if len(e.Models) > 8 {
		e.Models = e.Models[:8]
	}
	return e
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
