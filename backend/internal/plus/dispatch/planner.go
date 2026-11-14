package dispatch

import (
	"context"
	"sort"
	"time"
)

// CoverRow 计划落到的一个 账号 × 模型（或者测不了的原因）
type CoverRow struct {
	AccountID int64  `json:"account_id"`
	Model     string `json:"model"`
	OK        bool   `json:"ok"`
	Why       string `json:"why,omitempty"`
	SameAs    int64  `json:"same_as,omitempty"` // 和哪个账号凭证相同（只测那一个，结果共用）
	Idle      bool   `json:"idle,omitempty"`    // 近 24 小时没有这个模型的流量（备用号，按「备用号间隔」测）
	credKey   string
}

// Planner 计划覆盖：
//   - 只测调度开着的账号（调度已关闭的不测；被调度台暂停 / 调低权重的照常测）
//   - 只测账号自己提供的模型
//   - 计划指定了账号就只测这些账号
//   - 同一个凭证（地址 + Key）挂在多个账号上只测一次，结果共用
type Planner struct {
	Registry *Registry
}

// Cover 计划覆盖哪些 账号 × 模型
func (p Planner) Cover(ctx context.Context, plan Plan, item Item, cat *Catalog, tr *TrafficSet) []CoverRow {
	det, _ := p.Registry.Get(item.Type)
	only := map[int64]bool{}
	for _, id := range plan.AccountIDs {
		only[id] = true
	}
	var ids []int64
	for id, a := range cat.Accounts {
		if a.Platform == plan.Platform && (len(only) == 0 || only[id]) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	busy := busyPairs(tr)
	tested := map[string]int64{}
	var out []CoverRow
	for _, id := range ids {
		a := cat.Accounts[id]
		for _, m := range plan.Models {
			row := CoverRow{AccountID: id, Model: m, credKey: a.CredKey}
			switch {
			case !a.Serving():
				if len(only) == 0 {
					continue // 没指定账号时，调度已关闭的不列出来
				}
				row.Why = "调度已关闭"
			case !a.Supports(m):
				if len(only) == 0 {
					continue
				}
				row.Why = "账号没有这个模型"
			case !a.Direct:
				row.Why = a.DirectWhy
			case det == nil:
				row.Why = "检测项类型已不支持"
			default:
				if ok, why := det.Supports(ctx, plan.Platform, m); !ok {
					row.Why = why
					break
				}
				row.OK = true
				row.Idle = !busy[pairKey{id, m}]
				if rep, dup := tested[a.CredKey+"|"+m]; dup {
					row.SameAs = rep
				} else {
					tested[a.CredKey+"|"+m] = id
				}
			}
			out = append(out, row)
		}
	}
	return out
}

type pairKey struct {
	Account int64
	Model   string
}

// busyPairs 近 24 小时有流量的 账号 × 模型
func busyPairs(tr *TrafficSet) map[pairKey]bool {
	out := map[pairKey]bool{}
	if tr == nil {
		return out
	}
	tr.Each(TFilter{From: tr.NowBucket() - 23*3600}, func(r *TRow) {
		if r.Total() > 0 {
			out[pairKey{r.Account, r.Model}] = true
		}
	})
	return out
}

// Due 这一轮实际要发请求的：每个凭证只测一次；没有流量的备用号按 idle 间隔（不低于计划频率）测
func (p Planner) Due(ctx context.Context, plan Plan, item Item, cat *Catalog, tr *TrafficSet, last map[string]time.Time,
	idle time.Duration, now time.Time, manual bool) []CoverRow {
	var out []CoverRow
	for _, r := range p.Cover(ctx, plan, item, cat, tr) {
		if !r.OK || r.SameAs != 0 {
			continue
		}
		if r.Idle && !manual {
			gap := idle
			if every := time.Duration(plan.IntervalMinutes) * time.Minute; every > gap {
				gap = every
			}
			if at, ok := last[runKeyOf(item.ID, r.credKey, r.Model)]; ok && now.Sub(at) < gap-time.Minute {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}
