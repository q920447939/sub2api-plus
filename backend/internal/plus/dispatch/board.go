package dispatch

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Board 页面读取用的一次性数据：配置 + 目录 + 流量 + 当前判定 + 待处理 + 调整
type Board struct {
	Setup    Setup
	Snap     *Snapshot
	Verdicts map[[2]int64][]StoredVerdict // (分组, 账号) → 判定
	Open     []Event
	Adjust   map[[2]int64]Adjustment
	Registry *Registry
	shares   map[int64]map[int64]float64
}

// BoardLoader 组装 Board
type BoardLoader struct {
	Config   ConfigStore
	Setup    SetupStore
	Verdicts VerdictStore
	Events   EventStore
	Adjust   AdjustStore
	Snap     *Snapshotter
	Registry *Registry
}

func (l BoardLoader) Load(ctx context.Context) (*Board, error) {
	st, err := LoadSetup(ctx, l.Config, l.Setup)
	if err != nil {
		return nil, err
	}
	snap, err := l.Snap.Get(ctx, false)
	if err != nil {
		return nil, err
	}
	vs, err := l.Verdicts.LoadVerdicts(ctx)
	if err != nil {
		return nil, err
	}
	open, err := l.Events.OpenEvents(ctx)
	if err != nil {
		return nil, err
	}
	adj, err := l.Adjust.ListAdjustments(ctx)
	if err != nil {
		return nil, err
	}
	b := &Board{Setup: st, Snap: snap, Verdicts: map[[2]int64][]StoredVerdict{}, Open: open, Adjust: map[[2]int64]Adjustment{},
		Registry: l.Registry, shares: map[int64]map[int64]float64{}}
	for _, v := range vs {
		k := [2]int64{v.GroupID, v.AccountID}
		b.Verdicts[k] = append(b.Verdicts[k], v)
	}
	for _, a := range adj {
		b.Adjust[[2]int64{a.GroupID, a.AccountID}] = a
	}
	return b, nil
}

// Shares 分组里各账号近 24 小时流量占比
func (b *Board) Shares(groupID int64) map[int64]float64 {
	if s, ok := b.shares[groupID]; ok {
		return s
	}
	s, _ := b.Snap.Traffic.GroupShares(groupID)
	b.shares[groupID] = s
	return s
}

// HourShares 分组里各账号最近 1 小时的流量占比；请求太少时回退到 24 小时
func (b *Board) HourShares(groupID int64) (map[int64]float64, string, int64) {
	per := map[int64]int64{}
	var total int64
	for k, a := range b.Snap.Traffic.Now {
		if k.Group == groupID {
			per[k.Account] += a.Total()
			total += a.Total()
		}
	}
	if total >= 50 {
		out := map[int64]float64{}
		for id, n := range per {
			out[id] = float64(n) * 100 / float64(total)
		}
		return out, "过去 1 小时", total
	}
	s, t := b.Snap.Traffic.GroupShares(groupID)
	return s, "过去 24 小时（最近 1 小时请求太少）", t
}

// eventState 维度待处理的处理状态（confirmed / false / todo / ""）
func (b *Board) eventState(groupID, accountID, dimID int64) string {
	key := fmt.Sprintf("dim:%d:%d:%d", groupID, accountID, dimID)
	for _, e := range b.Open {
		if e.DedupKey == key {
			return e.State
		}
	}
	return ""
}

// DimResults 一个 分组 × 账号 各维度的结论；model 为空时跨模型取最差
func (b *Board) DimResults(groupID, accountID int64, model string) map[int64]DimResult {
	vs := b.Verdicts[[2]int64{groupID, accountID}]
	if model != "" {
		var f []StoredVerdict
		for _, v := range vs {
			if v.Model == model {
				f = append(f, v)
			}
		}
		vs = f
	}
	out := map[int64]DimResult{}
	for _, d := range b.Setup.Dims {
		if !d.Enabled {
			continue
		}
		res, _ := aggregateDim(b.Registry, b.Setup, d, vs)
		if res.Level.Problem() && b.eventState(groupID, accountID, d.ID) == EventFalse {
			res.Muted, res.Level = true, LevelOK
		}
		out[d.ID] = res
	}
	return out
}

// Worst 账号在分组里最差的结论（不含仅展示维度）
func (b *Board) Worst(res map[int64]DimResult) Level {
	w := LevelFew
	for _, d := range b.Setup.Dims {
		if !d.Enabled || d.Role == RoleDisplay {
			continue
		}
		if r, ok := res[d.ID]; ok && r.Level.Rank() > w.Rank() && r.Level != LevelInfo {
			w = r.Level
		}
	}
	return w
}

// OneLine 一句话结论
func (b *Board) OneLine(res map[int64]DimResult) string {
	type p struct {
		d Dimension
		r DimResult
	}
	var ps []p
	for _, d := range b.Setup.Dims {
		if r, ok := res[d.ID]; ok && d.Enabled && d.Role != RoleDisplay && r.Level.Problem() {
			ps = append(ps, p{d, r})
		}
	}
	if len(ps) == 0 {
		if b.Worst(res) == LevelFew {
			return "暂无结论"
		}
		return "各项正常"
	}
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].r.Level.Rank() > ps[j].r.Level.Rank() })
	first := ps[0]
	name := ""
	if it := b.Setup.item(first.r.ItemID); it != nil {
		name = it.Name
	}
	// 一句结论：维度 + 等级 + 检测项的简短值；完整依据在账号诊断详情里
	s := fmt.Sprintf("%s%s · %s %s", first.d.Name, levelName(first.r.Level), name, first.r.Short)
	if len(ps) > 1 {
		s += fmt.Sprintf("，另有 %d 项", len(ps)-1)
	}
	return s
}

// Models 分组 × 账号判定过的模型
func (b *Board) Models(groupID, accountID int64) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range b.Verdicts[[2]int64{groupID, accountID}] {
		if !seen[v.Model] {
			seen[v.Model] = true
			out = append(out, v.Model)
		}
	}
	sort.Strings(out)
	return out
}

// Simulate 一个分组的调度建议
func (b *Board) Simulate(groupID int64) Suggestion {
	g := b.Snap.Catalog.Groups[groupID]
	if g == nil {
		return Suggestion{GroupID: groupID}
	}
	shares, basis, total := b.HourShares(groupID)
	in := SimInput{Config: b.Setup.Config, Dims: b.Setup.Dims, Shares: shares, Basis: basis, Requests: total,
		DimRes: map[int64]map[int64]DimResult{}, Confirm: map[string]string{}, Rates: map[int64]float64{}, Names: map[int64]string{},
		Current: map[int64]float64{}}
	for _, id := range g.Accounts {
		a := b.Snap.Catalog.Accounts[id]
		if a == nil {
			continue
		}
		in.Accounts = append(in.Accounts, id)
		in.Rates[id], in.Names[id] = a.Rate, a.Name
		if adj, ok := b.Adjust[[2]int64{groupID, id}]; ok {
			f := adj.Factor
			if adj.Action == "pause" {
				f = 0
			}
			in.Current[id] = f
		}
		in.DimRes[id] = b.DimResults(groupID, id, "")
		for _, d := range b.Setup.Dims {
			if st := b.eventState(groupID, id, d.ID); st != "" {
				in.Confirm[fmt.Sprintf("%d:%d", id, d.ID)] = st
			}
		}
	}
	return Simulate(groupID, in)
}

// CoverageGaps 门槛维度没有被判定的账号（只有仅展示的检测项，或没有计划覆盖）
func (b *Board) CoverageGaps() []map[string]any {
	var out []map[string]any
	for _, d := range b.Setup.Dims {
		if !d.Enabled || d.Role != RoleGate {
			continue
		}
		var names []string
		for _, gid := range b.Snap.Catalog.GroupIDs {
			shares := b.Shares(gid)
			for _, aid := range b.Snap.Catalog.Groups[gid].Accounts {
				if shares[aid] < 1 {
					continue
				}
				r := b.DimResults(gid, aid, "")[d.ID]
				if r.Level == LevelNone || r.Level == LevelInfo {
					names = append(names, fmt.Sprintf("%s（%s）", b.Snap.Catalog.Accounts[aid].Name, b.Snap.Catalog.Groups[gid].Name))
				}
			}
		}
		if len(names) > 0 {
			out = append(out, map[string]any{"dimension": d.Name, "accounts": names})
		}
	}
	return out
}

// adjustText 调整的大白话
func adjustText(action string, factor float64) string {
	switch action {
	case "pause":
		return "已暂停"
	case "factor":
		return fmt.Sprintf("×%g", factor)
	}
	return "正常"
}

func since(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d 小时", int(d.Hours()))
	}
	return fmt.Sprintf("%d 天", int(d.Hours()/24))
}

func joinNames(ns []string) string { return strings.Join(ns, "、") }

// NewDays 「新接入」看几天：取这个平台已启用计划里「新接入的账号」规则最长的天数；没有时按默认
func (b *Board) NewDays(platform string) int {
	days := 0
	for _, p := range b.Setup.Plans {
		if !p.Enabled || p.Platform != platform {
			continue
		}
		if v, ok := scopeParam(p.Scopes, "new", "days"); ok && int(v) > days {
			days = int(v)
		}
	}
	if days == 0 {
		return defaultNewDays
	}
	return days
}
