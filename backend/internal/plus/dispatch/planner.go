package dispatch

import (
	"context"
	"math"
	"sort"
	"strings"
)

// CoverRow 计划覆盖到的一个 账号 × 模型（或者跳过的原因）
type CoverRow struct {
	AccountID int64             `json:"account_id"`
	Model     string            `json:"model"`
	OK        bool              `json:"ok"`
	Why       string            `json:"why,omitempty"`
	SameAs    int64             `json:"same_as,omitempty"` // 和哪个账号凭证相同（只测那一个，结果共用）
	Shares    map[int64]float64 `json:"shares"`            // 在所在各分组的近 24 小时流量占比
	Matched   []string          `json:"matched,omitempty"` // 命中了哪些账号范围规则
	credKey   string
}

// Planner 计算计划覆盖
type Planner struct {
	Registry *Registry
}

// accountModelTraffic 账号近 24 小时在所在分组里有没有这个模型的流量
func accountModelTraffic(tr *TrafficSet, acc *CatAccount, model string) bool {
	from := tr.NowAt.Add(-24 * timeHour).Truncate(timeHour)
	for _, g := range acc.Groups {
		k := TKey{g, acc.ID, model}
		if tr.Now[k].Total() > 0 || tr.Range(k, from, tr.NowAt.Add(timeHour)).Total() > 0 {
			return true
		}
	}
	return false
}

// Cover 计算一个计划覆盖哪些 账号 × 模型：先过排除和不能直连，再看计划选的账号范围规则命中任意一条
func (p Planner) Cover(ctx context.Context, plan Plan, item Item, cat *Catalog, tr *TrafficSet) []CoverRow {
	det := p.Registry.Active(item.Type)
	exc := idSet(plan.ExcludeIDs)
	shareCache := map[int64]map[int64]float64{}
	share := func(gid int64) map[int64]float64 {
		if s, ok := shareCache[gid]; ok {
			return s
		}
		s, _ := tr.GroupShares(gid)
		shareCache[gid] = s
		return s
	}
	var ids []int64
	for id, a := range cat.Accounts {
		if a.Platform == plan.Platform {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	tested := map[string]int64{} // cred|model → 代表账号
	var out []CoverRow
	for _, id := range ids {
		a := cat.Accounts[id]
		shares := map[int64]float64{}
		maxShare := 0.0
		for _, g := range a.Groups {
			v := share(g)[id]
			shares[g] = round1(v)
			maxShare = math.Max(maxShare, v)
		}
		for _, m := range plan.Models {
			row := CoverRow{AccountID: id, Model: m, Shares: shares, credKey: a.CredKey}
			switch {
			case !plan.Enabled:
				row.Why = "计划已停用"
			case exc[id]:
				row.Why = "已排除"
			case !a.Direct:
				row.Why = a.DirectWhy
			case det == nil:
				row.Why = "检测项类型不支持计划"
			default:
				if ok, why := det.Supports(ctx, plan.Platform, m); !ok {
					row.Why = why
					break
				}
				var whys []string
				t := ScopeTarget{Account: a, Model: m, MaxShare: maxShare, Traffic: tr}
				for _, sc := range plan.Scopes {
					r := scopeRule(sc.Type)
					if r == nil {
						continue
					}
					if ok, why := r.Match(t, sc); ok {
						row.Matched = append(row.Matched, r.Spec().Name)
					} else if why != "" {
						whys = append(whys, why)
					}
				}
				if len(row.Matched) == 0 {
					row.Why = strings.Join(whys, "；")
				} else if rep, dup := tested[a.CredKey+"|"+m]; dup {
					row.OK, row.SameAs = true, rep
				} else {
					row.OK = true
					tested[a.CredKey+"|"+m] = id
				}
			}
			out = append(out, row)
		}
	}
	return out
}

// Targets 计划这一轮实际要发请求的 账号 × 模型（每个凭证只测一次）
func (p Planner) Targets(ctx context.Context, plan Plan, item Item, cat *Catalog, tr *TrafficSet) []CoverRow {
	var out []CoverRow
	for _, r := range p.Cover(ctx, plan, item, cat, tr) {
		if r.OK && r.SameAs == 0 {
			out = append(out, r)
		}
	}
	return out
}

func idSet(ids []int64) map[int64]bool {
	m := make(map[int64]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}
