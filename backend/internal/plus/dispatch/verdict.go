package dispatch

import (
	"context"
	"sync"
	"time"
)

// judgeDepth 每个 检测项 × 凭证 × 模型 取最近多少次记录来判（规则里的 window 最大 50，再留出连续出错的余量）
const judgeDepth = 60

// Detection 主动检测的当前状态：检测项、最近的记录（按当前规则归类、误报已标出）、各 账号 × 模型 的结论
type Detection struct {
	Items   []Item
	Plans   []Plan
	runs    map[string][]Run // 检测项|凭证|模型 → 新 → 旧
	ignores map[string]Ignore
	reg     *Registry
	fault   int
	memo    sync.Map // key → *Verdict
}

// Item 按 id 取检测项
func (d *Detection) Item(id int64) *Item {
	for i := range d.Items {
		if d.Items[i].ID == id {
			return &d.Items[i]
		}
	}
	return nil
}

// Judging 会给出通过 / 未通过、且启用的检测项
func (d *Detection) Judging() []Item {
	var out []Item
	for _, it := range d.Items {
		if it.Enabled && d.reg.Spec(it.Type).Judges {
			out = append(out, it)
		}
	}
	return out
}

// Runs 某账号某模型在某检测项上最近的记录（新 → 旧）。账号不提供这个模型时不算（旧版计划测过没有的模型）
func (d *Detection) Runs(itemID int64, a *CatAccount, model string) []Run {
	if a == nil || !a.Supports(model) {
		return nil
	}
	return d.runs[runKeyOf(itemID, a.CredKey, model)]
}

// Verdict 结论；没测过返回 nil
func (d *Detection) Verdict(item Item, a *CatAccount, model string) *Verdict {
	if a == nil {
		return nil
	}
	k := runKeyOf(item.ID, a.CredKey, model)
	if v, ok := d.memo.Load(k); ok {
		x, _ := v.(*Verdict)
		return x
	}
	var v *Verdict
	rs := d.Runs(item.ID, a, model)
	if det, ok := d.reg.Get(item.Type); ok && len(rs) > 0 {
		v = det.Judge(item, rs)
		if d.reg.Spec(item.Type).Judges {
			v = faultVerdict(v, rs, d.fault)
		}
	}
	d.memo.Store(k, v)
	return v
}

// Ignored 这个 账号 × 模型 × 检测项 的误报标记
func (d *Detection) Ignored(itemID int64, a *CatAccount, model string) *Ignore {
	if a == nil {
		return nil
	}
	if ig, ok := d.ignores[runKeyOf(itemID, a.CredKey, model)]; ok {
		return &ig
	}
	return nil
}

// ModelsSeenIn 检测记录里出现过的模型（账号自己提供的）
func (a *CatAccount) ModelsSeenIn(d *Detection, itemID int64) []string {
	prefix := itoa(itemID) + "|" + a.CredKey + "|"
	var out []string
	for k, rs := range d.runs {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix && len(rs) > 0 && a.Supports(rs[0].Model) {
			out = append(out, rs[0].Model)
		}
	}
	return out
}

// DetectionLoader 带缓存的加载器（改了检测项、标了误报、有新记录时失效）
type DetectionLoader struct {
	Config  ConfigStore
	Setup   SetupStore
	Runs    RunStore
	Ignores IgnoreStore
	Reg     *Registry
	TTL     time.Duration

	mu   sync.Mutex
	last *Detection
	at   time.Time
}

func (l *DetectionLoader) Get(ctx context.Context) (*Detection, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ttl := l.TTL
	if ttl <= 0 {
		ttl = 20 * time.Second
	}
	if l.last != nil && time.Since(l.at) < ttl {
		return l.last, nil
	}
	cfg, _, err := l.Config.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	items, err := l.Setup.ListItems(ctx)
	if err != nil {
		return nil, err
	}
	plans, err := l.Setup.ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	runs, err := l.Runs.RecentRuns(ctx, judgeDepth)
	if err != nil {
		return nil, err
	}
	igs, err := l.Ignores.ListIgnores(ctx)
	if err != nil {
		return nil, err
	}
	d := &Detection{Items: items, Plans: plans, runs: map[string][]Run{}, ignores: map[string]Ignore{}, reg: l.Reg, fault: cfg.FaultRuns}
	for _, ig := range igs {
		d.ignores[runKeyOf(ig.ItemID, ig.CredKey, ig.Model)] = ig
	}
	byID := map[int64]Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	for _, r := range runs { // 已按 id 倒序
		it, ok := byID[r.ItemID]
		if !ok {
			continue
		}
		k := runKeyOf(r.ItemID, r.CredKey, r.Model)
		d.runs[k] = append(d.runs[k], settle(l.Reg, it, r, d.ignores[k]))
	}
	l.last, l.at = d, time.Now()
	return d, nil
}

// Invalidate 下一次重新加载
func (l *DetectionLoader) Invalidate() {
	l.mu.Lock()
	l.last = nil
	l.mu.Unlock()
}

// settle 按当前规则归类；被标为误报（标记时已有的）未通过改成 ignored
func settle(reg *Registry, it Item, r Run, ig Ignore) Run {
	det, ok := reg.Get(it.Type)
	if !ok {
		return r
	}
	o := det.Classify(it, r)
	if o == OutcomeFail && ig.UpToRun > 0 && r.ID <= ig.UpToRun {
		o = OutcomeIgnored
	}
	if o != r.Outcome {
		if ds, ok := det.(Describer); ok && r.Outcome != OutcomeError {
			r.Summary = ds.Describe(o, r.Detail)
		}
		r.Outcome = o
	}
	return r
}
