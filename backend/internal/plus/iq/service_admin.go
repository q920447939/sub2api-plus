// Package iq — [plus] 管理端用例：设置、检测任务、运行记录、检测画廊
package iq

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// AdminDeps 管理端依赖
type AdminDeps struct {
	Config      ConfigStore
	Tasks       TaskStore
	Runs        RunReader
	Queue       RunQueue
	Catalog     GroupCatalog
	Keys        KeyStore
	Provisioner KeyProvisioner
	Channel     Channel
	Client      LLMClient
	Models      func(ctx context.Context, t Target) ([]string, error)
	Signer      func(ctx context.Context) (Signer, error)
	Wake        func()
	Now         func() time.Time
}

// AdminService 管理端用例
type AdminService struct{ d AdminDeps }

func NewAdminService(d AdminDeps) *AdminService {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Wake == nil {
		d.Wake = func() {}
	}
	return &AdminService{d: d}
}

const recentPerTask = 5

// ---------- 设置 ----------

func (s *AdminService) Config(ctx context.Context) (AdminConfigView, error) {
	cfg, ver, err := s.d.Config.LoadConfig(ctx)
	if err != nil {
		return AdminConfigView{}, err
	}
	groups, err := s.d.Catalog.ListGroups(ctx)
	if err != nil {
		return AdminConfigView{}, err
	}
	return AdminConfigView{Config: cfg, Version: ver, Groups: groups, DefaultPrompt: PelicanPrompt}, nil
}

// SaveConfig 保存设置；题目改动时版本号由服务端 +1，不信任前端传入的版本
func (s *AdminService) SaveConfig(ctx context.Context, cfg Config, version int64) (int64, error) {
	if err := cfg.Validate(); err != nil {
		return 0, err
	}
	old, _, err := s.d.Config.LoadConfig(ctx)
	if err != nil {
		return 0, err
	}
	cfg.PromptVersion = old.PromptVersion
	if cfg.Prompt != old.Prompt {
		cfg.PromptVersion++
	}
	return s.d.Config.SaveConfig(ctx, cfg, version)
}

// ---------- 任务 ----------

func (s *AdminService) groupMap(ctx context.Context) (map[int64]GroupInfo, []GroupInfo, error) {
	groups, err := s.d.Catalog.ListGroups(ctx)
	if err != nil {
		return nil, nil, err
	}
	m := make(map[int64]GroupInfo, len(groups))
	for _, g := range groups {
		m[g.ID] = g
	}
	return m, groups, nil
}

// Overview 任务页：任务列表 + 统计
func (s *AdminService) Overview(ctx context.Context) (TasksOverview, error) {
	cfg, _, err := s.d.Config.LoadConfig(ctx)
	if err != nil {
		return TasksOverview{}, err
	}
	tasks, err := s.d.Tasks.ListTasks(ctx)
	if err != nil {
		return TasksOverview{}, err
	}
	gm, _, err := s.groupMap(ctx)
	if err != nil {
		return TasksOverview{}, err
	}
	ids, keyIDs := make([]int64, len(tasks)), make([]int64, len(tasks))
	for i, t := range tasks {
		ids[i], keyIDs[i] = t.ID, t.APIKeyID
	}
	recent, err := s.d.Runs.RecentRuns(ctx, ids, recentPerTask, false)
	if err != nil {
		return TasksOverview{}, err
	}
	keys, err := s.d.Keys.KeyInfos(ctx, keyIDs)
	if err != nil {
		return TasksOverview{}, err
	}
	active, err := s.d.Runs.ActiveByTask(ctx)
	if err != nil {
		return TasksOverview{}, err
	}
	var all []Run
	for _, rs := range recent {
		all = append(all, rs...)
	}
	views, err := s.runViews(ctx, all, gm, false)
	if err != nil {
		return TasksOverview{}, err
	}
	viewByID := make(map[int64]RunView, len(views))
	for _, v := range views {
		viewByID[v.ID] = v
	}

	out := TasksOverview{Tasks: make([]TaskView, 0, len(tasks))}
	for _, t := range tasks {
		g := gm[t.GroupID]
		tv := TaskView{Task: t, GroupName: groupName(g, t.GroupID), Platform: g.Platform, Recent: []RunView{}}
		if k, ok := keys[t.APIKeyID]; ok {
			tv.Key = &k
		}
		for _, r := range recent[t.ID] {
			tv.Recent = append(tv.Recent, viewByID[r.ID])
		}
		if st, ok := active[t.ID]; ok {
			tv.Active = &st
		}
		out.Tasks = append(out.Tasks, tv)
		out.Stats.TotalTasks++
		if t.Enabled {
			out.Stats.EnabledTasks++
			out.Stats.DailyRuns += 1440 / t.IntervalMinutes
		}
	}
	st, err := s.d.Runs.Stats(ctx, s.d.Now().Add(-24*time.Hour))
	if err != nil {
		return TasksOverview{}, err
	}
	if st.Finished > 0 {
		rate := float64(st.Success) / float64(st.Finished)
		out.Stats.SuccessRate24h = &rate
	}
	out.Stats.Running, out.Stats.Queued = st.Running, st.Queued
	out.Stats.Concurrency, out.Stats.SchedulerOn = cfg.Concurrency, cfg.Enabled
	return out, nil
}

func groupName(g GroupInfo, id int64) string {
	if g.Name != "" {
		return g.Name
	}
	return fmt.Sprintf("分组 #%d（已删除或停用）", id)
}

func (in *TaskInput) normalize() error {
	in.Model = strings.TrimSpace(in.Model)
	if in.Thinking == "" {
		in.Thinking = ThinkingOff
	}
	switch {
	case in.GroupID <= 0:
		return invalid("请选择分组")
	case in.Model == "" || len(in.Model) > 200:
		return invalid("请填写检测模型")
	case in.KeyMode != KeyAuto && in.KeyMode != KeyManual:
		return invalid("请选择探测 Key 来源")
	case in.KeyMode == KeyManual && in.APIKeyID <= 0:
		return invalid("请选择一个已有 Key")
	case !in.Format.valid():
		return invalid("请求格式不正确")
	case !in.Thinking.valid():
		return invalid("思考强度不正确")
	case in.IntervalMinutes < MinInterval || in.IntervalMinutes > MaxInterval:
		return invalid(fmt.Sprintf("检测间隔需在 %d-%d 分钟之间", MinInterval, MaxInterval))
	case in.Retention < 1 || in.Retention > MaxRetention:
		return invalid(fmt.Sprintf("保留条数需在 1-%d 之间", MaxRetention))
	}
	return nil
}

// AutoKeyName 自动生成的 Key 名称：前缀 + 分组名
func AutoKeyName(prefix, groupName string) string { return prefix + groupName }

// resolveKey 按来源确定任务使用的 Key。自动 Key 挂在 owner（任务创建人）名下。
func (s *AdminService) resolveKey(ctx context.Context, in TaskInput, group GroupInfo, owner int64) (int64, error) {
	if in.KeyMode == KeyManual {
		_, keyGroup, err := s.d.Keys.ResolveKey(ctx, in.APIKeyID)
		if errors.Is(err, ErrKeyUnavailable) {
			return 0, invalid("所选 Key 不可用（已删除、停用或过期）")
		}
		if err != nil {
			return 0, err
		}
		if keyGroup != in.GroupID {
			return 0, invalid("所选 Key 不属于该分组")
		}
		return in.APIKeyID, nil
	}
	cfg, _, err := s.d.Config.LoadConfig(ctx)
	if err != nil {
		return 0, err
	}
	id, err := s.d.Provisioner.EnsureGroupKey(ctx, owner, group.ID, AutoKeyName(cfg.KeyPrefix, group.Name))
	if err != nil {
		return 0, invalid("自动生成 Key 失败：" + err.Error())
	}
	return id, nil
}

func (s *AdminService) CreateTask(ctx context.Context, in TaskInput, adminID int64) (int64, error) {
	if err := in.normalize(); err != nil {
		return 0, err
	}
	gm, _, err := s.groupMap(ctx)
	if err != nil {
		return 0, err
	}
	g, ok := gm[in.GroupID]
	if !ok {
		return 0, invalid("分组不存在或已停用")
	}
	keyID, err := s.resolveKey(ctx, in, g, adminID)
	if err != nil {
		return 0, err
	}
	id, err := s.d.Tasks.CreateTask(ctx, Task{
		GroupID: in.GroupID, Model: in.Model, KeyMode: in.KeyMode, APIKeyID: keyID, Format: in.Format, Thinking: in.Thinking,
		IntervalMinutes: in.IntervalMinutes, Retention: in.Retention, Enabled: in.Enabled, CreatedBy: adminID,
	})
	if errors.Is(err, ErrDuplicateTask) {
		return 0, invalid("该分组已有同名模型的任务")
	}
	if err == nil && in.Enabled {
		s.d.Wake() // 新任务 next_run_at = NOW，马上跑第一次
	}
	return id, err
}

// UpdateTask 分组和创建人不可改；切换为自动 Key 时仍挂在创建人名下
func (s *AdminService) UpdateTask(ctx context.Context, id int64, in TaskInput) error {
	cur, err := s.d.Tasks.GetTask(ctx, id)
	if err != nil {
		return err
	}
	in.GroupID = cur.GroupID
	if err := in.normalize(); err != nil {
		return err
	}
	keyID := cur.APIKeyID
	if in.KeyMode != cur.KeyMode || (in.KeyMode == KeyManual && in.APIKeyID != cur.APIKeyID) {
		gm, _, err := s.groupMap(ctx)
		if err != nil {
			return err
		}
		g, ok := gm[cur.GroupID]
		if !ok {
			return invalid("分组不存在或已停用")
		}
		if keyID, err = s.resolveKey(ctx, in, g, cur.CreatedBy); err != nil {
			return err
		}
	}
	err = s.d.Tasks.UpdateTask(ctx, Task{
		ID: id, Model: in.Model, KeyMode: in.KeyMode, APIKeyID: keyID, Format: in.Format, Thinking: in.Thinking,
		IntervalMinutes: in.IntervalMinutes, Retention: in.Retention, Enabled: in.Enabled,
	})
	if errors.Is(err, ErrDuplicateTask) {
		return invalid("该分组已有同名模型的任务")
	}
	if err == nil {
		_ = s.d.Queue.Prune(ctx, id, in.Retention)
	}
	return err
}

func (s *AdminService) DeleteTask(ctx context.Context, id int64) error {
	return s.d.Tasks.DeleteTask(ctx, id)
}

// RunTasks 手动检测：taskID>0 单个；groupID>0 整组；都为 0 时全部（含停用的任务）。返回新加入队列的数量。
func (s *AdminService) RunTasks(ctx context.Context, taskID, groupID, adminID int64) (queued, skipped int, err error) {
	cfg, _, err := s.d.Config.LoadConfig(ctx)
	if err != nil {
		return 0, 0, err
	}
	var targets []Task
	if taskID > 0 {
		t, err := s.d.Tasks.GetTask(ctx, taskID)
		if err != nil {
			return 0, 0, err
		}
		targets = []Task{*t}
	} else {
		all, err := s.d.Tasks.ListTasks(ctx)
		if err != nil {
			return 0, 0, err
		}
		for _, t := range all {
			if (groupID > 0 && t.GroupID == groupID) || (groupID == 0 && t.Enabled) {
				targets = append(targets, t)
			}
		}
	}
	by := adminID
	for _, t := range targets {
		id, err := s.d.Queue.Enqueue(ctx, t, TriggerManual, &by, cfg.PromptVersion)
		if err != nil {
			return queued, skipped, err
		}
		if id > 0 {
			queued++
		} else {
			skipped++
		}
	}
	if queued > 0 {
		s.d.Wake()
	}
	return queued, skipped, nil
}

// TestTask 测试连接：用草稿配置发一句话，不落库
func (s *AdminService) TestTask(ctx context.Context, in TaskInput, adminID int64) (TestResult, error) {
	if in.Thinking == "" {
		in.Thinking = ThinkingOff
	}
	in.Model = strings.TrimSpace(in.Model)
	if in.GroupID <= 0 || in.Model == "" {
		return TestResult{}, invalid("请先选择分组并填写模型")
	}
	target, err := s.draftTarget(ctx, in, adminID)
	if err != nil {
		return TestResult{}, err
	}
	tctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	start := s.d.Now()
	comp, err := s.d.Client.Complete(tctx, CompletionRequest{Target: target, Prompt: "请只回复两个字母：OK", MaxTokens: 256, Thinking: ThinkingOff})
	res := TestResult{LatencyMs: int(s.d.Now().Sub(start) / time.Millisecond), ClientRequestID: comp.ClientRequestID, Reply: truncate(strings.TrimSpace(comp.Text), 200)}
	if err != nil {
		res.Message = truncate(err.Error(), 500)
		return res, nil
	}
	res.OK = true
	if res.Reply == "" {
		res.Message = "请求成功，但模型没有返回正文"
	}
	return res, nil
}

// draftTarget 用草稿解析请求目标；自动 Key 模式只查找、不创建
func (s *AdminService) draftTarget(ctx context.Context, in TaskInput, adminID int64) (Target, error) {
	cfg, _, err := s.d.Config.LoadConfig(ctx)
	if err != nil {
		return Target{}, err
	}
	gm, _, err := s.groupMap(ctx)
	if err != nil {
		return Target{}, err
	}
	g, ok := gm[in.GroupID]
	if !ok {
		return Target{}, invalid("分组不存在或已停用")
	}
	keyID := in.APIKeyID
	if in.KeyMode == KeyAuto {
		id, found, err := s.d.Provisioner.FindGroupKey(ctx, adminID, g.ID, AutoKeyName(cfg.KeyPrefix, g.Name))
		if err != nil {
			return Target{}, err
		}
		if !found {
			return Target{}, invalid("自动 Key 还没有生成：保存任务时会自动创建，也可以先选择一个已有 Key 测试")
		}
		keyID = id
	}
	if keyID <= 0 {
		return Target{}, invalid("请选择一个 Key")
	}
	t, err := s.d.Channel.Resolve(ctx, Task{GroupID: g.ID, Model: in.Model, APIKeyID: keyID, Format: in.Format}, g, cfg)
	if err != nil {
		return Target{}, invalid(err.Error())
	}
	return t, nil
}

// KeyChoices 新建 / 编辑任务时的 Key 选项
func (s *AdminService) KeyChoices(ctx context.Context, groupID, adminID int64) (KeyChoices, error) {
	cfg, _, err := s.d.Config.LoadConfig(ctx)
	if err != nil {
		return KeyChoices{}, err
	}
	gm, _, err := s.groupMap(ctx)
	if err != nil {
		return KeyChoices{}, err
	}
	g, ok := gm[groupID]
	if !ok {
		return KeyChoices{}, invalid("分组不存在或已停用")
	}
	keys, err := s.d.Keys.ListGroupKeys(ctx, groupID)
	if err != nil {
		return KeyChoices{}, err
	}
	out := KeyChoices{AutoName: AutoKeyName(cfg.KeyPrefix, g.Name), Keys: keys}
	if id, found, err := s.d.Provisioner.FindGroupKey(ctx, adminID, groupID, out.AutoName); err == nil && found {
		out.AutoKeyID = id
	}
	return out, nil
}

// Models 模型候选：用草稿里的 Key 读网关 /v1/models
func (s *AdminService) Models(ctx context.Context, in TaskInput, adminID int64) ([]string, error) {
	in.Model = "-"
	t, err := s.draftTarget(ctx, in, adminID)
	if err != nil {
		return nil, err
	}
	mctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	models, err := s.d.Models(mctx, t)
	if err != nil {
		return nil, invalid("读取模型列表失败：" + err.Error())
	}
	return models, nil
}

// ---------- 运行记录 ----------

func (s *AdminService) runViews(ctx context.Context, runs []Run, gm map[int64]GroupInfo, withKey bool) ([]RunView, error) {
	var accIDs, keyIDs []int64
	for _, r := range runs {
		for _, h := range r.AccountChain {
			accIDs = append(accIDs, h.AccountID)
		}
		if r.APIKeyID != nil {
			keyIDs = append(keyIDs, *r.APIKeyID)
		}
	}
	names, err := s.d.Catalog.AccountNames(ctx, accIDs)
	if err != nil {
		return nil, err
	}
	keys := map[int64]KeyOption{}
	if withKey {
		if keys, err = s.d.Keys.KeyInfos(ctx, keyIDs); err != nil {
			return nil, err
		}
	}
	signer, err := s.d.Signer(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RunView, len(runs))
	for i, r := range runs {
		v := RunView{Run: r, GroupName: groupName(gm[r.GroupID], r.GroupID), Chain: make([]HopView, len(r.AccountChain))}
		for j, h := range r.AccountChain {
			v.Chain[j] = HopView{AccountHop: h, AccountName: names[h.AccountID]}
		}
		if r.HasHTML {
			v.PreviewURL = signer.URL(r.ID, ScopeAdmin)
		}
		if r.APIKeyID != nil {
			if k, ok := keys[*r.APIKeyID]; ok {
				v.KeyName, v.KeyMasked = k.Name, k.Masked
			}
		}
		if !withKey {
			v.RawExcerpt = "" // 列表不带原始输出，详情接口再取
		}
		out[i] = v
	}
	return out, nil
}

func (s *AdminService) ListRuns(ctx context.Context, f RunFilter) ([]RunView, int64, error) {
	runs, total, err := s.d.Runs.ListRuns(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	gm, _, err := s.groupMap(ctx)
	if err != nil {
		return nil, 0, err
	}
	views, err := s.runViews(ctx, runs, gm, false)
	return views, total, err
}

func (s *AdminService) GetRun(ctx context.Context, id int64) (RunView, error) {
	r, err := s.d.Runs.GetRun(ctx, id)
	if err != nil {
		return RunView{}, err
	}
	gm, _, err := s.groupMap(ctx)
	if err != nil {
		return RunView{}, err
	}
	views, err := s.runViews(ctx, []Run{*r}, gm, true)
	if err != nil {
		return RunView{}, err
	}
	return views[0], nil
}

// RunSource 管理端查看 / 下载源码
func (s *AdminService) RunSource(ctx context.Context, id int64) (string, error) {
	html, _, _, err := s.d.Runs.RunHTML(ctx, id)
	return html, err
}

// ---------- 检测画廊 ----------

// Gallery 和用户端同样的横排布局，但包含失败记录和已停用的任务
func (s *AdminService) Gallery(ctx context.Context, groupID int64, perTask int) ([]AdminGalleryGroup, error) {
	if perTask <= 0 || perTask > 100 {
		perTask = 24
	}
	tasks, err := s.d.Tasks.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	gm, groups, err := s.groupMap(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := s.d.Catalog.AccountCounts(ctx)
	if err != nil {
		return nil, err
	}
	byGroup := map[int64][]Task{}
	var ids []int64
	for _, t := range tasks {
		if groupID > 0 && t.GroupID != groupID {
			continue
		}
		byGroup[t.GroupID] = append(byGroup[t.GroupID], t)
		ids = append(ids, t.ID)
	}
	recent, err := s.d.Runs.RecentRuns(ctx, ids, perTask, false)
	if err != nil {
		return nil, err
	}
	var all []Run
	for _, rs := range recent {
		all = append(all, rs...)
	}
	views, err := s.runViews(ctx, all, gm, false)
	if err != nil {
		return nil, err
	}
	byTask := map[int64][]RunView{}
	for _, v := range views {
		byTask[v.TaskID] = append(byTask[v.TaskID], v)
	}

	// 已停用 / 删除的分组下也可能还有任务，补到列表末尾
	ordered := append([]GroupInfo{}, groups...)
	for gid := range byGroup {
		if _, ok := gm[gid]; !ok {
			ordered = append(ordered, GroupInfo{ID: gid, Name: groupName(GroupInfo{}, gid)})
		}
	}
	out := []AdminGalleryGroup{}
	for _, g := range ordered {
		gt := byGroup[g.ID]
		if len(gt) == 0 {
			continue
		}
		sort.Slice(gt, func(i, j int) bool { return gt[i].Model < gt[j].Model })
		row := AdminGalleryGroup{GroupInfo: g, AccountCount: counts[g.ID]}
		for _, t := range gt {
			runs := byTask[t.ID]
			sort.Slice(runs, func(i, j int) bool { return runs[i].ID > runs[j].ID })
			if runs == nil {
				runs = []RunView{}
			}
			row.Tasks = append(row.Tasks, AdminGalleryTask{TaskID: t.ID, Model: t.Model, Enabled: t.Enabled, IntervalMinutes: t.IntervalMinutes, NextRunAt: t.NextRunAt, Runs: runs})
		}
		out = append(out, row)
	}
	return out, nil
}
