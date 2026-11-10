package dispatch

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// QueryService 页面查询（只读）
type QueryService struct {
	Boards   BoardLoader
	Runs     RunStore
	Verdicts VerdictStore
	EventDB  EventStore
	Adjust   AdjustStore
	Traffic  TrafficStore
	Registry *Registry
	Planner  Planner
	Eval     *Evaluator
	Library  *FingerprintLibrary
}

// ---------- 元数据 ----------

// Meta 页面需要的注册表：检测方式、指标库、判分方式、平台和模型、维度、检测项、计划
func (q *QueryService) Meta(ctx context.Context) (map[string]any, error) {
	b, err := q.Boards.Load(ctx)
	if err != nil {
		return nil, err
	}
	cat := b.Snap.Catalog
	// 平台 → 模型（按平台近 24 小时流量从高到低）
	type mshare struct {
		Model string  `json:"model"`
		Share float64 `json:"share"`
	}
	platModels := map[string]map[string]int64{}
	platTotal := map[string]int64{}
	platforms := map[string]bool{}
	for _, g := range cat.Groups {
		platforms[g.Platform] = true
	}
	for _, a := range cat.Accounts {
		platforms[a.Platform] = true
	}
	from := b.Snap.Traffic.NowAt.Add(-24 * time.Hour).Truncate(time.Hour)
	for _, k := range b.Snap.Traffic.Keys() {
		a := cat.Accounts[k.Account]
		if a == nil {
			continue
		}
		n := b.Snap.Traffic.Range(k, from, time.Now().Add(time.Hour)).Total()
		if platModels[a.Platform] == nil {
			platModels[a.Platform] = map[string]int64{}
		}
		platModels[a.Platform][k.Model] += n
		platTotal[a.Platform] += n
	}
	models := map[string][]mshare{}
	for p := range platforms {
		list := []mshare{}
		for m, n := range platModels[p] {
			s := 0.0
			if platTotal[p] > 0 {
				s = round1(float64(n) * 100 / float64(platTotal[p]))
			}
			list = append(list, mshare{m, s})
		}
		// 计划里已经选过的模型也列出来（没流量时）
		for _, pl := range b.Setup.Plans {
			if pl.Platform == p {
				for _, m := range pl.Models {
					found := false
					for _, x := range list {
						found = found || x.Model == m
					}
					if !found {
						list = append(list, mshare{m, 0})
					}
				}
			}
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].Share != list[j].Share {
				return list[i].Share > list[j].Share
			}
			return list[i].Model < list[j].Model
		})
		models[p] = list
	}
	var plist []string
	for p := range platforms {
		plist = append(plist, p)
	}
	sort.Strings(plist)
	groups := make([]map[string]any, 0, len(cat.GroupIDs))
	for _, id := range cat.GroupIDs {
		g := cat.Groups[id]
		groups = append(groups, map[string]any{"id": g.ID, "name": g.Name, "platform": g.Platform, "account_ids": g.Accounts})
	}
	accounts := make([]map[string]any, 0, len(cat.Accounts))
	ids := make([]int64, 0, len(cat.Accounts))
	for id := range cat.Accounts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		a := cat.Accounts[id]
		accounts = append(accounts, map[string]any{"id": a.ID, "name": a.Name, "platform": a.Platform, "group_ids": a.Groups,
			"direct": a.Direct, "direct_why": a.DirectWhy, "base_url": a.BaseURL})
	}
	metricList := make([]map[string]any, 0, len(metrics))
	for _, m := range metrics {
		metricList = append(metricList, map[string]any{"key": m.Key, "name": m.Name, "unit": m.Unit, "better": m.Better, "desc": m.Desc, "counted": m.Counted, "rule": m.Rule})
	}
	lib := map[string]any{"known": false}
	if ms := q.Library.Models(); ms != nil {
		var list []string
		for _, v := range ms {
			list = append(list, v)
		}
		sort.Strings(list)
		lib = map[string]any{"known": true, "models": list}
	}
	last, lastErr := q.Eval.LastRun()
	evalErr := ""
	if lastErr != nil {
		evalErr = lastErr.Error()
	}
	return map[string]any{
		"config": b.Setup.Config, "types": q.Registry.Specs(), "scopes": ScopeSpecs(), "metrics": metricList, "matchers": MatcherOptions(),
		"references": referenceNames, "platforms": plist, "models": models, "groups": groups, "accounts": accounts,
		"dimensions": b.Setup.Dims, "items": b.Setup.Items, "plans": b.Setup.Plans,
		"fingerprint_library": lib, "last_eval": last, "last_eval_error": evalErr,
	}, nil
}

// ---------- 总览 ----------

// OverviewRow 总览里的一行（分组 × 账号）
type OverviewRow struct {
	AccountID  int64               `json:"account_id"`
	Name       string              `json:"name"`
	BaseURL    string              `json:"base_url"`
	Rate       float64             `json:"rate"`
	Serving    bool                `json:"serving"`
	Status     string              `json:"status"`
	AlsoIn     []string            `json:"also_in"`
	Share      float64             `json:"share"`
	Dims       map[int64]DimResult `json:"dims"`
	Worst      Level               `json:"worst"`
	OneLine    string              `json:"one_line"`
	Adjustment *Adjustment         `json:"adjustment,omitempty"`
	Suggest    *SuggestRow         `json:"suggest,omitempty"`
	IsNew      bool                `json:"is_new"` // 3 天内新接入
	Direct     bool                `json:"direct"` // 能否直连体检
}

// OverviewGroup 总览里的一个分组
type OverviewGroup struct {
	ID       int64              `json:"id"`
	Name     string             `json:"name"`
	Platform string             `json:"platform"`
	Models   map[string]float64 `json:"models"`
	Requests int64              `json:"requests_24h"`
	Rows     []OverviewRow      `json:"rows"`
	Summary  string             `json:"suggest_summary"`
}

func (q *QueryService) Overview(ctx context.Context, platform string) (map[string]any, error) {
	b, err := q.Boards.Load(ctx)
	if err != nil {
		return nil, err
	}
	cat := b.Snap.Catalog
	counts := map[Level]int{}
	var groups []OverviewGroup
	for _, gid := range cat.GroupIDs {
		g := cat.Groups[gid]
		if platform != "" && g.Platform != platform {
			continue
		}
		shares, total := b.Snap.Traffic.GroupShares(gid)
		sug := b.Simulate(gid)
		sugBy := map[int64]SuggestRow{}
		for _, r := range sug.Rows {
			sugBy[r.AccountID] = r
		}
		og := OverviewGroup{ID: g.ID, Name: g.Name, Platform: g.Platform, Models: b.Snap.Traffic.GroupModelShares(gid), Requests: total, Summary: sug.Summary}
		for _, aid := range g.Accounts {
			a := cat.Accounts[aid]
			res := b.DimResults(gid, aid, "")
			row := OverviewRow{AccountID: aid, Name: a.Name, BaseURL: a.BaseURL, Rate: a.Rate, Serving: a.Serving(), Status: a.Status,
				Share: round1(shares[aid]), Dims: res, Worst: b.Worst(res), OneLine: b.OneLine(res), IsNew: a.IsNew(b.NewDays(g.Platform), time.Now()), Direct: a.Direct}
			if row.Worst == LevelFew && row.Share == 0 {
				if row.IsNew {
					row.OneLine = "新接入，暂无流量"
				} else {
					row.OneLine = "近 24 小时无流量"
				}
			}
			for _, og2 := range a.Groups {
				if og2 != gid {
					if x := cat.Groups[og2]; x != nil {
						row.AlsoIn = append(row.AlsoIn, x.Name)
					}
				}
			}
			if adj, ok := b.Adjust[[2]int64{gid, aid}]; ok {
				a2 := adj
				row.Adjustment = &a2
			}
			if s, ok := sugBy[aid]; ok && s.Action != "normal" && !s.Settled {
				s2 := s
				row.Suggest = &s2
			}
			counts[row.Worst]++
			og.Rows = append(og.Rows, row)
		}
		sort.SliceStable(og.Rows, func(i, j int) bool { return og.Rows[i].Share > og.Rows[j].Share })
		groups = append(groups, og)
	}
	faults := 0
	for _, e := range b.Open {
		if e.Kind == "fault" && e.State == EventTodo {
			faults++
		}
	}
	return map[string]any{
		"dimensions": b.Setup.Dims, "groups": groups, "mode": b.Setup.Config.Mode, "counts": counts, "faults": faults,
		"coverage_gaps": b.CoverageGaps(), "as_of": b.Snap.At, "adjustments": len(b.Adjust),
	}, nil
}

// ---------- 单个账号 ----------

func (q *QueryService) AccountDetail(ctx context.Context, groupID, accountID int64, model string) (map[string]any, error) {
	b, err := q.Boards.Load(ctx)
	if err != nil {
		return nil, err
	}
	cat := b.Snap.Catalog
	a := cat.Accounts[accountID]
	if a != nil && groupID == 0 {
		// 没指定分组（例如从账号列表的标签点进来）：默认看它所在的第一个分组
		for _, gid := range a.Groups {
			if cat.Groups[gid] != nil {
				groupID = gid
				break
			}
		}
	}
	g := cat.Groups[groupID]
	if a == nil || g == nil {
		return nil, ErrNotFound
	}
	models := b.Models(groupID, accountID)
	modelInfo := make([]map[string]any, 0, len(models))
	for _, m := range models {
		modelInfo = append(modelInfo, map[string]any{"model": m, "worst": b.Worst(b.DimResults(groupID, accountID, m))})
	}
	if model == "" || !contains(models, model) {
		model = ""
		// 默认看最差的模型
		worst := -1.0
		for _, m := range models {
			if r := b.Worst(b.DimResults(groupID, accountID, m)).Rank(); r > worst {
				worst, model = r, m
			}
		}
	}
	runs, err := q.Runs.RecentRunsForJudge(ctx, 60)
	if err != nil {
		return nil, err
	}
	env := NewJudgeEnv(time.Now(), cat, b.Snap.Traffic, q.Registry, runs)
	idx := buildCoverIndex(ctx, q.Planner, b.Setup, b.Snap)
	s := Subject{GroupID: groupID, AccountID: accountID, Model: model, Platform: a.Platform, CredKey: a.CredKey}

	vByItem := map[int64]StoredVerdict{}
	for _, v := range b.Verdicts[[2]int64{groupID, accountID}] {
		if v.Model == model {
			vByItem[v.ItemID] = v
		}
	}
	res := b.DimResults(groupID, accountID, model)
	var dims []map[string]any
	for _, d := range b.Setup.Dims {
		if !d.Enabled {
			continue
		}
		var items []map[string]any
		for _, it := range b.Setup.Items {
			if it.DimensionID != d.ID || !it.Enabled {
				continue
			}
			spec := q.Registry.Spec(it.Type)
			v, ok := vByItem[it.ID]
			var verdict Verdict
			var plan *Plan
			if ok {
				verdict = Verdict{Level: v.Level, Short: v.Short, Long: v.Long, Value: v.Value, Ref: v.Ref, Evidence: v.Evidence}
				_, plan = JudgeItem(ctx, env, b.Setup, idx, it, s)
			} else if model != "" {
				verdict, plan = JudgeItem(ctx, env, b.Setup, idx, it, s)
			} else {
				verdict = Verdict{Level: LevelNone, Short: "未检测", Long: "这个账号在本组还没有流量，也没有计划覆盖"}
			}
			entry := map[string]any{"item": it, "spec": spec, "verdict": verdict, "judged": it.Judged(spec)}
			if plan != nil {
				entry["plan"] = plan
			}
			if spec.Active {
				recent := env.RunsOf(it.ID, a.CredKey, model)
				if len(recent) > 10 {
					recent = recent[:10]
				}
				det := q.Registry.Active(it.Type)
				list := make([]map[string]any, 0, len(recent))
				for _, r := range recent {
					list = append(list, map[string]any{"id": r.ID, "t": r.QueuedAt, "outcome": det.Classify(it, r), "summary": r.Summary, "detail": r.Detail})
				}
				entry["runs"] = list
				if len(recent) > 0 && plan != nil {
					entry["next_run"] = plan.NextRunAt
				}
			}
			if it.Type == "traffic" && model != "" {
				entry["series"] = q.trafficSeries(env, it, s, 24)
				if m, _, ok := trafficItem(it); ok {
					entry["metric"] = map[string]any{"key": m.Key, "name": m.Name, "unit": m.Unit, "desc": m.Desc, "better": m.Better}
				}
			}
			items = append(items, entry)
		}
		dims = append(dims, map[string]any{"dimension": d, "result": res[d.ID], "items": items})
	}
	changes, err := q.Verdicts.ListChanges(ctx, accountID, time.Now().Add(-48*time.Hour), 50)
	if err != nil {
		return nil, err
	}
	logs, err := q.Adjust.ListAdjustmentLogs(ctx, groupID, accountID, 10)
	if err != nil {
		return nil, err
	}
	var also []map[string]any
	for _, gid := range a.Groups {
		if x := cat.Groups[gid]; x != nil {
			also = append(also, map[string]any{"id": x.ID, "name": x.Name})
		}
	}
	out := map[string]any{
		"account": a, "group": g, "groups": also, "aliases": cat.Aliases(accountID), "models": modelInfo, "model": model,
		"share": round1(b.Shares(groupID)[accountID]), "dims": dims, "worst": b.Worst(b.DimResults(groupID, accountID, "")),
		"one_line": b.OneLine(b.DimResults(groupID, accountID, "")), "changes": changes, "adjust_logs": logs,
		"item_names": itemNames(b.Setup.Items),
	}
	if adj, ok := b.Adjust[[2]int64{groupID, accountID}]; ok {
		out["adjustment"] = adj
	}
	for _, r := range b.Simulate(groupID).Rows {
		if r.AccountID == accountID {
			out["suggest"] = r
		}
	}
	return out, nil
}

func itemNames(items []Item) map[int64]string {
	m := map[int64]string{}
	for _, it := range items {
		m[it.ID] = it.Name
	}
	return m
}

// trafficSeries 最近 n 个小时：值、参照、可疑线
func (q *QueryService) trafficSeries(env *JudgeEnv, it Item, s Subject, n int) []map[string]any {
	m, r, ok := trafficItem(it)
	if !ok {
		return nil
	}
	ref := ReferenceFor(env, it, s)
	k := TKey{s.GroupID, s.AccountID, s.Model}
	base := env.Traffic.NowAt.Truncate(time.Hour)
	out := make([]map[string]any, 0, n)
	for i := n - 1; i >= 0; i-- {
		b := base.Add(-time.Duration(i) * time.Hour)
		pt := map[string]any{"t": b}
		if a, ok := env.Traffic.Hourly[k][b.Unix()]; ok && sampleOK(r, a) {
			if v, ok := m.Value(a); ok {
				pt["v"] = v
			}
		}
		if ref != nil {
			if rv, ok := ref.Value(b.Unix(), false); ok {
				pt["ref"], pt["warn"] = rv, r.threshold(m, rv, false)
			}
		}
		out = append(out, pt)
	}
	return out
}

// ---------- 指标对比 ----------

func (q *QueryService) Compare(ctx context.Context, groupID int64, model string, itemID int64, rng string, highlight int64) (map[string]any, error) {
	b, err := q.Boards.Load(ctx)
	if err != nil {
		return nil, err
	}
	g := b.Snap.Catalog.Groups[groupID]
	it := b.Setup.item(itemID)
	if g == nil || it == nil {
		return nil, ErrNotFound
	}
	if it.Type == "graded" {
		return q.compareGraded(ctx, b, g, model, *it, highlight)
	}
	m, r, ok := trafficItem(*it)
	if !ok {
		return nil, fmt.Errorf("%w：这个检测项不能做横向对比", ErrInvalid)
	}
	hours := map[string]int{"24h": 24, "7d": 168, "30d": 720}[rng]
	if hours == 0 {
		rng, hours = "24h", 24
	}
	now := time.Now()
	start := now.Truncate(time.Hour).Add(-time.Duration(hours-1) * time.Hour)
	hourly, err := q.Traffic.LoadHourly(ctx, start, 0)
	if err != nil {
		return nil, err
	}
	// 把 30 天数据放进判定环境，参照线（全站 / 历史）也能算
	ts := &TrafficSet{Hourly: hourly, Now: b.Snap.Traffic.Now, NowAt: b.Snap.Traffic.NowAt}
	for k, mm := range b.Snap.Traffic.Hourly {
		if ts.Hourly[k] == nil {
			ts.Hourly[k] = mm
		}
	}
	env := NewJudgeEnv(now, b.Snap.Catalog, ts, q.Registry, nil)
	daily := rng == "30d"
	var labels []time.Time
	if daily {
		for d := 29; d >= 0; d-- {
			labels = append(labels, now.Truncate(24*time.Hour).Add(-time.Duration(d)*24*time.Hour))
		}
	} else {
		for i := 0; i < hours; i++ {
			labels = append(labels, start.Add(time.Duration(i)*time.Hour))
		}
	}
	valueAt := func(k TKey, t time.Time) (float64, bool) {
		var a Agg
		if daily {
			a = ts.Range(k, t, t.Add(24*time.Hour))
		} else {
			a = ts.Hourly[k][t.Unix()]
		}
		if !sampleOK(TrafficRule{MinSamples: 1}, a) {
			return 0, false
		}
		return m.Value(a)
	}
	accounts := g.Accounts
	if highlight == 0 && len(accounts) > 0 {
		highlight = accounts[0]
	}
	type line struct {
		AccountID int64      `json:"account_id"`
		Name      string     `json:"name"`
		Values    []*float64 `json:"values"`
	}
	var lines []line
	var rows []map[string]any
	for _, aid := range accounts {
		a := b.Snap.Catalog.Accounts[aid]
		k := TKey{groupID, aid, model}
		ln := line{AccountID: aid, Name: a.Name}
		sum, n := 0.0, 0
		var worst *float64
		var worstAt time.Time
		for _, t := range labels {
			if v, ok := valueAt(k, t); ok {
				vv := v
				ln.Values = append(ln.Values, &vv)
				sum += v
				n++
				if worst == nil || (m.Better == "low" && v > *worst) || (m.Better == "high" && v < *worst) {
					worst, worstAt = &vv, t
				}
			} else {
				ln.Values = append(ln.Values, nil)
			}
		}
		lines = append(lines, ln)
		s := Subject{GroupID: groupID, AccountID: aid, Model: model, Platform: a.Platform, CredKey: a.CredKey}
		v := TrafficDetector{}.Judge(env, *it, s)
		row := map[string]any{"account_id": aid, "name": a.Name, "verdict": v, "requests_now": ts.Now[k].Total()}
		if n > 0 {
			row["mean"] = sum / float64(n)
		}
		if worst != nil {
			row["worst"], row["worst_at"] = *worst, worstAt
		}
		rows = append(rows, row)
	}
	out := map[string]any{"item": it, "metric": map[string]any{"key": m.Key, "name": m.Name, "unit": m.Unit, "desc": m.Desc, "better": m.Better},
		"rule": r, "labels": labels, "daily": daily, "lines": lines, "rows": rows, "highlight": highlight, "range": rng}
	if ha := b.Snap.Catalog.Accounts[highlight]; ha != nil {
		s := Subject{GroupID: groupID, AccountID: highlight, Model: model, Platform: ha.Platform, CredKey: ha.CredKey}
		if ref := ReferenceFor(env, *it, s); ref != nil {
			var refv, warn, bad []*float64
			for _, t := range labels {
				if daily {
					refv, warn, bad = append(refv, nil), append(warn, nil), append(bad, nil)
					continue
				}
				if rv, ok := ref.Value(t.Unix(), false); ok {
					a, w, bd := rv, r.threshold(m, rv, false), r.threshold(m, rv, true)
					refv, warn, bad = append(refv, &a), append(warn, &w), append(bad, &bd)
				} else {
					refv, warn, bad = append(refv, nil), append(warn, nil), append(bad, nil)
				}
			}
			out["reference"] = map[string]any{"label": ref.Label, "source": ref.Source, "values": refv, "warn": warn, "bad": bad}
			out["heatmap"] = q.heatmap(ts, m, r, ref, TKey{groupID, highlight, model}, now)
		}
	}
	return out, nil
}

// heatmap 最近 7 天 × 24 小时，每格 = 那个小时和参照比的结论
func (q *QueryService) heatmap(ts *TrafficSet, m Metric, r TrafficRule, ref *Reference, k TKey, now time.Time) map[string]any {
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	var days []map[string]any
	hot := make([]int, 24)
	for d := 6; d >= 0; d-- {
		day := today.AddDate(0, 0, -d)
		cells := make([]string, 24)
		for h := 0; h < 24; h++ {
			t := day.Add(time.Duration(h) * time.Hour)
			if t.After(now) {
				cells[h] = "na"
				continue
			}
			a, ok := ts.Hourly[k][t.Unix()]
			if !ok || !sampleOK(r, a) {
				cells[h] = "few"
				continue
			}
			v, ok1 := m.Value(a)
			rv, ok2 := ref.Value(t.Unix(), false)
			if !ok1 || !ok2 {
				cells[h] = "few"
				continue
			}
			lv := r.level(m, v, rv)
			cells[h] = string(lv)
			if lv.Problem() {
				hot[h]++
			}
		}
		days = append(days, map[string]any{"day": day, "cells": cells})
	}
	var ranges [][2]int
	for h := 0; h < 24; h++ {
		if hot[h] >= 3 {
			if n := len(ranges); n > 0 && ranges[n-1][1] == h-1 {
				ranges[n-1][1] = h
			} else {
				ranges = append(ranges, [2]int{h, h})
			}
		}
	}
	return map[string]any{"days": days, "hot_ranges": ranges}
}

func (q *QueryService) compareGraded(ctx context.Context, b *Board, g *CatGroup, model string, it Item, highlight int64) (map[string]any, error) {
	runs, err := q.Runs.RecentRunsForJudge(ctx, 200)
	if err != nil {
		return nil, err
	}
	env := NewJudgeEnv(time.Now(), b.Snap.Catalog, b.Snap.Traffic, q.Registry, runs)
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var labels []time.Time
	for d := 6; d >= 0; d-- {
		labels = append(labels, today.AddDate(0, 0, -d))
	}
	var lines, rows []map[string]any
	seen := map[string]bool{}
	for _, aid := range g.Accounts {
		a := b.Snap.Catalog.Accounts[aid]
		if seen[a.CredKey] {
			continue
		}
		seen[a.CredKey] = true
		rs := env.RunsOf(it.ID, a.CredKey, model)
		vals := make([]*float64, len(labels))
		pass, total, errs := 0, 0, 0
		for i, day := range labels {
			p, n := 0, 0
			for _, r := range rs {
				if r.QueuedAt.Before(day) || !r.QueuedAt.Before(day.AddDate(0, 0, 1)) {
					continue
				}
				switch r.Outcome {
				case OutcomePass:
					p++
					n++
				case OutcomeFail:
					n++
				case OutcomeError:
					errs++
				}
			}
			if n > 0 {
				v := float64(p) * 100 / float64(n)
				vals[i] = &v
			}
			pass, total = pass+p, total+n
		}
		if total == 0 && len(rs) == 0 {
			continue
		}
		lines = append(lines, map[string]any{"account_id": aid, "name": a.Name, "values": vals})
		s := Subject{GroupID: g.ID, AccountID: aid, Model: model, Platform: a.Platform, CredKey: a.CredKey}
		row := map[string]any{"account_id": aid, "name": a.Name, "verdict": GradedDetector{}.Judge(env, it, s), "pass_7d": pass,
			"total_7d": total, "errors_7d": errs}
		rows = append(rows, row)
	}
	return map[string]any{"item": it, "graded": true, "labels": labels, "lines": lines, "rows": rows, "highlight": highlight, "range": "7d"}, nil
}

// ---------- 检测记录 ----------

// RunsBoard 「按账号看最近几次」
func (q *QueryService) RunsBoard(ctx context.Context, f RunFilter, groupID int64, perAccount int) (map[string]any, error) {
	b, err := q.Boards.Load(ctx)
	if err != nil {
		return nil, err
	}
	it := b.Setup.item(f.ItemID)
	if it == nil {
		return nil, ErrNotFound
	}
	det := q.Registry.Active(it.Type)
	if det == nil {
		return nil, fmt.Errorf("%w：「%s」不发请求，没有检测记录", ErrInvalid, it.Name)
	}
	cat := b.Snap.Catalog
	if groupID > 0 {
		g := cat.Groups[groupID]
		if g == nil {
			return nil, ErrNotFound
		}
		for _, id := range g.Accounts {
			if a := cat.Accounts[id]; a != nil && (f.AccountID == 0 || f.AccountID == id) {
				f.CredKeys = append(f.CredKeys, a.CredKey)
			}
		}
		if len(f.CredKeys) == 0 {
			f.CredKeys = []string{"-"}
		}
	} else if f.AccountID > 0 {
		if a := cat.Accounts[f.AccountID]; a != nil {
			f.CredKeys = []string{a.CredKey}
		}
	}
	if perAccount <= 0 || perAccount > 30 {
		perAccount = 10
	}
	f.Since = time.Now().Add(-7 * 24 * time.Hour)
	counts, err := q.Runs.CountOutcomes(ctx, f)
	if err != nil {
		return nil, err
	}
	f.Limit = 500
	list, err := q.Runs.ListRuns(ctx, f)
	if err != nil {
		return nil, err
	}
	type key struct {
		cred, model string
	}
	order := []key{}
	by := map[key][]map[string]any{}
	totals := map[key]int{}
	repAcc := map[key]int64{}
	for _, r := range list {
		k := key{r.CredKey, r.Model}
		if _, ok := by[k]; !ok {
			order = append(order, k)
			repAcc[k] = r.AccountID
		}
		totals[k]++
		if len(by[k]) < perAccount {
			by[k] = append(by[k], map[string]any{"id": r.ID, "t": r.QueuedAt, "outcome": det.Classify(*it, r), "summary": r.Summary, "detail": r.Detail})
		}
	}
	vs, err := q.Verdicts.LoadVerdicts(ctx)
	if err != nil {
		return nil, err
	}
	verdictOf := func(accountID int64, model string) *StoredVerdict {
		for i := range vs {
			if vs[i].AccountID == accountID && vs[i].Model == model && vs[i].ItemID == it.ID {
				return &vs[i]
			}
		}
		return nil
	}
	var rows []map[string]any
	for _, k := range order {
		aid := repAcc[k]
		a := cat.Accounts[aid]
		name, groups := fmt.Sprintf("#%d", aid), []string{}
		if a != nil {
			name = a.Name
			for _, alias := range cat.Aliases(aid) {
				if x := cat.Accounts[alias]; x != nil {
					groups = append(groups, cat.GroupNames(x.Groups)...)
				}
			}
		}
		row := map[string]any{"account_id": aid, "name": name, "groups": groups, "model": k.model, "runs": by[k], "total": totals[k]}
		if v := verdictOf(aid, k.model); v != nil {
			row["verdict"] = map[string]any{"level": v.Level, "short": v.Short}
		}
		rows = append(rows, row)
	}
	return map[string]any{"item": it, "spec": det.Spec(), "counts": counts, "rows": rows}, nil
}

// RunsList 「按时间看明细」
func (q *QueryService) RunsList(ctx context.Context, f RunFilter, groupID int64) ([]map[string]any, error) {
	b, err := q.Boards.Load(ctx)
	if err != nil {
		return nil, err
	}
	cat := b.Snap.Catalog
	if groupID > 0 {
		if g := cat.Groups[groupID]; g != nil {
			for _, id := range g.Accounts {
				if a := cat.Accounts[id]; a != nil && (f.AccountID == 0 || f.AccountID == id) {
					f.CredKeys = append(f.CredKeys, a.CredKey)
				}
			}
		}
		if len(f.CredKeys) == 0 {
			f.CredKeys = []string{"-"}
		}
	} else if f.AccountID > 0 {
		if a := cat.Accounts[f.AccountID]; a != nil {
			f.CredKeys = []string{a.CredKey}
		}
	}
	list, err := q.Runs.ListRuns(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(list))
	for _, r := range list {
		it := b.Setup.item(r.ItemID)
		outcome := r.Outcome
		if it != nil {
			if det := q.Registry.Active(it.Type); det != nil {
				outcome = det.Classify(*it, r)
			}
		}
		name := ""
		if a := cat.Accounts[r.AccountID]; a != nil {
			name = a.Name
		}
		out = append(out, map[string]any{"id": r.ID, "t": r.QueuedAt, "item_id": r.ItemID, "plan_id": r.PlanID, "account_id": r.AccountID,
			"account": name, "model": r.Model, "outcome": outcome, "summary": r.Summary, "error": r.Error, "duration_ms": r.DurationMs,
			"trigger": r.Trigger, "input_tokens": r.InputTokens, "output_tokens": r.OutputTokens})
	}
	return out, nil
}

// RunDetail 一次执行的完整内容
func (q *QueryService) RunDetail(ctx context.Context, id int64) (map[string]any, error) {
	r, err := q.Runs.GetRun(ctx, id)
	if err != nil {
		return nil, err
	}
	b, err := q.Boards.Load(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"run": r}
	if it := b.Setup.item(r.ItemID); it != nil {
		out["item"] = it
		out["spec"] = q.Registry.Spec(it.Type)
		if det := q.Registry.Active(it.Type); det != nil {
			out["outcome"] = det.Classify(*it, *r)
		}
	}
	if a := b.Snap.Catalog.Accounts[r.AccountID]; a != nil {
		out["account"] = map[string]any{"id": a.ID, "name": a.Name, "base_url": a.BaseURL, "groups": b.Snap.Catalog.GroupNames(a.Groups)}
	}
	if r.PlanID != nil {
		for _, p := range b.Setup.Plans {
			if p.ID == *r.PlanID {
				out["plan"] = p
			}
		}
	}
	return out, nil
}

// ---------- 待处理 ----------

func (q *QueryService) Events(ctx context.Context, state string) (map[string]any, error) {
	b, err := q.Boards.Load(ctx)
	if err != nil {
		return nil, err
	}
	states := []string{state}
	if state == "" {
		states = []string{EventTodo}
	}
	list, err := q.EventDB.ListEvents(ctx, states, 200)
	if err != nil {
		return nil, err
	}
	cat := b.Snap.Catalog
	counts := map[string]int{}
	for _, e := range b.Open {
		counts[e.State]++
	}
	sugs := map[int64]Suggestion{}
	var out []map[string]any
	for _, e := range list {
		row := map[string]any{"event": e}
		if a := cat.Accounts[e.AccountID]; a != nil {
			row["account"] = a.Name
		}
		if g := cat.Groups[e.GroupID]; g != nil {
			row["group"] = g.Name
			row["share"] = round1(b.Shares(e.GroupID)[e.AccountID])
		}
		if d := b.Setup.dim(e.DimensionID); d != nil {
			row["dimension"] = d
		}
		if it := b.Setup.item(e.ItemID); it != nil {
			row["item"] = it.Name
		}
		if e.Kind == "dim" {
			if _, ok := sugs[e.GroupID]; !ok {
				sugs[e.GroupID] = b.Simulate(e.GroupID)
			}
			for _, r := range sugs[e.GroupID].Rows {
				if r.AccountID == e.AccountID {
					row["suggest"] = r
				}
			}
			row["impact"] = impactText(b, e)
			if adj, ok := b.Adjust[[2]int64{e.GroupID, e.AccountID}]; ok {
				row["adjustment"] = adj
			}
		}
		out = append(out, row)
	}
	counts[EventResolved] = 0
	return map[string]any{"events": out, "counts": counts}, nil
}

func impactText(b *Board, e Event) string {
	d := b.Setup.dim(e.DimensionID)
	if d == nil {
		return ""
	}
	cfg := b.Setup.Config
	switch {
	case e.State == EventFalse:
		return "已标为误报，按正常处理"
	case d.Role == RoleGate && e.Level == LevelBad:
		g := d.Gate
		if e.State == EventConfirmed {
			if g.After == "stop" {
				return "已确认，建议停止分配"
			}
			return fmt.Sprintf("已确认，建议降到 %g%%", g.AfterPct)
		}
		switch g.Before {
		case "stop":
			return "待确认，建议停止分配"
		case "none":
			return "待确认，只提醒"
		}
		return fmt.Sprintf("待确认，建议先降到 %g%%", g.BeforePct)
	case e.Level == LevelBad:
		return fmt.Sprintf("建议份额 ×%g", cfg.BadFactor)
	}
	return fmt.Sprintf("建议份额 ×%g", cfg.WarnFactor)
}

// ---------- 调度建议 ----------

func (q *QueryService) Suggestions(ctx context.Context) ([]map[string]any, error) {
	b, err := q.Boards.Load(ctx)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, gid := range b.Snap.Catalog.GroupIDs {
		g := b.Snap.Catalog.Groups[gid]
		sug := b.Simulate(gid)
		names := map[int64]string{}
		adj := map[int64]Adjustment{}
		for _, id := range g.Accounts {
			names[id] = b.Snap.Catalog.Accounts[id].Name
			if a, ok := b.Adjust[[2]int64{gid, id}]; ok {
				adj[id] = a
			}
		}
		out = append(out, map[string]any{"group": g, "suggestion": sug, "names": names, "adjustments": adj})
	}
	return out, nil
}
