package dispatch

import (
	"context"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/plus/channelstatus"
)

// Cols 一排数字（账号列表、模型列表、同模型账号对比共用）
type Cols struct {
	Score     *int     `json:"score"`
	Status    Status   `json:"status"`
	Hours     int      `json:"hours"`      // 健康分看的是最近几小时
	N24       int64    `json:"n24"`        // 近 24 小时请求
	Failed    int64    `json:"failed"`     // 近 24 小时失败（报错 + 限流）
	ErrPct    *float64 `json:"err_pct"`    // 近 24 小时失败率 %
	TTFT      *float64 `json:"ttft"`       // 首字耗时中位数（秒）
	TPS       *float64 `json:"tps"`        // 输出速度 tok/s
	Cache     *float64 `json:"cache"`      // 缓存命中率 %
	CacheRead int64    `json:"cache_read"` // 命中的 token
}

func (st *state) cols(f TFilter) Cols {
	cur := st.health.Current(st.tr, f)
	f.From = st.tr.NowBucket() - 23*3600
	d := st.tr.Sum(f)
	c := Cols{Score: cur.Score, Status: cur.Status, Hours: cur.Hours, N24: d.Total(), Failed: d.Failed(), CacheRead: d.CacheRead}
	put := func(dst **float64, key string) {
		if v, ok := metricVal(key, d); ok {
			v = round1(v)
			*dst = &v
		}
	}
	if d.Total() > 0 {
		v := round1(float64(d.Failed()) * 100 / float64(d.Total()))
		c.ErrPct = &v
	}
	put(&c.TTFT, "ttft")
	put(&c.TPS, "tps")
	put(&c.Cache, "cache")
	return c
}

// ProbeTag 一个检测项在账号（或它的某个模型）上的状态
type ProbeTag struct {
	ItemID int64    `json:"item_id"`
	Name   string   `json:"name"`
	State  string   `json:"state"` // fail | pass | ignored | fault | few | none
	Models []string `json:"models,omitempty"`
	Short  string   `json:"short,omitempty"`
}

// accountModels 账号近 24 小时有流量的模型 + 测过的模型（按请求数从多到少）
func (st *state) accountModels(a *CatAccount) []laneModel {
	per := map[string]int64{}
	st.tr.Each(TFilter{Account: a.ID, From: st.tr.NowBucket() - 23*3600}, func(r *TRow) { per[r.Model] += r.Total() })
	for _, it := range st.det.Items {
		for _, m := range a.ModelsSeenIn(st.det, it.ID) {
			if _, ok := per[m]; !ok {
				per[m] = 0
			}
		}
	}
	out := make([]laneModel, 0, len(per))
	for m, n := range per {
		out = append(out, laneModel{m, n})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].N > out[j].N || (out[i].N == out[j].N && out[i].Model < out[j].Model)
	})
	return out
}

// modelProbe 一个模型在一个检测项上的标签
func (st *state) modelProbe(it Item, a *CatAccount, model string) ProbeTag {
	t := ProbeTag{ItemID: it.ID, Name: it.Name, State: "none"}
	v := st.det.Verdict(it, a, model)
	switch {
	case v == nil:
	case v.Level == LevelBad:
		t.State, t.Short = "fail", v.Short
	case v.Level == LevelFault:
		t.State, t.Short = "fault", v.Short
	case v.Level == LevelOK && v.Ignored && v.Bad == 0:
		t.State, t.Short = "ignored", v.Short
	case v.Level == LevelOK:
		t.State = "pass"
	default:
		t.State, t.Short = "few", v.Short
		if v.Ignored {
			t.State = "ignored"
		}
	}
	return t
}

// accountProbes 每个判定类检测项在整个账号上的汇总：有模型没通过 → 未通过（写出模型）；否则有通过的 → 通过
func (st *state) accountProbes(a *CatAccount, models []laneModel) []ProbeTag {
	var out []ProbeTag
	for _, it := range st.det.Judging() {
		by := map[string][]string{}
		for _, m := range models {
			t := st.modelProbe(it, a, m.Model)
			by[t.State] = append(by[t.State], m.Model)
		}
		t := ProbeTag{ItemID: it.ID, Name: it.Name}
		for _, s := range []string{"fail", "pass", "ignored", "fault", "few", "none"} {
			if len(by[s]) > 0 {
				t.State = s
				if s != "pass" {
					t.Models = by[s]
				}
				break
			}
		}
		if t.State == "" {
			t.State = "none"
		}
		out = append(out, t)
	}
	return out
}

// AccountRow 账号列表的一行（一份运营摘要）
type AccountRow struct {
	ID         int64       `json:"id"`
	Name       string      `json:"name"`
	Platform   string      `json:"platform"`
	Groups     []int64     `json:"group_ids"`
	CreatedAt  time.Time   `json:"created_at"`
	IsNew      bool        `json:"is_new"`
	LastAt     *time.Time  `json:"last_at"`   // 最近一次请求（近 2 小时内才有精确时间）
	LastHour   *int64      `json:"last_hour"` // 否则给最近有请求的小时
	Cols       Cols        `json:"cols"`
	Bars       []Slot      `json:"bars"`
	WeekOK     int64       `json:"week_ok"`
	WeekFail   int64       `json:"week_fail"`
	DownModels []string    `json:"down_models"`
	Probes     []ProbeTag  `json:"probes"`
	Rules      []Rule      `json:"rules"`
	Marks      []time.Time `json:"marks"` // 调整发生的时间（状态条上标出来）
	Needs      bool        `json:"needs"` // 需要处理：健康分降级 / 异常、有模型异常、有检测没通过
}

const newAccountDays = 3

func (s *Service) accountRow(ctx context.Context, st *state, a *CatAccount, g Gran, slots int, marks []time.Time) AccountRow {
	models := st.accountModels(a)
	r := AccountRow{ID: a.ID, Name: a.Name, Platform: a.Platform, Groups: a.Groups, CreatedAt: a.CreatedAt,
		IsNew: a.IsNew(newAccountDays, st.now), Cols: st.cols(TFilter{Account: a.ID}),
		Bars: st.health.Timeline(st.tr, TFilter{Account: a.ID}, g, slots), Probes: st.accountProbes(a, models),
		DownModels: []string{}, Rules: []Rule{}, Marks: marks}
	if r.Groups == nil {
		r.Groups = []int64{}
	}
	if r.Marks == nil {
		r.Marks = []time.Time{}
	}
	if t, ok := s.lastSeen(ctx)[a.ID]; ok {
		r.LastAt = &t
	}
	var lastB int64
	st.tr.Each(TFilter{Account: a.ID, From: st.tr.NowBucket() - 7*24*3600}, func(x *TRow) {
		r.WeekOK += x.Requests
		r.WeekFail += x.Failed()
		if x.Total() > 0 && x.Bucket > lastB {
			lastB = x.Bucket
		}
	})
	if lastB > 0 {
		r.LastHour = &lastB
	}
	for _, m := range models {
		if m.N > 0 && st.health.Current(st.tr, TFilter{Account: a.ID, Model: m.Model}).Status == channelstatus.StatusDown {
			r.DownModels = append(r.DownModels, m.Model)
		}
	}
	for _, x := range st.rules {
		if x.AccountID == a.ID {
			r.Rules = append(r.Rules, x)
		}
	}
	r.Needs = r.Cols.Status == channelstatus.StatusDown || r.Cols.Status == channelstatus.StatusWarn || len(r.DownModels) > 0
	for _, p := range r.Probes {
		if p.State == "fail" {
			r.Needs = true
		}
	}
	return r
}

// PlatformSum 平台 Tab 和上方一排大数字
type PlatformSum struct {
	Platform string `json:"platform"`
	Accounts int    `json:"accounts"`
	Worst    Status `json:"worst"`
	Cols     Cols   `json:"cols"`
}

// Board 上游账号列表
type Board struct {
	Now       time.Time     `json:"now"`
	Apply     bool          `json:"apply"` // 权重设置是否生效
	Platforms []PlatformSum `json:"platforms"`
	Groups    []*CatGroup   `json:"groups"`
	Accounts  []AccountRow  `json:"accounts"`
}

func (s *Service) marksByAccount(ctx context.Context, since time.Time) map[int64][]time.Time {
	out := map[int64][]time.Time{}
	recs, err := s.Rules.ListRecords(ctx, 0, 300)
	if err != nil {
		return out
	}
	for _, r := range recs {
		if r.CreatedAt.After(since) {
			out[r.AccountID] = append(out[r.AccountID], r.CreatedAt)
		}
	}
	return out
}

func slotsFor(g Gran, hourly int) int {
	if g == GranDay {
		return 30
	}
	return hourly
}

// Board 上游账号列表：调度开着的账号，每个一行
func (s *Service) Board(ctx context.Context, g Gran) (*Board, error) {
	st, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	n := slotsFor(g, 24)
	marks := s.marksByAccount(ctx, st.now.Add(-time.Duration(n)*time.Hour*map[Gran]time.Duration{GranHour: 1, GranDay: 24}[g]))
	b := &Board{Now: st.now, Apply: st.cfg.Apply, Accounts: []AccountRow{}, Groups: []*CatGroup{}}
	plat := map[string]*PlatformSum{}
	platAcc := map[string]map[int64]bool{}
	used := map[int64]bool{}
	for _, a := range st.serving("") {
		row := s.accountRow(ctx, st, a, g, n, marks[a.ID])
		b.Accounts = append(b.Accounts, row)
		p := plat[a.Platform]
		if p == nil {
			p = &PlatformSum{Platform: a.Platform, Worst: channelstatus.StatusUnknown}
			plat[a.Platform] = p
			platAcc[a.Platform] = map[int64]bool{}
		}
		p.Accounts++
		platAcc[a.Platform][a.ID] = true
		if statusRank(row.Cols.Status) > statusRank(p.Worst) {
			p.Worst = row.Cols.Status
		}
		for _, gid := range a.Groups {
			used[gid] = true
		}
	}
	for k, p := range plat {
		p.Cols = st.cols(TFilter{Accounts: platAcc[k]})
		b.Platforms = append(b.Platforms, *p)
	}
	sort.Slice(b.Platforms, func(i, j int) bool { return b.Platforms[i].Cols.N24 > b.Platforms[j].Cols.N24 })
	for _, gid := range st.cat.GroupIDs {
		if used[gid] {
			b.Groups = append(b.Groups, st.cat.Groups[gid])
		}
	}
	return b, nil
}

// ---------- 账号详情 ----------

// ModelRow 账号的一个模型
type ModelRow struct {
	Model  string     `json:"model"`
	Cols   Cols       `json:"cols"`
	Bars   []Slot     `json:"bars"`
	Probes []ProbeTag `json:"probes"`
	Rules  []Rule     `json:"rules"`
}

// GroupRow 账号所在的一个分组
type GroupRow struct {
	GroupID int64     `json:"group_id"`
	Name    string    `json:"name"`
	Bars    []Slot    `json:"bars"`
	Score   *int      `json:"score"`
	Status  Status    `json:"status"`
	N24     int64     `json:"n24"`
	Pct     float64   `json:"pct"`    // 它承接的
	Others  []LaneRow `json:"others"` // 同组其他账号承接的
}

// AccountDetail 账号详情
type AccountDetail struct {
	Account  AccountRow   `json:"account"`
	Models   []ModelRow   `json:"models"`
	Quiet    int          `json:"quiet"` // 另有几个模型请求很少（不列出）
	Groups   []GroupRow   `json:"groups"`
	Records  []RecordView `json:"records"`
	Names    NameMap      `json:"names"`
	Platform string       `json:"platform"`
}

// NameMap 页面上出现的账号 / 分组名字
type NameMap struct {
	Accounts map[int64]string `json:"accounts"`
	Groups   map[int64]string `json:"groups"`
}

func (st *state) names() NameMap {
	n := NameMap{Accounts: map[int64]string{}, Groups: map[int64]string{}}
	for id, a := range st.cat.Accounts {
		n.Accounts[id] = a.Name
	}
	for id, g := range st.cat.Groups {
		n.Groups[id] = g.Name
	}
	return n
}

func (s *Service) Account(ctx context.Context, id int64, g Gran) (*AccountDetail, error) {
	st, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	a := st.cat.Accounts[id]
	if a == nil {
		return nil, ErrNotFound
	}
	n := slotsFor(g, 48)
	recs, err := s.records(ctx, st, id, "auto")
	if err != nil {
		return nil, err
	}
	var marks []time.Time
	for _, r := range recs {
		marks = append(marks, r.CreatedAt)
	}
	d := &AccountDetail{Account: s.accountRow(ctx, st, a, g, n, marks), Models: []ModelRow{}, Groups: []GroupRow{}, Records: recs,
		Names: st.names(), Platform: a.Platform}
	for i, m := range st.accountModels(a) {
		if i >= 12 || (m.N < 10 && !st.testedAny(a, m.Model)) {
			d.Quiet++
			continue
		}
		row := ModelRow{Model: m.Model, Cols: st.cols(TFilter{Account: id, Model: m.Model}),
			Bars: st.health.Timeline(st.tr, TFilter{Account: id, Model: m.Model}, g, n), Probes: []ProbeTag{}, Rules: []Rule{}}
		for _, it := range st.det.Items {
			if it.Enabled && s.Registry.Spec(it.Type).Judges {
				row.Probes = append(row.Probes, st.modelProbe(it, a, m.Model))
			}
		}
		for _, x := range st.rules {
			if x.AccountID == id && x.Model == m.Model {
				row.Rules = append(row.Rules, x)
			}
		}
		d.Models = append(d.Models, row)
	}
	l := st.lanes()
	for _, gid := range a.Groups {
		if st.cat.Groups[gid] == nil {
			continue
		}
		gv := l.GroupView(id, "", gid, st.rules)
		cur := st.health.Current(st.tr, TFilter{Group: gid})
		row := GroupRow{GroupID: gid, Name: st.groupName(gid), Bars: st.health.Timeline(st.tr, TFilter{Group: gid}, GranHour, 24),
			Score: cur.Score, Status: cur.Status, N24: st.tr.Sum(TFilter{Group: gid, From: st.tr.NowBucket() - 23*3600}).Total(),
			Pct: round1(gv.Pct), Others: []LaneRow{}}
		for _, r := range gv.Rows {
			if r.ID != id {
				row.Others = append(row.Others, LaneRow{r.ID, round1(r.Pct)})
			}
		}
		d.Groups = append(d.Groups, row)
	}
	return d, nil
}

func (st *state) testedAny(a *CatAccount, model string) bool {
	for _, it := range st.det.Items {
		if len(st.det.Runs(it.ID, a, model)) > 0 {
			return true
		}
	}
	return false
}

// ---------- 账号的一个模型：指标和同类比、检测、趋势、跑这个模型的账号 ----------

// MetricRow 一项指标
type MetricRow struct {
	Key   string   `json:"key"`
	Name  string   `json:"name"`
	Unit  string   `json:"unit"`
	Value *float64 `json:"value"`
	Peer  *float64 `json:"peer"`  // 同平台跑同一个模型的其他账号，现在的中位数
	Worse bool     `json:"worse"` // 明显比同类差
}

// RunDot 一次检测（小方块）
type RunDot struct {
	ID      int64     `json:"id"`
	Outcome string    `json:"outcome"`
	Summary string    `json:"summary"`
	At      time.Time `json:"at"`
}

// ModelProbe 模型详情里的一个检测项
type ModelProbe struct {
	ProbeTag
	Long string   `json:"long"`
	Dots []RunDot `json:"dots"`
}

// Series 趋势（每小时一个点）
type Series struct {
	Labels []int64               `json:"labels"`
	Mine   map[string][]*float64 `json:"mine"`
	Peers  map[string][]*float64 `json:"peers"`
}

// PeerRow 跑同一个模型的账号
type PeerRow struct {
	AccountID int64      `json:"account_id"`
	Name      string     `json:"name"`
	Me        bool       `json:"me"`
	Cols      Cols       `json:"cols"`
	Bars      []Slot     `json:"bars"`
	Probes    []ProbeTag `json:"probes"`
}

// ModelDetail 账号的一个模型
type ModelDetail struct {
	Hours   int          `json:"hours"`
	Metrics []MetricRow  `json:"metrics"`
	Probes  []ModelProbe `json:"probes"`
	Trend   Series       `json:"trend"`
	Peers   []PeerRow    `json:"peers"`
}

func dots(rs []Run, n int) []RunDot {
	if len(rs) > n {
		rs = rs[:n]
	}
	out := make([]RunDot, 0, len(rs))
	for i := len(rs) - 1; i >= 0; i-- { // 旧 → 新
		r := rs[i]
		msg := r.Summary
		if r.Outcome == OutcomeError && r.Error != "" {
			msg = r.Error
		}
		out = append(out, RunDot{ID: r.ID, Outcome: r.Outcome, Summary: truncateRunes(msg, 160), At: r.QueuedAt})
	}
	return out
}

func median(vs []float64) (float64, bool) {
	if len(vs) == 0 {
		return 0, false
	}
	s := append([]float64(nil), vs...)
	sort.Float64s(s)
	m := len(s) / 2
	if len(s)%2 == 1 {
		return s[m], true
	}
	return (s[m-1] + s[m]) / 2, true
}

func worseThanPeer(m Metric, v, ref float64) bool {
	if m.Unit == "%" {
		if m.Better == "low" {
			return v-ref >= 5
		}
		return ref-v >= 30
	}
	if m.Better == "low" {
		return v >= ref*2 && v >= 3
	}
	return v <= ref*0.5
}

func (s *Service) ModelDetail(ctx context.Context, id int64, model string) (*ModelDetail, error) {
	st, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	a := st.cat.Accounts[id]
	if a == nil {
		return nil, ErrNotFound
	}
	cur := st.health.Current(st.tr, TFilter{Account: id, Model: model})
	out := &ModelDetail{Hours: cur.Hours, Metrics: []MetricRow{}, Probes: []ModelProbe{}, Peers: []PeerRow{}}
	// 同类：同平台、跑这个模型的其他账号（调度开着的）
	var peers []*CatAccount
	for _, o := range st.serving(a.Platform) {
		if o.ID != id && (o.Supports(model) || st.tr.Sum(TFilter{Account: o.ID, Model: model, From: st.tr.NowBucket() - 23*3600}).Total() > 0) {
			peers = append(peers, o)
		}
	}
	for _, m := range metrics {
		row := MetricRow{Key: m.Key, Name: m.Name, Unit: m.Unit}
		if v, ok := m.Value(cur.Agg); ok {
			v = round1(v)
			row.Value = &v
		}
		var vs []float64
		for _, o := range peers {
			c := st.health.Current(st.tr, TFilter{Account: o.ID, Model: model})
			if c.Agg.Total() < st.health.Rules.MinSample {
				continue
			}
			if v, ok := m.Value(c.Agg); ok {
				vs = append(vs, v)
			}
		}
		if med, ok := median(vs); ok {
			med = round1(med)
			row.Peer = &med
			row.Worse = row.Value != nil && worseThanPeer(m, *row.Value, med)
		}
		out.Metrics = append(out.Metrics, row)
	}
	for _, it := range st.det.Items {
		if !it.Enabled {
			continue
		}
		mp := ModelProbe{ProbeTag: st.modelProbe(it, a, model), Dots: dots(st.det.Runs(it.ID, a, model), 12)}
		if v := st.det.Verdict(it, a, model); v != nil {
			mp.Long = v.Long
		}
		if !s.Registry.Spec(it.Type).Judges {
			mp.State = "info"
		}
		out.Probes = append(out.Probes, mp)
	}
	// 趋势：这个账号一条线，同模型其他账号的中位数一条线
	starts := slotStarts(st.now, GranHour, 48)
	out.Trend = Series{Labels: starts, Mine: map[string][]*float64{}, Peers: map[string][]*float64{}}
	keys := []string{"score", "err", "r429", "ttft", "tps"}
	series := func(acc int64) map[string][]*float64 {
		sums := make([]Agg, len(starts))
		st.tr.Each(TFilter{Account: acc, Model: model, From: starts[0]}, func(r *TRow) {
			if i := slotIndex(starts, r.Bucket); i >= 0 {
				sums[i] = sums[i].Add(r.Agg)
			}
		})
		res := map[string][]*float64{}
		for _, k := range keys {
			vals := make([]*float64, len(starts))
			for i, x := range sums {
				if k == "score" {
					if sc := st.health.Score(x, st.health.Rules.HistoryMinSample); sc != nil {
						f := float64(*sc)
						vals[i] = &f
					}
				} else if x.Total() >= 5 {
					if v, ok := metricVal(k, x); ok {
						v = round1(v)
						vals[i] = &v
					}
				}
			}
			res[k] = vals
		}
		return res
	}
	out.Trend.Mine = series(id)
	peerSeries := make([]map[string][]*float64, 0, len(peers))
	for _, o := range peers {
		peerSeries = append(peerSeries, series(o.ID))
	}
	for _, k := range keys {
		vals := make([]*float64, len(starts))
		for i := range starts {
			var vs []float64
			for _, ps := range peerSeries {
				if v := ps[k][i]; v != nil {
					vs = append(vs, *v)
				}
			}
			if med, ok := median(vs); ok {
				med = round1(med)
				vals[i] = &med
			}
		}
		out.Trend.Peers[k] = vals
	}
	// 跑这个模型的账号：自己排第一，其余按近 24 小时请求数
	for _, o := range append([]*CatAccount{a}, peers...) {
		pr := PeerRow{AccountID: o.ID, Name: o.Name, Me: o.ID == id, Cols: st.cols(TFilter{Account: o.ID, Model: model}),
			Bars: st.health.Timeline(st.tr, TFilter{Account: o.ID, Model: model}, GranHour, 48), Probes: []ProbeTag{}}
		if pr.Cols.N24 == 0 && !pr.Me && !st.testedAny(o, model) {
			continue
		}
		for _, it := range st.det.Judging() {
			pr.Probes = append(pr.Probes, st.modelProbe(it, o, model))
		}
		out.Peers = append(out.Peers, pr)
	}
	sort.SliceStable(out.Peers, func(i, j int) bool {
		if out.Peers[i].Me != out.Peers[j].Me {
			return out.Peers[i].Me
		}
		return out.Peers[i].Cols.N24 > out.Peers[j].Cols.N24
	})
	return out, nil
}
