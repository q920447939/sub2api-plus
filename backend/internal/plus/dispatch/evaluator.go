package dispatch

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

// Setup 一次评估用到的配置
type Setup struct {
	Config Config
	Dims   []Dimension
	Items  []Item
	Plans  []Plan
}

func (s Setup) dim(id int64) *Dimension {
	for i := range s.Dims {
		if s.Dims[i].ID == id {
			return &s.Dims[i]
		}
	}
	return nil
}

func (s Setup) item(id int64) *Item {
	for i := range s.Items {
		if s.Items[i].ID == id {
			return &s.Items[i]
		}
	}
	return nil
}

// LoadSetup 读取配置
func LoadSetup(ctx context.Context, cs ConfigStore, ss SetupStore) (Setup, error) {
	var (
		st  Setup
		err error
	)
	if st.Config, _, err = cs.LoadConfig(ctx); err != nil {
		return st, err
	}
	if st.Dims, err = ss.ListDimensions(ctx); err != nil {
		return st, err
	}
	if st.Items, err = ss.ListItems(ctx); err != nil {
		return st, err
	}
	st.Plans, err = ss.ListPlans(ctx)
	return st, err
}

// coverIndex 每个计划覆盖到的 账号|模型 → 覆盖结果
type coverIndex map[int64]map[string]CoverRow

func buildCoverIndex(ctx context.Context, p Planner, st Setup, snap *Snapshot) coverIndex {
	idx := coverIndex{}
	for _, plan := range st.Plans {
		it := st.item(plan.ItemID)
		if it == nil {
			continue
		}
		m := map[string]CoverRow{}
		for _, r := range p.Cover(ctx, plan, *it, snap.Catalog, snap.Traffic) {
			m[fmt.Sprintf("%d|%s", r.AccountID, r.Model)] = r
		}
		idx[plan.ID] = m
	}
	return idx
}

// coverFor 主动检测项对某个对象的覆盖：返回负责的计划，或者没覆盖的原因
func coverFor(st Setup, idx coverIndex, item Item, s Subject) (*Plan, string) {
	why := "还没有检测计划覆盖这个平台和模型"
	for i := range st.Plans {
		p := &st.Plans[i]
		if p.ItemID != item.ID || p.Platform != s.Platform || !contains(p.Models, s.Model) {
			continue
		}
		row, ok := idx[p.ID][fmt.Sprintf("%d|%s", s.AccountID, s.Model)]
		if !ok {
			continue
		}
		if row.OK {
			return p, ""
		}
		why = row.Why
	}
	return nil, why
}

// JudgeItem 一个检测项对一个对象的结论（含覆盖判断）
func JudgeItem(ctx context.Context, env *JudgeEnv, st Setup, idx coverIndex, item Item, s Subject) (Verdict, *Plan) {
	det, ok := env.Registry.Get(item.Type)
	if !ok {
		return Verdict{Level: LevelNone, Short: "未知类型", Long: "检测方式 " + item.Type + " 不存在"}, nil
	}
	spec := det.Spec()
	if !spec.Active {
		if !contains(item.Platforms, s.Platform) {
			return Verdict{Level: LevelNone, Short: "未启用", Long: "这个检测项没有在该平台启用"}, nil
		}
		return det.Judge(env, item, s), nil
	}
	plan, why := coverFor(st, idx, item, s)
	if plan == nil {
		// 不在任何计划范围里，但近期有检测记录：照样给结论，并说明结果从哪来
		// （自己做过「立即体检」，或者同一个凭证的其他账号在计划里——同凭证结果共用）
		if hasRecentRuns(env, item.ID, s, checkupWindow) {
			v := det.Judge(env, item, s)
			if env.RunsOf(item.ID, s.CredKey, s.Model)[0].Trigger == "checkup" {
				v.Long += "（来自立即体检）"
			} else {
				v.Long += "（同凭证账号的结果）"
			}
			return v, nil
		}
		return Verdict{Level: LevelNone, Short: "未检测", Long: "未检测：" + why}, nil
	}
	return det.Judge(env, item, s), plan
}

const checkupWindow = 7 * 24 * time.Hour

func hasRecentRuns(env *JudgeEnv, itemID int64, s Subject, within time.Duration) bool {
	rs := env.RunsOf(itemID, s.CredKey, s.Model)
	return len(rs) > 0 && env.Now.Sub(rs[0].QueuedAt) < within
}

// buildSubjects 每个分组里要判定的 账号 × 模型：近 24 小时有流量、被计划覆盖，或者近期做过体检
func buildSubjects(st Setup, idx coverIndex, snap *Snapshot, runs []Run) []Subject {
	cat, tr := snap.Catalog, snap.Traffic
	from := tr.NowAt.Add(-24 * time.Hour).Truncate(time.Hour)
	seen := map[string]bool{}
	var out []Subject
	add := func(g int64, a *CatAccount, m string) {
		k := fmt.Sprintf("%d|%d|%s", g, a.ID, m)
		if m == "" || seen[k] {
			return
		}
		seen[k] = true
		out = append(out, Subject{GroupID: g, AccountID: a.ID, Model: m, Platform: a.Platform, CredKey: a.CredKey})
	}
	for _, k := range tr.Keys() {
		a := cat.Accounts[k.Account]
		if a == nil || cat.Groups[k.Group] == nil {
			continue
		}
		if tr.Now[k].Total() > 0 || tr.Range(k, from, tr.NowAt.Add(time.Hour)).Total() > 0 {
			add(k.Group, a, k.Model)
		}
	}
	for _, m := range idx {
		for _, r := range m {
			if !r.OK {
				continue
			}
			if a := cat.Accounts[r.AccountID]; a != nil {
				for _, g := range a.Groups {
					add(g, a, r.Model)
				}
			}
		}
	}
	byCred := map[string][]*CatAccount{}
	for _, a := range cat.Accounts {
		byCred[a.CredKey] = append(byCred[a.CredKey], a)
	}
	for _, r := range runs {
		if time.Since(r.QueuedAt) > checkupWindow {
			continue
		}
		for _, a := range byCred[r.CredKey] {
			for _, g := range a.Groups {
				add(g, a, r.Model)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GroupID != out[j].GroupID {
			return out[i].GroupID < out[j].GroupID
		}
		if out[i].AccountID != out[j].AccountID {
			return out[i].AccountID < out[j].AccountID
		}
		return out[i].Model < out[j].Model
	})
	return out
}

// DimResult 一个维度对一个 分组 × 账号 的结论 = 下面参与判定的检测项里最差的（跨模型取最差）
type DimResult struct {
	DimensionID int64  `json:"dimension_id"`
	Level       Level  `json:"level"`
	Model       string `json:"model,omitempty"`
	ItemID      int64  `json:"item_id,omitempty"`
	Short       string `json:"short"`
	Long        string `json:"long"`
	Muted       bool   `json:"muted,omitempty"`  // 已标误报，按正常处理
	NoRef       bool   `json:"no_ref,omitempty"` // 有数据但没有可比的参照
}

// verdictWeight 取「最差」时的排序：同为数据不足时，有数值的（比如有流量但没参照）排在没流量的前面
func verdictWeight(v StoredVerdict) float64 {
	w := v.Level.Rank()
	if v.Level == LevelFew && v.Value != nil {
		w += 0.1
	}
	return w
}

// aggregateDim 维度结论
func aggregateDim(reg *Registry, st Setup, d Dimension, vs []StoredVerdict) (DimResult, []StoredVerdict) {
	res := DimResult{DimensionID: d.ID, Level: LevelNone}
	var problems []StoredVerdict
	var info *StoredVerdict
	best := 0.0
	for i := range vs {
		v := vs[i]
		it := st.item(v.ItemID)
		if it == nil || it.DimensionID != d.ID || !it.Enabled {
			continue
		}
		if !it.Judged(reg.Spec(it.Type)) {
			if info == nil && v.Level == LevelInfo {
				info = &vs[i]
			}
			continue
		}
		if v.Level.Problem() {
			problems = append(problems, v)
		}
		if w := verdictWeight(v); w > best || res.ItemID == 0 && w >= best {
			best = w
			res.Level, res.Model, res.ItemID, res.Short, res.Long = v.Level, v.Model, v.ItemID, v.Short, v.Long
			res.NoRef = v.Evidence["no_reference"] == true
		}
	}
	if res.ItemID == 0 && info != nil {
		res.Level, res.Model, res.ItemID, res.Short, res.Long = LevelInfo, info.Model, info.ItemID, info.Short, info.Long
	}
	return res, problems
}

// Evaluator 定时评估
type Evaluator struct {
	Config   ConfigStore
	Setup    SetupStore
	Runs     RunStore
	Verdicts VerdictStore
	Events   EventStore
	Registry *Registry
	Planner  Planner
	Snap     *Snapshotter
	Traffic  *TrafficLoader
	Interval time.Duration

	mu         sync.Mutex
	lastRun    time.Time
	lastErr    error
	lastReport EvalReport
}

// EvalReport 一次判定的结果概要
type EvalReport struct {
	Subjects int `json:"subjects"` // 判定了多少个 分组 × 账号 × 模型
	Verdicts int `json:"verdicts"` // 产生了多少条检测项结论
	Changed  int `json:"changed"`  // 其中有多少条和上次不一样
}

// RunNow 立即判定一轮，返回概要（「重新诊断」用）
func (e *Evaluator) RunNow(ctx context.Context) (EvalReport, error) {
	if err := e.Run(ctx); err != nil {
		return EvalReport{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastReport, nil
}

func (e *Evaluator) Start(ctx context.Context) {
	if e.Interval <= 0 {
		e.Interval = 5 * time.Minute
	}
	go func() {
		// 启动后稍等一会儿，避开启动时的数据库压力
		select {
		case <-ctx.Done():
			return
		case <-time.After(20 * time.Second):
		}
		t := time.NewTicker(e.Interval)
		defer t.Stop()
		for {
			if err := e.Run(ctx); err != nil {
				log.Printf("[plus.dispatch] evaluate: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

// LastRun 上次评估时间和错误
func (e *Evaluator) LastRun() (time.Time, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastRun, e.lastErr
}

// Run 评估一次（同一实例内串行）
func (e *Evaluator) Run(ctx context.Context) (err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	defer func() { e.lastRun, e.lastErr = time.Now(), err }()

	st, err := LoadSetup(ctx, e.Config, e.Setup)
	if err != nil {
		return err
	}
	if st.Config.Mode == ModeOff {
		return nil
	}
	now := time.Now()
	if err := e.Traffic.Refresh(ctx, now); err != nil {
		return fmt.Errorf("汇总流量：%w", err)
	}
	snap, err := e.Snap.Get(ctx, true)
	if err != nil {
		return err
	}
	runs, err := e.Runs.RecentRunsForJudge(ctx, 60)
	if err != nil {
		return err
	}
	env := NewJudgeEnv(now, snap.Catalog, snap.Traffic, e.Registry, runs)
	idx := buildCoverIndex(ctx, e.Planner, st, snap)
	subjects := buildSubjects(st, idx, snap, runs)

	prev, err := e.Verdicts.LoadVerdicts(ctx)
	if err != nil {
		return err
	}
	prevBy := map[string]StoredVerdict{}
	for _, v := range prev {
		prevBy[verdictKey(v.GroupID, v.AccountID, v.Model, v.ItemID)] = v
	}
	dimOn := map[int64]bool{}
	for _, d := range st.Dims {
		dimOn[d.ID] = d.Enabled
	}
	var (
		out     []StoredVerdict
		changes []VerdictChange
	)
	for _, s := range subjects {
		for _, it := range st.Items {
			if !it.Enabled || !dimOn[it.DimensionID] {
				continue
			}
			v, _ := JudgeItem(ctx, env, st, idx, it, s)
			sv := StoredVerdict{GroupID: s.GroupID, AccountID: s.AccountID, Model: s.Model, ItemID: it.ID, Level: v.Level, Short: v.Short,
				Long: v.Long, Value: v.Value, Ref: v.Ref, Evidence: v.Evidence, Since: now, UpdatedAt: now}
			if p, ok := prevBy[verdictKey(s.GroupID, s.AccountID, s.Model, it.ID)]; ok {
				if p.Level == v.Level {
					sv.Since = p.Since
				} else if p.Level != LevelNone && v.Level != LevelNone {
					changes = append(changes, VerdictChange{GroupID: s.GroupID, AccountID: s.AccountID, Model: s.Model, ItemID: it.ID,
						From: p.Level, To: v.Level, Text: fmt.Sprintf("%s：%s", it.Name, v.Long)})
				}
			}
			out = append(out, sv)
		}
	}
	if err := e.Verdicts.ReplaceVerdicts(ctx, out, changes); err != nil {
		return err
	}
	e.lastReport = EvalReport{Subjects: len(subjects), Verdicts: len(out), Changed: len(changes)}
	return e.syncEvents(ctx, st, env, snap, out, now)
}

func verdictKey(g, a int64, m string, item int64) string {
	return fmt.Sprintf("%d|%d|%s|%d", g, a, m, item)
}

// syncEvents 根据最新判定开 / 关待处理
func (e *Evaluator) syncEvents(ctx context.Context, st Setup, env *JudgeEnv, snap *Snapshot, vs []StoredVerdict, now time.Time) error {
	want := map[string]Event{}
	cat := snap.Catalog
	accName := func(id int64) string {
		if a := cat.Accounts[id]; a != nil {
			return a.Name
		}
		return fmt.Sprintf("#%d", id)
	}
	// 1. 维度可疑 / 异常
	byGA := map[[2]int64][]StoredVerdict{}
	for _, v := range vs {
		k := [2]int64{v.GroupID, v.AccountID}
		byGA[k] = append(byGA[k], v)
	}
	for k, list := range byGA {
		for _, d := range st.Dims {
			if !d.Enabled || d.Role == RoleDisplay {
				continue
			}
			res, problems := aggregateDim(e.Registry, st, d, list)
			if !res.Level.Problem() {
				continue
			}
			var evidence []string
			for _, p := range problems {
				if p.Model != res.Model {
					continue
				}
				if it := st.item(p.ItemID); it != nil {
					evidence = append(evidence, fmt.Sprintf("%s：%s", it.Name, p.Long))
				}
			}
			key := fmt.Sprintf("dim:%d:%d:%d", k[0], k[1], d.ID)
			want[key] = Event{Kind: "dim", DedupKey: key, GroupID: k[0], AccountID: k[1], Model: res.Model, DimensionID: d.ID,
				ItemID: res.ItemID, Level: res.Level, Evidence: evidence,
				Title: fmt.Sprintf("%s 的「%s」%s", accName(k[1]), d.Name, levelName(res.Level))}
		}
	}
	// 2. 检测本身连续出错
	for _, it := range st.Items {
		if !it.Enabled || e.Registry.Active(it.Type) == nil {
			continue
		}
		seen := map[string]bool{}
		for _, a := range cat.Accounts {
			for _, p := range st.Plans {
				if p.ItemID != it.ID || p.Platform != a.Platform {
					continue
				}
				for _, m := range p.Models {
					ck := a.CredKey + "|" + m
					if seen[ck] {
						continue
					}
					rs := env.RunsOf(it.ID, a.CredKey, m)
					n := leadingErrors(rs)
					if n < st.Config.FaultRuns {
						continue
					}
					seen[ck] = true
					g := int64(0)
					if len(a.Groups) > 0 {
						g = a.Groups[0]
					}
					key := fmt.Sprintf("fault:%d:%s:%s", it.ID, a.CredKey, m)
					want[key] = Event{Kind: "fault", DedupKey: key, GroupID: g, AccountID: a.ID, Model: m, DimensionID: it.DimensionID,
						ItemID: it.ID, Level: LevelWarn,
						Title:    fmt.Sprintf("「%s」在 %s 上连续 %d 次请求出错", it.Name, accName(a.ID), n),
						Evidence: []string{rs[0].Error}}
				}
			}
		}
	}
	// 3. 有流量的模型没被计划覆盖（只看参与判定、且在这个平台已有计划的主动检测项）
	platModel := map[string]map[string]int64{}
	platTotal := map[string]int64{}
	from := snap.Traffic.NowAt.Add(-24 * time.Hour).Truncate(time.Hour)
	for _, k := range snap.Traffic.Keys() {
		a := cat.Accounts[k.Account]
		if a == nil {
			continue
		}
		n := snap.Traffic.Range(k, from, now.Add(time.Hour)).Total()
		if platModel[a.Platform] == nil {
			platModel[a.Platform] = map[string]int64{}
		}
		platModel[a.Platform][k.Model] += n
		platTotal[a.Platform] += n
	}
	for _, it := range st.Items {
		det := e.Registry.Active(it.Type)
		if !it.Enabled || det == nil || !it.Judged(det.Spec()) {
			continue
		}
		covered := map[string]map[string]bool{}
		for _, p := range st.Plans {
			if p.ItemID == it.ID && p.Enabled {
				if covered[p.Platform] == nil {
					covered[p.Platform] = map[string]bool{}
				}
				for _, m := range p.Models {
					covered[p.Platform][m] = true
				}
			}
		}
		for plat, models := range covered {
			for m, n := range platModel[plat] {
				if models[m] || platTotal[plat] == 0 {
					continue
				}
				share := float64(n) * 100 / float64(platTotal[plat])
				if share < st.Config.GapShare {
					continue
				}
				if ok, _ := det.Supports(ctx, plat, m); !ok {
					continue
				}
				key := fmt.Sprintf("gap:%d:%s:%s", it.ID, plat, m)
				want[key] = Event{Kind: "gap", DedupKey: key, Model: m, DimensionID: it.DimensionID, ItemID: it.ID, Level: LevelWarn,
					Title:    fmt.Sprintf("%s 占 %s 平台流量 %.0f%%，但「%s」没有测它", m, plat, share, it.Name),
					Evidence: nil}
			}
		}
	}

	open, err := e.Events.OpenEvents(ctx)
	if err != nil {
		return err
	}
	var resolve []string
	for _, o := range open {
		if _, ok := want[o.DedupKey]; !ok {
			resolve = append(resolve, o.DedupKey)
		}
	}
	for _, ev := range want {
		if err := e.Events.UpsertEvent(ctx, ev); err != nil {
			return err
		}
	}
	return e.Events.ResolveEvents(ctx, resolve, now)
}

func levelName(l Level) string {
	return map[Level]string{LevelOK: "正常", LevelWarn: "可疑", LevelBad: "异常", LevelFew: "数据不足", LevelNone: "未检测", LevelInfo: ""}[l]
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
