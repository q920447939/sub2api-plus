package dispatch

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// SuggestReason 某个维度带来的调整理由
type SuggestReason struct {
	DimensionID int64  `json:"dimension_id"`
	Dimension   string `json:"dimension"`
	Role        Role   `json:"role"`
	Level       Level  `json:"level"`
	Evidence    string `json:"evidence"`
	Action      string `json:"action"`
}

// SuggestRow 一个账号的建议
type SuggestRow struct {
	AccountID int64           `json:"account_id"`
	Before    float64         `json:"before"`
	After     float64         `json:"after"`
	Kind      string          `json:"kind"` // keep（正常，接收流量）| gate | score | few（数据不足，不动）| idle（没流量）
	Reasons   []SuggestReason `json:"reasons"`
	// 采纳时生成的调整：pause / factor（after/before）/ normal（不需要调整）
	Action string  `json:"action"`
	Factor float64 `json:"factor"`
	// Settled 现在生效的调整已经和建议一样严或更严（例如已暂停），不用再调，也不计入要挪动的流量
	Settled bool `json:"settled"`
}

// Suggestion 一个分组的调度建议
type Suggestion struct {
	GroupID    int64        `json:"group_id"`
	Basis      string       `json:"basis"` // 起点用的是哪段时间的实际份额
	Requests   int64        `json:"requests"`
	Rows       []SuggestRow `json:"rows"`
	Moved      float64      `json:"moved"`
	Wanted     float64      `json:"wanted"`
	Capped     bool         `json:"capped"`
	NoReceiver bool         `json:"no_receiver"`
	Summary    string       `json:"summary"`
}

// SimInput 模拟一个分组需要的数据
type SimInput struct {
	Config   Config
	Dims     []Dimension
	Shares   map[int64]float64 // 账号 → 起点份额（%）
	Basis    string
	Requests int64
	Accounts []int64
	DimRes   map[int64]map[int64]DimResult // 账号 → 维度 → 结论
	Confirm  map[string]string             // "账号:维度" → confirmed / false
	Rates    map[int64]float64
	Names    map[int64]string
	Current  map[int64]float64 // 账号 → 现在生效的调整系数（暂停 = 0；没有调整的不在里面）
}

// Simulate 「不积极」的调度建议：以当前实际份额为起点，只从可疑 / 异常的账号挪走，正常的按现有份额接收。
// 纯函数，方便单测。
func Simulate(groupID int64, in SimInput) Suggestion {
	cfg := in.Config
	sug := Suggestion{GroupID: groupID, Basis: in.Basis, Requests: in.Requests}
	type work struct {
		SuggestRow
		f      float64
		fixed  *float64
		exempt bool
		raw    float64
	}
	rows := make([]*work, 0, len(in.Accounts))
	for _, id := range in.Accounts {
		w := &work{SuggestRow: SuggestRow{AccountID: id, Before: in.Shares[id], Kind: "keep", Reasons: []SuggestReason{}}, f: 1}
		w.After = w.Before
		allFew := true
		for _, d := range in.Dims {
			if !d.Enabled || d.Role == RoleDisplay {
				continue
			}
			res, ok := in.DimRes[id][d.ID]
			if !ok {
				continue
			}
			if res.Level == LevelOK || res.Level.Problem() {
				allFew = false
			}
			if !res.Level.Problem() {
				continue
			}
			state := in.Confirm[fmt.Sprintf("%d:%d", id, d.ID)]
			if state == EventFalse {
				continue // 已标误报：按正常处理
			}
			reason := SuggestReason{DimensionID: d.ID, Dimension: d.Name, Role: d.Role, Level: res.Level, Evidence: res.Long}
			if d.Role == RoleGate && res.Level == LevelBad {
				confirmed := state == EventConfirmed
				act, pct := d.Gate.Before, d.Gate.BeforePct
				if confirmed {
					act, pct = d.Gate.After, d.Gate.AfterPct
				}
				prefix := "还没确认 → "
				if confirmed {
					prefix = "已确认 → "
				}
				w.Kind = "gate"
				switch act {
				case "stop":
					zero := 0.0
					w.fixed, w.exempt = &zero, confirmed
					reason.Action = prefix + "停止分配"
				case "cut":
					v := pct
					if w.fixed == nil || *w.fixed > v {
						w.fixed = &v
					}
					reason.Action = fmt.Sprintf("%s降到 %g%%", prefix, pct)
				default:
					reason.Action = prefix + "只提醒，不动流量"
				}
			} else {
				f := cfg.WarnFactor
				if res.Level == LevelBad {
					f = cfg.BadFactor
				}
				w.f = math.Min(w.f, f)
				if w.Kind != "gate" {
					w.Kind = "score"
				}
				reason.Action = fmt.Sprintf("份额 ×%g", f)
			}
			w.Reasons = append(w.Reasons, reason)
		}
		if w.Kind == "keep" && allFew {
			w.Kind = "few"
		}
		if w.Kind == "keep" && w.Before <= 0 {
			w.Kind = "idle"
		}
		rows = append(rows, w)
	}

	var deg []*work
	for _, w := range rows {
		if w.Kind == "gate" || w.Kind == "score" {
			deg = append(deg, w)
			t := w.Before * w.f
			if w.fixed != nil {
				t = math.Min(t, *w.fixed)
			}
			if w.fixed == nil || *w.fixed != 0 {
				t = math.Max(t, math.Min(w.Before, cfg.MinShare)) // 至少保留一点流量，用来观察有没有恢复
			}
			w.raw = t
		}
	}
	red := 0.0
	for _, w := range deg {
		if !w.exempt {
			red += w.Before - w.raw
		}
	}
	k := 1.0
	if red > cfg.MaxMove {
		k = cfg.MaxMove / red
		sug.Capped = true
	}
	sug.Wanted = round1(red)
	freed := 0.0
	for _, w := range deg {
		if w.exempt {
			w.After = w.raw
		} else {
			w.After = w.Before - (w.Before-w.raw)*k
		}
		freed += w.Before - w.After
	}
	var recv []*work
	for _, w := range rows {
		if w.Kind == "keep" && w.Before > 0 {
			recv = append(recv, w)
		}
	}
	if len(recv) == 0 && freed > 0 {
		for _, w := range deg {
			w.After = w.Before
		}
		freed, sug.NoReceiver = 0, true
	}
	minRate := math.Inf(1)
	for _, w := range recv {
		if r := in.Rates[w.AccountID]; r < minRate {
			minRate = r
		}
	}
	weight := func(w *work) float64 {
		x := w.Before
		if cfg.Pref == "cost" {
			// 倍率越低分得越多；开平方避免一下子全压到最便宜的一家
			x *= math.Sqrt(math.Max(minRate, 0.01) / math.Max(in.Rates[w.AccountID], 0.01))
		}
		return x
	}
	W, pool := 0.0, freed
	for _, w := range recv {
		W += weight(w)
		pool += w.Before
	}
	for _, w := range recv {
		if W > 0 {
			w.After = pool * weight(w) / W
		}
	}
	for _, w := range rows {
		w.Before, w.After = round1(w.Before), round1(w.After)
		w.Action, w.Factor = "normal", 1
		// 没有流量的账号没什么可挪的，不给「暂停 / 降份额」建议（理由照样保留，页面上能看到它有问题）
		if (w.Kind == "gate" || w.Kind == "score") && w.Before > 0 {
			switch {
			case w.After <= 0:
				w.Action, w.Factor = "pause", 0
			case w.Before > 0 && w.After < w.Before:
				w.Action = "factor"
				w.Factor = math.Max(0.05, math.Min(0.95, math.Round(w.After/w.Before*100)/100))
			}
		}
		if cur, ok := in.Current[w.AccountID]; ok && w.Action != "normal" && cur <= w.Factor {
			w.Settled = true
		}
		if d := w.Before - w.After; d > 0 && !w.Settled {
			sug.Moved += d
		}
		sug.Rows = append(sug.Rows, w.SuggestRow)
	}
	sug.Moved = round1(sug.Moved)
	sort.SliceStable(sug.Rows, func(i, j int) bool { return sug.Rows[i].Before > sug.Rows[j].Before })

	var downs, settled []string
	for _, r := range sug.Rows {
		switch {
		case r.Settled:
			settled = append(settled, fmt.Sprintf("%s %s", nameOr(in.Names, r.AccountID), adjustLabel(in.Current[r.AccountID])))
		case r.After < r.Before:
			downs = append(downs, fmt.Sprintf("%s %.0f%% → %.0f%%", nameOr(in.Names, r.AccountID), r.Before, r.After))
		}
	}
	switch {
	case sug.NoReceiver:
		sug.Summary = "有账号出问题，但本组没有其他正常账号可以接，不调整"
	case sug.Moved > 0:
		sug.Summary = fmt.Sprintf("建议挪动 %.0f%% 流量：%s", sug.Moved, strings.Join(downs, "，"))
		if len(settled) > 0 {
			sug.Summary += "（" + strings.Join(settled, "，") + "，已调整到位）"
		}
	case len(settled) > 0:
		sug.Summary = "有问题的账号已调整到位，不用再调：" + strings.Join(settled, "，")
	default:
		sug.Summary = "各账号都正常，不调整"
	}
	return sug
}

func nameOr(names map[int64]string, id int64) string {
	if n := names[id]; n != "" {
		return n
	}
	return fmt.Sprintf("#%d", id)
}

// PredictShares 按调整预测份额：份额 × 系数（暂停为 0），再换算回 100%
func PredictShares(before map[int64]float64, factors map[int64]float64) map[int64]float64 {
	out := map[int64]float64{}
	sum := 0.0
	for id, s := range before {
		f, ok := factors[id]
		if !ok {
			f = 1
		}
		sum += s * f
	}
	total := 0.0
	for _, s := range before {
		total += s
	}
	for id, s := range before {
		f, ok := factors[id]
		if !ok {
			f = 1
		}
		if sum > 0 {
			out[id] = round1(s * f / sum * total)
		}
	}
	return out
}

// adjustLabel 由系数还原调整的说法（暂停 = 0）
func adjustLabel(f float64) string {
	if f <= 0 {
		return adjustText("pause", 0)
	}
	return "已 " + adjustText("factor", f)
}
