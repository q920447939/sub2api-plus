package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
)

// Auditor 操作记录（所有改动都经过它）
type Auditor struct{ Store AuditStore }

func (a Auditor) Record(ctx context.Context, actor Actor, action, targetType, targetID, summary string, before, after any) {
	email := actor.Email
	if email == "" && actor.ID > 0 {
		email = a.Store.OperatorEmail(ctx, actor.ID)
	}
	var op *int64
	if actor.ID > 0 {
		id := actor.ID
		op = &id
	}
	if err := a.Store.AddAudit(ctx, AuditEntry{OperatorID: op, Operator: email, Action: action, TargetType: targetType,
		TargetID: targetID, Summary: summary, Before: before, After: after}); err != nil {
		log.Printf("[plus.dispatch] audit: %v", err)
	}
}

// SetupService 检测配置：全局配置、维度、检测项、计划
type SetupService struct {
	Config   ConfigStore
	Setup    SetupStore
	Registry *Registry
	Planner  Planner
	Snap     *Snapshotter
	Sched    *Scheduler
	Audit    Auditor
	OnChange func() // 配置变化后（刷新钩子规则、触发评估）
}

func (s *SetupService) changed() {
	if s.OnChange != nil {
		go s.OnChange()
	}
}

// ---------- 全局配置 ----------

func (s *SetupService) GetConfig(ctx context.Context) (Config, int64, error) {
	return s.Config.LoadConfig(ctx)
}

var modeNames = map[string]string{ModeOff: "关闭", ModeObserve: "只观察", ModeManual: "人工调整"}

func (s *SetupService) SaveConfig(ctx context.Context, actor Actor, cfg Config, ver int64) (int64, error) {
	if err := cfg.normalize(); err != nil {
		return 0, err
	}
	old, _, err := s.Config.LoadConfig(ctx)
	if err != nil {
		return 0, err
	}
	nv, err := s.Config.SaveConfig(ctx, cfg, ver)
	if err != nil {
		return 0, err
	}
	summary := "修改调度台设置"
	if old.Mode != cfg.Mode {
		summary = fmt.Sprintf("档位：%s → %s", modeNames[old.Mode], modeNames[cfg.Mode])
	}
	s.Audit.Record(ctx, actor, "update", "config", "1", summary, old, cfg)
	s.changed()
	return nv, nil
}

// ---------- 维度 ----------

func (s *SetupService) SaveDimension(ctx context.Context, actor Actor, d Dimension) (Dimension, error) {
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" {
		return d, fmt.Errorf("%w：维度名称不能为空", ErrInvalid)
	}
	if !d.Role.valid() {
		return d, fmt.Errorf("%w：作用只能是 gate / score / display", ErrInvalid)
	}
	d.Gate.normalize()
	dims, err := s.Setup.ListDimensions(ctx)
	if err != nil {
		return d, err
	}
	var old *Dimension
	maxSort := 0
	for i := range dims {
		if dims[i].ID == d.ID {
			old = &dims[i]
		} else if dims[i].Name == d.Name {
			return d, fmt.Errorf("%w：已经有叫「%s」的维度", ErrInvalid, d.Name)
		}
		if dims[i].Sort > maxSort {
			maxSort = dims[i].Sort
		}
	}
	if d.ID != 0 && old == nil {
		return d, ErrNotFound
	}
	if d.ID == 0 {
		d.Sort = maxSort + 10
	}
	id, err := s.Setup.SaveDimension(ctx, d)
	if err != nil {
		return d, err
	}
	d.ID = id
	action, summary := "create", "新建维度「"+d.Name+"」"
	if old != nil {
		action, summary = "update", "修改维度「"+d.Name+"」"
	}
	s.Audit.Record(ctx, actor, action, "dimension", itoa(id), summary, old, d)
	s.changed()
	return d, nil
}

func (s *SetupService) ReorderDimensions(ctx context.Context, actor Actor, ids []int64) error {
	dims, err := s.Setup.ListDimensions(ctx)
	if err != nil {
		return err
	}
	pos := map[int64]int{}
	for i, id := range ids {
		pos[id] = (i + 1) * 10
	}
	for _, d := range dims {
		if p, ok := pos[d.ID]; ok && p != d.Sort {
			d.Sort = p
			if _, err := s.Setup.SaveDimension(ctx, d); err != nil {
				return err
			}
		}
	}
	s.Audit.Record(ctx, actor, "update", "dimension", "", "调整维度顺序", nil, ids)
	return nil
}

func (s *SetupService) DeleteDimension(ctx context.Context, actor Actor, id int64) error {
	dims, _ := s.Setup.ListDimensions(ctx)
	var old *Dimension
	for i := range dims {
		if dims[i].ID == id {
			old = &dims[i]
		}
	}
	if err := s.Setup.DeleteDimension(ctx, id); err != nil {
		return err
	}
	name := ""
	if old != nil {
		name = old.Name
	}
	s.Audit.Record(ctx, actor, "delete", "dimension", itoa(id), "删除维度「"+name+"」", old, nil)
	s.changed()
	return nil
}

// ---------- 检测项 ----------

func (s *SetupService) SaveItem(ctx context.Context, actor Actor, it Item) (Item, error) {
	it.Name = strings.TrimSpace(it.Name)
	if it.Name == "" {
		return it, fmt.Errorf("%w：检测项名称不能为空", ErrInvalid)
	}
	items, err := s.Setup.ListItems(ctx)
	if err != nil {
		return it, err
	}
	var old *Item
	for i := range items {
		if items[i].ID == it.ID && it.ID != 0 {
			old = &items[i]
		}
	}
	if it.ID != 0 {
		if old == nil {
			return it, ErrNotFound
		}
		it.Type = old.Type // 类型建好后不能改
		if old.Type == "traffic" {
			// 流量指标建好后口径固定，要换指标请新建检测项
			var op, np TrafficParams
			_ = json.Unmarshal(old.Params, &op)
			_ = json.Unmarshal(it.Params, &np)
			if np.Metric != "" && np.Metric != op.Metric {
				return it, fmt.Errorf("%w：统计口径建好后不能改，要换指标请新建检测项", ErrInvalid)
			}
			it.Params = old.Params
		}
	}
	det, ok := s.Registry.Get(it.Type)
	if !ok {
		return it, fmt.Errorf("%w：不认识的检测方式 %q", ErrInvalid, it.Type)
	}
	dims, err := s.Setup.ListDimensions(ctx)
	if err != nil {
		return it, err
	}
	found := false
	for _, d := range dims {
		found = found || d.ID == it.DimensionID
	}
	if !found {
		return it, fmt.Errorf("%w：请选择归属维度", ErrInvalid)
	}
	params, rule, err := det.NormalizeItem(it.Params, it.Rule)
	if err != nil {
		return it, err
	}
	it.Params, it.Rule = params, rule
	if det.Spec().Active {
		it.Platforms = []string{} // 主动类型的范围由计划决定
	} else {
		it.Platforms = cleanStrings(it.Platforms)
	}
	if !det.Spec().Judges {
		it.DisplayOnly = true
	}
	id, err := s.Setup.SaveItem(ctx, it)
	if err != nil {
		return it, err
	}
	it.ID = id
	action, summary := "create", fmt.Sprintf("新建检测项「%s」（%s）", it.Name, det.Spec().Name)
	if old != nil {
		action, summary = "update", "修改检测项「"+it.Name+"」"
	}
	s.Audit.Record(ctx, actor, action, "item", itoa(id), summary, old, it)
	s.changed()
	return it, nil
}

func (s *SetupService) DeleteItem(ctx context.Context, actor Actor, id int64) error {
	items, _ := s.Setup.ListItems(ctx)
	var old *Item
	for i := range items {
		if items[i].ID == id {
			old = &items[i]
		}
	}
	if old == nil {
		return ErrNotFound
	}
	if err := s.Setup.DeleteItem(ctx, id); err != nil {
		return err
	}
	s.Audit.Record(ctx, actor, "delete", "item", itoa(id), "删除检测项「"+old.Name+"」（连同它的计划和检测记录）", old, nil)
	s.changed()
	return nil
}

// ---------- 计划 ----------

func (s *SetupService) normalizePlan(ctx context.Context, p *Plan) (*Item, error) {
	items, err := s.Setup.ListItems(ctx)
	if err != nil {
		return nil, err
	}
	var item *Item
	for i := range items {
		if items[i].ID == p.ItemID {
			item = &items[i]
		}
	}
	if item == nil {
		return nil, fmt.Errorf("%w：请选择检测项", ErrInvalid)
	}
	det := s.Registry.Active(item.Type)
	if det == nil {
		return nil, fmt.Errorf("%w：「%s」不发请求，不需要计划；在检测项上选平台即可", ErrInvalid, item.Name)
	}
	p.Platform = strings.TrimSpace(p.Platform)
	if p.Platform == "" {
		return nil, fmt.Errorf("%w：请选择平台", ErrInvalid)
	}
	p.Models = cleanStrings(p.Models)
	if len(p.Models) == 0 {
		return nil, fmt.Errorf("%w：至少选一个模型", ErrInvalid)
	}
	if p.IntervalMinutes < minIntervalMinutes || p.IntervalMinutes > maxIntervalMinutes {
		return nil, fmt.Errorf("%w：频率要在 %d 到 %d 分钟之间", ErrInvalid, minIntervalMinutes, maxIntervalMinutes)
	}
	if p.TimeoutSeconds < 10 || p.TimeoutSeconds > 1800 {
		return nil, fmt.Errorf("%w：单次超时要在 10 到 1800 秒之间", ErrInvalid)
	}
	scopes, err := normalizeScopes(p.Scopes)
	if err != nil {
		return nil, err
	}
	p.Scopes, p.ExcludeIDs = scopes, uniqueIDs(p.ExcludeIDs)
	// 同一个检测项的同一个模型只能属于一个计划，避免重复花钱
	plans, err := s.Setup.ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	for _, o := range plans {
		if o.ID == p.ID || o.ItemID != p.ItemID || o.Platform != p.Platform {
			continue
		}
		for _, m := range p.Models {
			if contains(o.Models, m) {
				return nil, fmt.Errorf("%w：「%s」已经有计划在测 %s 平台的 %s，同一个检测项的同一个模型只能属于一个计划", ErrConflict, item.Name, p.Platform, m)
			}
		}
	}
	return item, nil
}

func (s *SetupService) SavePlan(ctx context.Context, actor Actor, p Plan) (Plan, error) {
	var old *Plan
	if p.ID != 0 {
		plans, err := s.Setup.ListPlans(ctx)
		if err != nil {
			return p, err
		}
		for i := range plans {
			if plans[i].ID == p.ID {
				old = &plans[i]
			}
		}
		if old == nil {
			return p, ErrNotFound
		}
		p.ItemID, p.Platform = old.ItemID, old.Platform // 检测项和平台建好后不能改
	}
	item, err := s.normalizePlan(ctx, &p)
	if err != nil {
		return p, err
	}
	id, err := s.Setup.SavePlan(ctx, p)
	if err != nil {
		return p, err
	}
	p.ID = id
	action, summary := "create", fmt.Sprintf("新建计划「%s · %s」：%s，每 %d 分钟", item.Name, p.Platform, strings.Join(p.Models, "、"), p.IntervalMinutes)
	if old != nil {
		action, summary = "update", fmt.Sprintf("修改计划「%s · %s」", item.Name, p.Platform)
	}
	s.Audit.Record(ctx, actor, action, "plan", itoa(id), summary, old, p)
	if s.Sched != nil {
		s.Sched.Wake()
	}
	s.changed()
	return p, nil
}

func (s *SetupService) DeletePlan(ctx context.Context, actor Actor, id int64) error {
	plans, _ := s.Setup.ListPlans(ctx)
	var old *Plan
	for i := range plans {
		if plans[i].ID == id {
			old = &plans[i]
		}
	}
	if old == nil {
		return ErrNotFound
	}
	if err := s.Setup.DeletePlan(ctx, id); err != nil {
		return err
	}
	s.Audit.Record(ctx, actor, "delete", "plan", itoa(id), fmt.Sprintf("删除计划 #%d（%s）", id, old.Platform), old, nil)
	s.changed()
	return nil
}

// PlanPreview 计划会测哪些账号（实时预览，不保存）
type PlanPreview struct {
	Rows        []CoverRow `json:"rows"`
	Targets     int        `json:"targets"`
	RequestsDay float64    `json:"requests_per_day"`
}

func (s *SetupService) PreviewPlan(ctx context.Context, p Plan) (PlanPreview, error) {
	if p.ID != 0 {
		plans, _ := s.Setup.ListPlans(ctx)
		for _, o := range plans {
			if o.ID == p.ID {
				p.ItemID, p.Platform = o.ItemID, o.Platform
			}
		}
	}
	p.Enabled = true
	if p.IntervalMinutes < minIntervalMinutes {
		p.IntervalMinutes = minIntervalMinutes
	}
	scopes, err := normalizeScopes(p.Scopes)
	if err != nil {
		return PlanPreview{}, err
	}
	p.Scopes = scopes
	items, err := s.Setup.ListItems(ctx)
	if err != nil {
		return PlanPreview{}, err
	}
	var item *Item
	for i := range items {
		if items[i].ID == p.ItemID {
			item = &items[i]
		}
	}
	if item == nil {
		return PlanPreview{}, fmt.Errorf("%w：请选择检测项", ErrInvalid)
	}
	snap, err := s.Snap.Get(ctx, false)
	if err != nil {
		return PlanPreview{}, err
	}
	rows := s.Planner.Cover(ctx, p, *item, snap.Catalog, snap.Traffic)
	out := PlanPreview{Rows: rows}
	for _, r := range rows {
		if r.OK && r.SameAs == 0 {
			out.Targets++
		}
	}
	perRun := 1.0
	if item.Type == "fingerprint" {
		perRun = 3
	}
	out.RequestsDay = round1(float64(out.Targets) * perRun * 1440 / float64(p.IntervalMinutes))
	return out, nil
}

// RunPlanNow 立即执行一轮（不影响原来的频率）
func (s *SetupService) RunPlanNow(ctx context.Context, actor Actor, id int64) (int, error) {
	plans, err := s.Setup.ListPlans(ctx)
	if err != nil {
		return 0, err
	}
	for _, p := range plans {
		if p.ID != id {
			continue
		}
		by := actor.ID
		n, err := s.Sched.EnqueuePlans(ctx, []Plan{p}, "manual", &by)
		if err != nil {
			return 0, err
		}
		s.Audit.Record(ctx, actor, "run", "plan", itoa(id), fmt.Sprintf("立即执行计划 #%d（排队 %d 次检测）", id, n), nil, nil)
		s.Sched.Wake()
		return n, nil
	}
	return 0, ErrNotFound
}

// RerunOne 用同样的检测项对某个账号 × 模型重测一次
func (s *SetupService) RerunOne(ctx context.Context, actor Actor, run Run) (int64, error) {
	snap, err := s.Snap.Get(ctx, false)
	if err != nil {
		return 0, err
	}
	a := snap.Catalog.Accounts[run.AccountID]
	if a == nil {
		return 0, ErrNotFound
	}
	by := actor.ID
	id, ok, err := s.Sched.Runs.Enqueue(ctx, Run{PlanID: run.PlanID, ItemID: run.ItemID, AccountID: a.ID, CredKey: a.CredKey,
		Platform: a.Platform, Model: run.Model, Trigger: "manual", TriggeredBy: &by})
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("%w：这个账号的这一项已经在排队或运行中", ErrConflict)
	}
	s.Audit.Record(ctx, actor, "run", "run", itoa(id), fmt.Sprintf("对 %s · %s 重测一次", a.Name, run.Model), nil, nil)
	s.Sched.Wake()
	return id, nil
}

func cleanStrings(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func uniqueIDs(in []int64) []int64 {
	seen := map[int64]bool{}
	out := []int64{}
	for _, id := range in {
		if id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// CheckupRequest 立即体检：对一个账号直连测一轮，不受计划范围和流量限制
type CheckupRequest struct {
	AccountID int64    `json:"account_id"`
	ItemIDs   []int64  `json:"item_ids"`
	Models    []string `json:"models"`
}

// CheckupResult 体检排队结果
type CheckupResult struct {
	Queued  int      `json:"queued"`
	Skipped []string `json:"skipped"`
}

// Checkup 立即体检：适合刚接入、还没放量的账号，先测再放量
func (s *SetupService) Checkup(ctx context.Context, actor Actor, req CheckupRequest) (CheckupResult, error) {
	var out CheckupResult
	snap, err := s.Snap.Get(ctx, true)
	if err != nil {
		return out, err
	}
	a := snap.Catalog.Accounts[req.AccountID]
	if a == nil {
		return out, ErrNotFound
	}
	if !a.Direct {
		return out, fmt.Errorf("%w：%s", ErrInvalid, a.DirectWhy)
	}
	models := cleanStrings(req.Models)
	if len(models) == 0 {
		return out, fmt.Errorf("%w：至少选一个模型", ErrInvalid)
	}
	if len(req.ItemIDs) == 0 {
		return out, fmt.Errorf("%w：至少选一个检测项", ErrInvalid)
	}
	items, err := s.Setup.ListItems(ctx)
	if err != nil {
		return out, err
	}
	plans, err := s.Setup.ListPlans(ctx)
	if err != nil {
		return out, err
	}
	want := idSet(req.ItemIDs)
	by := actor.ID
	var names []string
	for _, it := range items {
		if !want[it.ID] {
			continue
		}
		det := s.Registry.Active(it.Type)
		if det == nil || !it.Enabled {
			out.Skipped = append(out.Skipped, fmt.Sprintf("%s：不是启用中的主动检测项", it.Name))
			continue
		}
		names = append(names, it.Name)
		for _, m := range models {
			if ok, why := det.Supports(ctx, a.Platform, m); !ok {
				out.Skipped = append(out.Skipped, fmt.Sprintf("%s · %s：%s", it.Name, m, why))
				continue
			}
			var planID *int64
			for i := range plans {
				if plans[i].ItemID == it.ID && plans[i].Platform == a.Platform {
					planID = &plans[i].ID // 用计划的超时和改题；没有计划时用默认值
				}
			}
			_, ok, err := s.Sched.Runs.Enqueue(ctx, Run{PlanID: planID, ItemID: it.ID, AccountID: a.ID, CredKey: a.CredKey,
				Platform: a.Platform, Model: m, Trigger: "checkup", TriggeredBy: &by})
			if err != nil {
				return out, err
			}
			if ok {
				out.Queued++
			} else {
				out.Skipped = append(out.Skipped, fmt.Sprintf("%s · %s：已经在排队或检测中", it.Name, m))
			}
		}
	}
	if out.Queued > 0 {
		s.Audit.Record(ctx, actor, "run", "account", itoa(a.ID), fmt.Sprintf("对 %s 立即体检：%s × %s（排队 %d 次）", a.Name,
			strings.Join(names, "、"), strings.Join(models, "、"), out.Queued), nil, req)
		s.Sched.Wake()
	}
	return out, nil
}
