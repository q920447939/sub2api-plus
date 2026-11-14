package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ---------- 配置 ----------

// ConfigView 检测配置页上方的开关、健康分规则（含当前生效的）和检测范围
type ConfigView struct {
	Config    Config `json:"config"`
	Version   int64  `json:"version"`
	Effective any    `json:"effective_rules"` // 现在实际用的健康分规则（同渠道状态时是渠道状态的）
	Counts    any    `json:"counts"`          // 按现在的规则：正常 / 降级 / 异常 / 暂无数据 各几个账号
	Serving   int    `json:"serving"`         // 调度开着的账号数（会测）
	Closed    int    `json:"closed"`          // 调度已关闭的账号数（不测）
}

func (s *Service) GetConfig(ctx context.Context) (*ConfigView, error) {
	cfg, ver, err := s.Config.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	st, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	counts := map[Status]int{}
	v := &ConfigView{Config: cfg, Version: ver, Effective: st.health.Rules, Counts: counts}
	for _, a := range st.cat.Accounts {
		if a.Status != "active" {
			continue
		}
		if !a.Schedulable {
			v.Closed++
			continue
		}
		v.Serving++
		counts[st.health.Current(st.tr, TFilter{Account: a.ID}).Status]++
	}
	return v, nil
}

func (s *Service) SaveConfig(ctx context.Context, cfg Config, ver int64, actor Actor) (*ConfigView, error) {
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	old, _, err := s.Config.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.Config.SaveConfig(ctx, cfg, ver); err != nil {
		return nil, err
	}
	var lines []string
	onoff := func(b bool) string {
		if b {
			return "打开"
		}
		return "关闭"
	}
	if old.Detect != cfg.Detect {
		lines = append(lines, "定时检测："+onoff(cfg.Detect))
	}
	if old.Apply != cfg.Apply {
		lines = append(lines, "权重设置生效："+onoff(cfg.Apply))
	}
	if old.SameAsChannel != cfg.SameAsChannel {
		lines = append(lines, "健康分和渠道状态用同一套规则："+onoff(cfg.SameAsChannel))
	}
	num := func(name string, a, b int) {
		if a != b {
			lines = append(lines, fmt.Sprintf("%s：%d → %d", name, a, b))
		}
	}
	num("同时检测", old.Concurrency, cfg.Concurrency)
	num("备用号检测间隔（分钟）", old.IdleMinutes, cfg.IdleMinutes)
	num("连续出错几次算测不了", old.FaultRuns, cfg.FaultRuns)
	if !cfg.SameAsChannel && mustJSONString(old.Rules) != mustJSONString(cfg.Rules) {
		lines = append(lines, rulesDiff(old, cfg)...)
	}
	if len(lines) > 0 {
		s.audit(ctx, actor, "config", "update", "config", strings.Join(lines, "；"), old, cfg)
	}
	s.Detect.Invalidate()
	s.rulesChanged()
	if old.Detect != cfg.Detect && cfg.Detect {
		s.Sched.Wake()
	}
	return s.GetConfig(ctx)
}

func mustJSONString(v any) string { return string(mustJSON(v)) }

func rulesDiff(a, b Config) []string {
	x, y := a.Rules, b.Rules
	var out []string
	f := func(name string, p, q float64, unit string) {
		if p != q {
			out = append(out, fmt.Sprintf("健康分%s：%g%s → %g%s", name, p, unit, q, unit))
		}
	}
	f("正常线", x.UpMin, y.UpMin, "")
	f("降级线", x.WarnMin, y.WarnMin, "")
	f("失败率记 0 分", x.CriticalErrorRate*100, y.CriticalErrorRate*100, "%")
	f("首字满分", float64(x.TargetTTFTMs)/1000, float64(y.TargetTTFTMs)/1000, " 秒")
	f("首字记 0 分", float64(x.CriticalTTFTMs)/1000, float64(y.CriticalTTFTMs)/1000, " 秒")
	f("失败率比重", x.ErrorWeight*100, y.ErrorWeight*100, "")
	f("首字比重", x.TTFTWeight*100, y.TTFTWeight*100, "")
	f("缓存比重", x.CacheWeight*100, y.CacheWeight*100, "")
	f("每格最少请求", float64(x.HistoryMinSample), float64(y.HistoryMinSample), "")
	f("当前状态最少请求", float64(x.MinSample), float64(y.MinSample), "")
	return out
}

// ---------- 元数据：检测方式、平台、各平台的模型（带流量占比）和账号 ----------

// PlatformModel 平台上的一个模型
type PlatformModel struct {
	Model string  `json:"model"`
	N     int64   `json:"n"`
	Share float64 `json:"share"` // 占这个平台近 24 小时请求的百分比
}

// PickAccount 计划里可选的账号
type PickAccount struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Serving bool     `json:"serving"`
	Direct  bool     `json:"direct"`
	Why     string   `json:"why,omitempty"`
	Models  []string `json:"models"`
}

// Meta 页面需要的元数据
type Meta struct {
	Types     []TypeSpec                 `json:"types"`
	Metrics   []Metric                   `json:"metrics"`
	Platforms []string                   `json:"platforms"`
	Models    map[string][]PlatformModel `json:"models"`
	Accounts  map[string][]PickAccount   `json:"accounts"`
	Library   map[string]any             `json:"library"`
}

func (s *Service) Meta(ctx context.Context) (*Meta, error) {
	st, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	m := &Meta{Types: s.Registry.Specs(), Metrics: metrics, Platforms: []string{}, Models: map[string][]PlatformModel{}, Accounts: map[string][]PickAccount{}}
	per := map[string]map[string]int64{}
	sum := map[string]int64{}
	st.tr.Each(TFilter{From: st.tr.NowBucket() - 23*3600}, func(r *TRow) {
		a := st.cat.Accounts[r.Account]
		if a == nil || !a.Serving() {
			return
		}
		if per[a.Platform] == nil {
			per[a.Platform] = map[string]int64{}
		}
		per[a.Platform][r.Model] += r.Total()
		sum[a.Platform] += r.Total()
	})
	plats := map[string]bool{}
	for _, a := range st.cat.Accounts {
		if a.Status != "active" {
			continue
		}
		plats[a.Platform] = true
		m.Accounts[a.Platform] = append(m.Accounts[a.Platform], PickAccount{ID: a.ID, Name: a.Name, Serving: a.Serving(), Direct: a.Direct,
			Why: a.DirectWhy, Models: nonNil(a.Models)})
		// 模型映射里写了、但近 24 小时没流量的模型也列出来（计划可以选）
		for _, x := range a.Models {
			if !strings.ContainsAny(x, "*") {
				if per[a.Platform] == nil {
					per[a.Platform] = map[string]int64{}
				}
				if _, ok := per[a.Platform][x]; !ok {
					per[a.Platform][x] = 0
				}
			}
		}
	}
	for p := range plats {
		m.Platforms = append(m.Platforms, p)
		list := []PlatformModel{}
		for x, n := range per[p] {
			pm := PlatformModel{Model: x, N: n}
			if sum[p] > 0 {
				pm.Share = round1(float64(n) * 100 / float64(sum[p]))
			}
			list = append(list, pm)
		}
		sort.Slice(list, func(i, j int) bool {
			return list[i].N > list[j].N || (list[i].N == list[j].N && list[i].Model < list[j].Model)
		})
		m.Models[p] = list
		sort.Slice(m.Accounts[p], func(i, j int) bool { return m.Accounts[p][i].ID < m.Accounts[p][j].ID })
	}
	sort.Slice(m.Platforms, func(i, j int) bool { return sum[m.Platforms[i]] > sum[m.Platforms[j]] })
	if ms := s.Library.Models(); ms != nil {
		list := make([]string, 0, len(ms))
		for _, v := range ms {
			list = append(list, v)
		}
		sort.Strings(list)
		m.Library = map[string]any{"known": true, "models": list, "built_at": s.Library.BuiltAt()}
	} else {
		m.Library = map[string]any{"known": false}
	}
	return m, nil
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// ---------- 检测项 + 计划 ----------

// ItemView 检测配置页的一行
type ItemView struct {
	Item
	Spec     TypeSpec        `json:"spec"`
	Plans    []PlanView      `json:"plans"`
	Runs24   int             `json:"runs24"`
	Pass24   int             `json:"pass24"`
	Judged24 int             `json:"judged24"`
	Tokens24 int64           `json:"tokens24"`
	Failing  int             `json:"failing"` // 现在没通过的 账号 × 模型
	Targets  int             `json:"targets"` // 会测的 账号 × 模型
	Covered  *float64        `json:"covered"` // 覆盖了多少流量 %
	Missing  []PlatformModel `json:"missing"` // 没覆盖的模型（占全站请求 %）
}

// PlanView 计划 + 会测几个
type PlanView struct {
	Plan
	Targets int `json:"targets"`
}

func (s *Service) Items(ctx context.Context) ([]ItemView, error) {
	st, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	stats, err := s.Runs.RunStats(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		return nil, err
	}
	// 近 24 小时各 账号 × 模型 的请求数（覆盖率用）
	pairN := map[pairKey]int64{}
	var all int64
	st.tr.Each(TFilter{From: st.tr.NowBucket() - 23*3600}, func(r *TRow) {
		if a := st.cat.Accounts[r.Account]; a != nil && a.Serving() {
			pairN[pairKey{r.Account, r.Model}] += r.Total()
			all += r.Total()
		}
	})
	out := []ItemView{}
	for _, it := range st.det.Items {
		v := ItemView{Item: it, Spec: s.Registry.Spec(it.Type), Plans: []PlanView{}, Missing: []PlatformModel{}}
		for _, x := range stats {
			if x.ItemID != it.ID {
				continue
			}
			v.Runs24 += x.N
			v.Tokens24 += x.Tokens
			if x.Outcome == OutcomePass {
				v.Pass24 += x.N
			}
			if x.Outcome == OutcomePass || x.Outcome == OutcomeFail {
				v.Judged24 += x.N
			}
		}
		targets := map[pairKey]bool{}
		for _, p := range st.det.Plans {
			if p.ItemID != it.ID {
				continue
			}
			pv := PlanView{Plan: p}
			for _, r := range s.Planner.Cover(ctx, p, it, st.cat, st.tr) {
				if r.OK {
					pv.Targets++
					if p.Enabled {
						targets[pairKey{r.AccountID, r.Model}] = true
					}
				}
			}
			v.Plans = append(v.Plans, pv)
		}
		v.Targets = len(targets)
		if v.Spec.Judges {
			for k := range targets {
				if vd := st.det.Verdict(it, st.cat.Accounts[k.Account], k.Model); vd != nil && vd.Level == LevelBad {
					v.Failing++
				}
			}
		}
		if all > 0 {
			var cov int64
			miss := map[[2]string]int64{}
			for k, n := range pairN {
				if targets[k] {
					cov += n
					continue
				}
				miss[[2]string{st.cat.Accounts[k.Account].Platform, k.Model}] += n
			}
			c := round1(float64(cov) * 100 / float64(all))
			v.Covered = &c
			for k, n := range miss {
				v.Missing = append(v.Missing, PlatformModel{Model: k[0] + "|" + k[1], N: n, Share: round1(float64(n) * 100 / float64(all))})
			}
			sort.Slice(v.Missing, func(i, j int) bool { return v.Missing[i].N > v.Missing[j].N })
			if len(v.Missing) > 8 {
				v.Missing = v.Missing[:8]
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// ItemReq 新建 / 修改检测项（连同它的计划一起保存）
type ItemReq struct {
	Item  Item   `json:"item"`
	Plans []Plan `json:"plans"`
}

func (s *Service) normalizePlan(p *Plan, spec TypeSpec) error {
	p.Platform = strings.TrimSpace(p.Platform)
	if p.Platform == "" {
		return fmt.Errorf("%w：计划要选平台", ErrInvalid)
	}
	seen := map[string]bool{}
	models := []string{}
	for _, m := range p.Models {
		if m = strings.TrimSpace(m); m != "" && !seen[m] {
			seen[m] = true
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		return fmt.Errorf("%w：计划至少选一个模型", ErrInvalid)
	}
	p.Models = models
	if p.AccountIDs == nil {
		p.AccountIDs = []int64{}
	}
	if p.IntervalMinutes == 0 {
		p.IntervalMinutes = spec.DefaultEvery
	}
	if p.IntervalMinutes < minIntervalMinutes || p.IntervalMinutes > maxIntervalMinutes {
		return fmt.Errorf("%w：频率要在 %d 到 %d 分钟之间", ErrInvalid, minIntervalMinutes, maxIntervalMinutes)
	}
	if p.TimeoutSeconds <= 0 {
		p.TimeoutSeconds = 180
	}
	if p.TimeoutSeconds > 1800 {
		p.TimeoutSeconds = 1800
	}
	return nil
}

func planText(names func(int64) string, p Plan) string {
	acc := "全部账号"
	if len(p.AccountIDs) > 0 {
		var ns []string
		for _, id := range p.AccountIDs {
			ns = append(ns, names(id))
		}
		acc = strings.Join(ns, "、")
	}
	state := ""
	if !p.Enabled {
		state = "（停用）"
	}
	return fmt.Sprintf("%s %s · %s · 每 %d 分钟%s", p.Platform, strings.Join(p.Models, "、"), acc, p.IntervalMinutes, state)
}

func (s *Service) SaveItem(ctx context.Context, req ItemReq, actor Actor) (int64, error) {
	it := req.Item
	det, ok := s.Registry.Get(it.Type)
	if !ok {
		return 0, fmt.Errorf("%w：不认识的检测方式 %q", ErrInvalid, it.Type)
	}
	it.Name = strings.TrimSpace(it.Name)
	if it.Name == "" {
		return 0, fmt.Errorf("%w：名称不能为空", ErrInvalid)
	}
	params, rule, err := det.NormalizeItem(it.Params, it.Rule)
	if err != nil {
		return 0, err
	}
	it.Params, it.Rule = params, rule
	spec := det.Spec()
	for i := range req.Plans {
		if err := s.normalizePlan(&req.Plans[i], spec); err != nil {
			return 0, err
		}
	}
	items, err := s.Setup.ListItems(ctx)
	if err != nil {
		return 0, err
	}
	var old *Item
	for i := range items {
		if items[i].ID == it.ID {
			old = &items[i]
		}
		if items[i].ID != it.ID && items[i].Name == it.Name {
			return 0, fmt.Errorf("%w：已经有叫「%s」的检测项", ErrInvalid, it.Name)
		}
	}
	if it.ID > 0 && old == nil {
		return 0, ErrNotFound
	}
	if old != nil {
		it.Type = old.Type // 建好以后类型不能改
	}
	plans, err := s.Setup.ListPlans(ctx)
	if err != nil {
		return 0, err
	}
	id, err := s.Setup.SaveItem(ctx, it)
	if err != nil {
		return 0, err
	}
	// 计划：按 id 对齐，多的删、少的建
	st, _ := s.load(ctx)
	name := func(id int64) string {
		if st != nil {
			return st.accountName(id)
		}
		return "#" + itoa(id)
	}
	keep := map[int64]bool{}
	var lines []string
	oldPlan := map[int64]Plan{}
	for _, p := range plans {
		if p.ItemID == id {
			oldPlan[p.ID] = p
		}
	}
	for _, p := range req.Plans {
		p.ItemID = id
		if o, ok := oldPlan[p.ID]; p.ID > 0 && !ok {
			p.ID = 0
		} else if ok && planText(name, o) != planText(name, p) {
			lines = append(lines, "计划 "+planText(name, o)+" → "+planText(name, p))
		}
		if p.ID == 0 {
			lines = append(lines, "新建计划 "+planText(name, p))
		}
		pid, err := s.Setup.SavePlan(ctx, p)
		if err != nil {
			return id, err
		}
		keep[pid] = true
	}
	for pid, p := range oldPlan {
		if !keep[pid] {
			if err := s.Setup.DeletePlan(ctx, pid); err != nil {
				return id, err
			}
			lines = append(lines, "删除计划 "+planText(name, p))
		}
	}
	verb := "修改"
	if old == nil {
		verb = "新建"
	} else {
		if old.Name != it.Name {
			lines = append([]string{"名称「" + old.Name + "」→「" + it.Name + "」"}, lines...)
		}
		if string(old.Params) != string(it.Params) {
			lines = append([]string{"题目"}, lines...)
		}
		if string(old.Rule) != string(it.Rule) {
			lines = append([]string{"判定规则 " + ruleSentence(spec, old.Rule) + " → " + ruleSentence(spec, it.Rule)}, lines...)
		}
		if old.Enabled != it.Enabled {
			lines = append(lines, map[bool]string{true: "启用", false: "停用"}[it.Enabled])
		}
	}
	sum := fmt.Sprintf("%s检测项「%s」（%s）", verb, it.Name, spec.Name)
	if len(lines) > 0 {
		sum += "：" + strings.Join(lines, "；")
	}
	s.audit(ctx, actor, "config", strings.ToLower(map[bool]string{true: "create", false: "update"}[old == nil]), itoa(id), sum, old, req)
	s.Detect.Invalidate()
	return id, nil
}

// ruleSentence 判定规则写成一句话
func ruleSentence(spec TypeSpec, raw json.RawMessage) string {
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	out := spec.RuleText
	for _, f := range spec.RuleFields {
		out = strings.ReplaceAll(out, "{"+f.Key+"}", fmt.Sprint(m[f.Key]))
	}
	return out
}

// ToggleItem 启用 / 停用
func (s *Service) ToggleItem(ctx context.Context, id int64, on bool, actor Actor) error {
	items, err := s.Setup.ListItems(ctx)
	if err != nil {
		return err
	}
	for _, it := range items {
		if it.ID == id {
			it.Enabled = on
			if _, err := s.Setup.SaveItem(ctx, it); err != nil {
				return err
			}
			s.audit(ctx, actor, "config", "update", itoa(id), fmt.Sprintf("检测项「%s」：%s", it.Name, map[bool]string{true: "启用", false: "停用"}[on]), nil, nil)
			s.Detect.Invalidate()
			return nil
		}
	}
	return ErrNotFound
}

// DeleteItem 删除检测项（连同计划和检测记录）
func (s *Service) DeleteItem(ctx context.Context, id int64, actor Actor) error {
	items, err := s.Setup.ListItems(ctx)
	if err != nil {
		return err
	}
	name := ""
	for _, it := range items {
		if it.ID == id {
			name = it.Name
		}
	}
	if err := s.Setup.DeleteItem(ctx, id); err != nil {
		return err
	}
	s.audit(ctx, actor, "config", "delete", itoa(id), fmt.Sprintf("删除检测项「%s」（计划和检测记录一并删除）", name), nil, nil)
	s.Detect.Invalidate()
	return nil
}

// TogglePlan 计划的启用开关
func (s *Service) TogglePlan(ctx context.Context, id int64, on bool, actor Actor) error {
	plans, err := s.Setup.ListPlans(ctx)
	if err != nil {
		return err
	}
	st, err := s.load(ctx)
	if err != nil {
		return err
	}
	for _, p := range plans {
		if p.ID == id {
			p.Enabled = on
			if _, err := s.Setup.SavePlan(ctx, p); err != nil {
				return err
			}
			item := ""
			if it := st.det.Item(p.ItemID); it != nil {
				item = it.Name
			}
			s.audit(ctx, actor, "config", "update", "plan:"+itoa(id), fmt.Sprintf("「%s」的计划 %s：%s", item, planText(st.accountName, p),
				map[bool]string{true: "启用", false: "停用"}[on]), nil, nil)
			s.Detect.Invalidate()
			return nil
		}
	}
	return ErrNotFound
}

// PreviewPlan 计划会测哪些 账号 × 模型（编辑时实时看）
func (s *Service) PreviewPlan(ctx context.Context, itemType string, p Plan) ([]CoverRow, error) {
	st, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	p.Enabled = true
	return s.Planner.Cover(ctx, p, Item{Type: itemType}, st.cat, st.tr), nil
}

// RunPlan 立即执行一个计划
func (s *Service) RunPlan(ctx context.Context, id int64, actor Actor) (int, error) {
	plans, err := s.Setup.ListPlans(ctx)
	if err != nil {
		return 0, err
	}
	for _, p := range plans {
		if p.ID == id {
			n, err := s.Sched.EnqueuePlans(ctx, []Plan{p}, "manual", &actor.ID)
			if err != nil {
				return 0, err
			}
			s.Sched.Wake()
			st, _ := s.load(ctx)
			item := ""
			if st != nil {
				if it := st.det.Item(p.ItemID); it != nil {
					item = it.Name
				}
			}
			s.audit(ctx, actor, "detect", "run", "plan:"+itoa(id), fmt.Sprintf("立即执行「%s」的计划 %s（排队 %d 次检测）", item, planText(func(i int64) string {
				if st != nil {
					return st.accountName(i)
				}
				return itoa(i)
			}, p), n), nil, nil)
			return n, nil
		}
	}
	return 0, ErrNotFound
}

// ---------- 检测详情（某账号的某项检测，按模型列出最近的每一次） ----------

// ProbeModel 一个模型
type ProbeModel struct {
	Model   string   `json:"model"`
	Verdict *Verdict `json:"verdict"`
	Every   *int     `json:"every"` // 定时检测的频率（分钟）；没有计划为 nil
	Ignore  *Ignore  `json:"ignore"`
	Runs    []Run    `json:"runs"` // 旧 → 新，最多 12 次
}

// ProbeSheet 检测详情
type ProbeSheet struct {
	Item     Item         `json:"item"`
	Spec     TypeSpec     `json:"spec"`
	Account  string       `json:"account"`
	Models   []ProbeModel `json:"models"`
	Untested []string     `json:"untested"`
}

func planEvery(plans []Plan, item int64, a *CatAccount, model string) *int {
	var best *int
	for _, p := range plans {
		if p.ItemID != item || !p.Enabled || p.Platform != a.Platform {
			continue
		}
		if len(p.AccountIDs) > 0 && !containsID(p.AccountIDs, a.ID) {
			continue
		}
		for _, m := range p.Models {
			if m == model && (best == nil || p.IntervalMinutes < *best) {
				v := p.IntervalMinutes
				best = &v
			}
		}
	}
	return best
}

func containsID(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func (s *Service) ProbeSheet(ctx context.Context, accountID, itemID int64) (*ProbeSheet, error) {
	st, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	a := st.cat.Accounts[accountID]
	it := st.det.Item(itemID)
	if a == nil || it == nil {
		return nil, ErrNotFound
	}
	out := &ProbeSheet{Item: *it, Spec: s.Registry.Spec(it.Type), Account: a.Name, Models: []ProbeModel{}, Untested: []string{}}
	tested := map[string]bool{}
	for _, m := range a.ModelsSeenIn(st.det, itemID) {
		tested[m] = true
	}
	for _, m := range st.accountModels(a) {
		if !tested[m.Model] {
			if len(out.Untested) < 12 && (m.N > 0 || a.Supports(m.Model)) {
				out.Untested = append(out.Untested, m.Model)
			}
			continue
		}
		rs := st.det.Runs(itemID, a, m.Model)
		if len(rs) > 12 {
			rs = rs[:12]
		}
		asc := make([]Run, len(rs))
		for i := range rs {
			asc[len(rs)-1-i] = rs[i]
		}
		out.Models = append(out.Models, ProbeModel{Model: m.Model, Verdict: st.det.Verdict(*it, a, m.Model), Every: planEvery(st.det.Plans, itemID, a, m.Model),
			Ignore: st.det.Ignored(itemID, a, m.Model), Runs: asc})
	}
	return out, nil
}

// Retest 手动检测一次（一个或几个模型）
func (s *Service) Retest(ctx context.Context, accountID, itemID int64, models []string, actor Actor) (int, error) {
	st, err := s.load(ctx)
	if err != nil {
		return 0, err
	}
	a := st.cat.Accounts[accountID]
	it := st.det.Item(itemID)
	if a == nil || it == nil {
		return 0, ErrNotFound
	}
	if !a.Direct {
		return 0, fmt.Errorf("%w：%s", ErrInvalid, a.DirectWhy)
	}
	if len(models) == 0 {
		return 0, fmt.Errorf("%w：至少选一个模型", ErrInvalid)
	}
	n := 0
	for _, m := range models {
		ok, err := s.Sched.EnqueueOne(ctx, *it, a, m, &actor.ID)
		if err != nil {
			return n, err
		}
		if ok {
			n++
		}
	}
	s.audit(ctx, actor, "detect", "run", itoa(accountID), fmt.Sprintf("手动检测「%s」：%s · %s", it.Name, a.Name, strings.Join(models, "、")), nil, nil)
	return n, nil
}

// SetIgnore 标为误报 / 取消误报
func (s *Service) SetIgnore(ctx context.Context, accountID, itemID int64, model string, on bool, actor Actor) error {
	st, err := s.load(ctx)
	if err != nil {
		return err
	}
	a := st.cat.Accounts[accountID]
	it := st.det.Item(itemID)
	if a == nil || it == nil {
		return ErrNotFound
	}
	if on {
		rs := st.det.Runs(itemID, a, model)
		if len(rs) == 0 {
			return fmt.Errorf("%w：还没有检测记录", ErrInvalid)
		}
		actor.Email = s.operator(ctx, actor)
		if err := s.Ignores.SetIgnore(ctx, Ignore{CredKey: a.CredKey, Model: model, ItemID: itemID, UpToRun: rs[0].ID}, actor); err != nil {
			return err
		}
	} else if err := s.Ignores.DeleteIgnore(ctx, a.CredKey, model, itemID); err != nil {
		return err
	}
	s.audit(ctx, actor, "detect", map[bool]string{true: "ignore", false: "unignore"}[on], itoa(accountID),
		fmt.Sprintf("%s：%s · %s · %s", map[bool]string{true: "标为误报", false: "取消误报"}[on], a.Name, model, it.Name), nil, nil)
	s.Detect.Invalidate()
	return nil
}

// Run 一次检测的全部内容（含题面和回答）
func (s *Service) Run(ctx context.Context, id int64) (*Run, error) {
	r, err := s.Runs.GetRun(ctx, id)
	if err != nil {
		return nil, err
	}
	det, err := s.Detect.Get(ctx)
	if err != nil {
		return nil, err
	}
	if it := det.Item(r.ItemID); it != nil {
		x := settle(s.Registry, *it, *r, det.ignores[runKeyOf(r.ItemID, r.CredKey, r.Model)])
		x.Prompt, x.Reply = r.Prompt, r.Reply
		return &x, nil
	}
	return r, nil
}

// ---------- 检测记录页 ----------

// RunsQuery 筛选
type RunsQuery struct {
	ItemID   int64
	Platform string
	Result   string // 按账号：fail | fault；按时间：outcome
	Keyword  string
}

// RunsKPI 近 24 小时
type RunsKPI struct {
	Total    int            `json:"total"`
	Outcomes map[string]int `json:"outcomes"`
	Tokens   int64          `json:"tokens"`
	Tested   int            `json:"tested"`  // 已测到的 账号 × 模型
	Targets  int            `json:"targets"` // 计划里该测的 账号 × 模型
}

// RunsRow 按账号：一个 账号 × 模型 × 检测项
type RunsRow struct {
	AccountID int64    `json:"account_id"`
	Account   string   `json:"account"`
	Groups    []int64  `json:"group_ids"`
	Model     string   `json:"model"`
	ItemID    int64    `json:"item_id"`
	Item      string   `json:"item"`
	Dots      []RunDot `json:"dots"`
	Verdict   *Verdict `json:"verdict"`
	Last      *RunDot  `json:"last"`
	Every     *int     `json:"every"`
	Thumb     *int64   `json:"thumb,omitempty"` // 存档题：最近一份作品的记录 id
}

// RunsBoard 检测记录（按账号）
type RunsBoard struct {
	KPI    RunsKPI   `json:"kpi"`
	Rows   []RunsRow `json:"rows"`
	Names  NameMap   `json:"names"`
	NoPlan bool      `json:"no_plan"` // 选中的检测项还没有计划
}

func (s *Service) RunsBoard(ctx context.Context, q RunsQuery) (*RunsBoard, error) {
	st, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	stats, err := s.Runs.RunStats(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		return nil, err
	}
	kw := strings.ToLower(strings.TrimSpace(q.Keyword))
	match := func(a *CatAccount) bool {
		return a != nil && (q.Platform == "" || a.Platform == q.Platform) && (kw == "" || strings.Contains(strings.ToLower(a.Name), kw))
	}
	out := &RunsBoard{KPI: RunsKPI{Outcomes: map[string]int{}}, Rows: []RunsRow{}, Names: st.names()}
	for _, x := range stats {
		a := st.cat.Accounts[x.AccountID]
		if (q.ItemID > 0 && x.ItemID != q.ItemID) || !match(a) || !a.Supports(x.Model) {
			continue
		}
		out.KPI.Total += x.N
		out.KPI.Outcomes[x.Outcome] += x.N
		out.KPI.Tokens += x.Tokens
	}
	hasPlan := false
	seen := map[string]bool{}
	add := func(it Item, a *CatAccount, model string) {
		k := runKeyOf(it.ID, itoa(a.ID), model)
		if seen[k] {
			return
		}
		seen[k] = true
		rs := st.det.Runs(it.ID, a, model)
		row := RunsRow{AccountID: a.ID, Account: a.Name, Groups: a.Groups, Model: model, ItemID: it.ID, Item: it.Name, Dots: dots(rs, 12),
			Verdict: st.det.Verdict(it, a, model), Every: planEvery(st.det.Plans, it.ID, a, model)}
		if len(row.Dots) > 0 {
			l := row.Dots[len(row.Dots)-1]
			row.Last = &l
			out.KPI.Tested++
		}
		for _, r := range rs {
			if r.Outcome == OutcomeSaved {
				id := r.ID
				row.Thumb = &id
				break
			}
		}
		out.Rows = append(out.Rows, row)
	}
	for _, it := range st.det.Items {
		if q.ItemID > 0 && it.ID != q.ItemID {
			continue
		}
		// 计划里该测的（包括还没测到的）
		for _, p := range st.det.Plans {
			if p.ItemID != it.ID || !p.Enabled {
				continue
			}
			hasPlan = true
			for _, r := range s.Planner.Cover(ctx, p, it, st.cat, st.tr) {
				if a := st.cat.Accounts[r.AccountID]; r.OK && match(a) {
					out.KPI.Targets++
					add(it, a, r.Model)
				}
			}
		}
		// 不在计划里但测过的（手动测的、计划改过的）
		for _, a := range st.serving("") {
			if !match(a) {
				continue
			}
			for _, m := range a.ModelsSeenIn(st.det, it.ID) {
				add(it, a, m)
			}
		}
	}
	out.NoPlan = q.ItemID > 0 && !hasPlan && len(out.Rows) == 0
	// 筛选结果；未通过排最前，其次最近出错的，再按最近一次时间
	rank := func(r RunsRow) int {
		switch {
		case r.Verdict != nil && r.Verdict.Level == LevelBad:
			return 3
		case r.Verdict != nil && r.Verdict.Level == LevelFault:
			return 2
		case r.Last == nil:
			return 0
		}
		return 1
	}
	rows := out.Rows[:0]
	for _, r := range out.Rows {
		switch q.Result {
		case "fail":
			if rank(r) != 3 {
				continue
			}
		case "fault":
			if rank(r) != 2 {
				continue
			}
		case "untested":
			if r.Last != nil {
				continue
			}
		}
		rows = append(rows, r)
	}
	out.Rows = rows
	sort.SliceStable(out.Rows, func(i, j int) bool {
		a, b := out.Rows[i], out.Rows[j]
		if rank(a) != rank(b) {
			return rank(a) > rank(b)
		}
		if a.Last != nil && b.Last != nil {
			return a.Last.At.After(b.Last.At)
		}
		return a.Account < b.Account
	})
	return out, nil
}

// RunsList 检测记录（按时间）
func (s *Service) RunsList(ctx context.Context, q RunsQuery, beforeID int64) ([]Run, error) {
	st, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	kw := strings.ToLower(strings.TrimSpace(q.Keyword))
	var out []Run
	cursor := beforeID
	for page := 0; page < 10 && len(out) < 100; page++ {
		rs, err := s.Runs.ListRuns(ctx, RunFilter{ItemID: q.ItemID, BeforeID: cursor, Limit: 300})
		if err != nil {
			return nil, err
		}
		if len(rs) == 0 {
			break
		}
		for _, r := range rs {
			cursor = r.ID
			a := st.cat.Accounts[r.AccountID]
			it := st.det.Item(r.ItemID)
			if a == nil || it == nil || !a.Supports(r.Model) || (q.Platform != "" && a.Platform != q.Platform) ||
				(kw != "" && !strings.Contains(strings.ToLower(a.Name), kw)) {
				continue
			}
			x := settle(s.Registry, *it, r, st.det.ignores[runKeyOf(r.ItemID, r.CredKey, r.Model)])
			if q.Result != "" && x.Outcome != q.Result {
				continue
			}
			out = append(out, x)
			if len(out) >= 100 {
				break
			}
		}
	}
	if out == nil {
		out = []Run{}
	}
	return out, nil
}

// ---------- 操作日志 ----------

// AuditPage 一页
type AuditPage struct {
	List   []AuditEntry     `json:"list"`
	Total  int64            `json:"total"`
	Counts map[string]int64 `json:"counts"`
}

func (s *Service) Audit(ctx context.Context, f AuditFilter) (*AuditPage, error) {
	list, total, counts, err := s.AuditDB.ListAudit(ctx, f)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []AuditEntry{}
	}
	return &AuditPage{List: list, Total: total, Counts: counts}, nil
}
