package dispatch

import (
	"encoding/json"
	"fmt"
	"time"
)

// TrafficRule 流量指标的判定规则：和参照比，超过倍数 / 百分点并持续一段时间
type TrafficRule struct {
	Bases      []string `json:"bases"`       // 参照回退链，按顺序用第一个可用的：peer | site | self | fixed
	MinPeers   int      `json:"min_peers"`   // 可比的上游少于几家时换下一个参照
	MinSamples int      `json:"min_samples"` // 近 1 小时请求少于几次不下结论
	MinEvents  int      `json:"min_events"`  // 计数类指标（报错率、限流率）：出问题的请求少于几次不算（避免偶发 1 次就报警）
	Mode       string   `json:"mode"`        // ratio（是参照的几倍）| pct（只有参照的百分之几）| diff（差几个百分点）
	Warn       float64  `json:"warn"`
	Bad        float64  `json:"bad"`
	Floor      *float64 `json:"floor,omitempty"` // 越低越好的指标：低于这个值一律算正常
	Duration   int      `json:"duration_minutes"`
	Fixed      *float64 `json:"fixed,omitempty"`
}

var referenceNames = map[string]string{
	"peer":  "同组同模型的其他账号",
	"site":  "全站同平台同模型的其他账号",
	"self":  "它自己过去 7 天同一时段",
	"fixed": "固定值",
}

func (r *TrafficRule) normalize(def TrafficRule) error {
	if len(r.Bases) == 0 {
		r.Bases = []string{"peer", "site", "self"}
	}
	seen := map[string]bool{}
	bases := r.Bases[:0]
	for _, b := range r.Bases {
		if _, ok := referenceNames[b]; ok && !seen[b] {
			seen[b] = true
			bases = append(bases, b)
		}
	}
	r.Bases = bases
	if seen["fixed"] && r.Fixed == nil {
		return fmt.Errorf("%w：选了「固定值」参照就要填固定值", ErrInvalid)
	}
	if r.MinPeers < 1 {
		r.MinPeers = 2
	}
	if r.MinSamples < 1 {
		r.MinSamples = 50
	}
	if r.MinEvents < 1 {
		r.MinEvents = 3
	}
	if r.Mode == "" {
		r.Mode = def.Mode
	}
	if r.Mode != "ratio" && r.Mode != "pct" && r.Mode != "diff" {
		return fmt.Errorf("%w：比较方式只能是 ratio / pct / diff", ErrInvalid)
	}
	if r.Warn == 0 && r.Bad == 0 {
		r.Warn, r.Bad = def.Warn, def.Bad
	}
	if r.Duration < 0 {
		r.Duration = 0
	}
	// 阈值方向：pct 越小越严重，其余越大越严重
	if r.Mode == "pct" && r.Bad > r.Warn || r.Mode != "pct" && r.Bad < r.Warn {
		return fmt.Errorf("%w：「异常」的阈值要比「可疑」更严", ErrInvalid)
	}
	return nil
}

// level 当前值相对参照的等级
func (r TrafficRule) level(m Metric, v, ref float64) Level {
	if m.Better == "low" && r.Floor != nil && v < *r.Floor {
		return LevelOK
	}
	switch r.Mode {
	case "ratio":
		g := 1.0
		if ref > 0 {
			g = v / ref
		}
		if m.Better == "high" { // 越高越好的指标用倍数：参照是它的几倍
			if v > 0 {
				g = ref / v
			} else {
				g = r.Bad
			}
		}
		return pick(g >= r.Bad, g >= r.Warn)
	case "pct":
		g := 100.0
		if ref > 0 {
			g = v / ref * 100
		}
		if m.Better == "low" && v > 0 {
			g = ref / v * 100
		}
		return pick(g <= r.Bad, g <= r.Warn)
	default:
		d := v - ref
		if m.Better == "high" {
			d = ref - v
		}
		return pick(d >= r.Bad, d >= r.Warn)
	}
}

// threshold 参照对应的「可疑 / 异常」线（画图用）
func (r TrafficRule) threshold(m Metric, ref float64, bad bool) float64 {
	t := r.Warn
	if bad {
		t = r.Bad
	}
	switch r.Mode {
	case "ratio":
		if m.Better == "high" {
			return ref / t
		}
		return ref * t
	case "pct":
		if m.Better == "low" {
			return ref * 100 / t
		}
		return ref * t / 100
	}
	if m.Better == "low" {
		return ref + t
	}
	return ref - t
}

func (r TrafficRule) gapText(m Metric, v, ref float64, short bool) string {
	switch r.Mode {
	case "ratio":
		g := v / ref
		if m.Better == "high" {
			g = ref / v
		}
		if short {
			return fmt.Sprintf("%.1f 倍", g)
		}
		if m.Better == "high" {
			return fmt.Sprintf("参照是它的 %.1f 倍", g)
		}
		return fmt.Sprintf("是参照的 %.1f 倍", g)
	case "pct":
		g := v / ref * 100
		if short {
			return fmt.Sprintf("参照的 %.0f%%", g)
		}
		return fmt.Sprintf("是参照的 %.0f%%", g)
	}
	d := v - ref
	word := "高"
	if d < 0 {
		word, d = "低", -d
	}
	if short {
		return fmt.Sprintf("%s %.1f 个点", map[string]string{"高": "+", "低": "−"}[word], d)
	}
	return fmt.Sprintf("比参照%s %.1f 个百分点", word, d)
}

func pick(bad, warn bool) Level {
	switch {
	case bad:
		return LevelBad
	case warn:
		return LevelWarn
	}
	return LevelOK
}

// Reference 参照
type Reference struct {
	Source string  `json:"source"`
	Label  string  `json:"label"`
	Peers  []int64 `json:"peers,omitempty"` // 参与比较的账号
	value  func(bucket int64, now bool) (float64, bool)
}

// Value 某个时间点（小时桶或「现在」）的参照值
func (r *Reference) Value(bucket int64, now bool) (float64, bool) { return r.value(bucket, now) }

// TrafficParams 流量统计检测项的参数
type TrafficParams struct {
	Metric string `json:"metric"`
}

// TrafficDetector 流量统计：从真实用户请求里统计，不额外发请求
type TrafficDetector struct{}

var _ Detector = TrafficDetector{}

func (TrafficDetector) Spec() TypeSpec {
	return TypeSpec{
		Key: "traffic", Name: "流量统计", Judges: true, Comparable: true,
		Description: "从真实用户请求里统计，不额外发请求、不花钱，每 5 分钟汇总一次。",
		Supports:    "所有平台、所有模型（有流量才有数据）",
		JudgeDesc:   "和参照比（同组 / 全站 / 自己的历史 / 固定值），超过倍数或百分点，并持续一段时间",
	}
}

func (TrafficDetector) NormalizeItem(params, rule json.RawMessage) (json.RawMessage, json.RawMessage, error) {
	var p TrafficParams
	if err := decodeInto(params, &p); err != nil {
		return nil, nil, err
	}
	m, ok := metricByKey(p.Metric)
	if !ok {
		return nil, nil, fmt.Errorf("%w：不认识的指标 %q", ErrInvalid, p.Metric)
	}
	r := m.Rule
	r.Bases = nil
	if len(rule) > 0 && string(rule) != "{}" && string(rule) != "null" {
		r = TrafficRule{}
		if err := decodeInto(rule, &r); err != nil {
			return nil, nil, err
		}
	}
	if err := r.normalize(m.Rule); err != nil {
		return nil, nil, err
	}
	return mustJSON(p), mustJSON(r), nil
}

func trafficItem(item Item) (Metric, TrafficRule, bool) {
	var p TrafficParams
	_ = json.Unmarshal(item.Params, &p)
	m, ok := metricByKey(p.Metric)
	if !ok {
		return Metric{}, TrafficRule{}, false
	}
	r := m.Rule
	_ = json.Unmarshal(item.Rule, &r)
	_ = r.normalize(m.Rule)
	return m, r, true
}

// sampleOK 样本量够不够下结论
func sampleOK(r TrafficRule, a Agg) bool { return a.Total() >= int64(r.MinSamples) }

// ReferenceFor 按回退链找参照
func ReferenceFor(env *JudgeEnv, item Item, s Subject) *Reference {
	m, r, ok := trafficItem(item)
	if !ok {
		return nil
	}
	tr, cat := env.Traffic, env.Catalog
	at := func(k TKey, bucket int64, now bool) (Agg, bool) {
		if now {
			a, ok := tr.Now[k]
			return a, ok
		}
		a, ok := tr.Hourly[k][bucket]
		return a, ok
	}
	// 同凭证的别名账号不算同类
	alias := map[int64]bool{}
	for _, id := range cat.Aliases(s.AccountID) {
		alias[id] = true
	}
	alias[s.AccountID] = true
	peerKeys := func(sameGroup bool) []TKey {
		var out []TKey
		best := map[int64]TKey{} // 全站：一个账号只取请求最多的那个分组
		for _, k := range tr.Keys() {
			if k.Model != s.Model || alias[k.Account] {
				continue
			}
			a := cat.Accounts[k.Account]
			if a == nil {
				continue
			}
			if sameGroup {
				if k.Group == s.GroupID {
					out = append(out, k)
				}
				continue
			}
			if a.Platform != s.Platform {
				continue
			}
			if old, ok := best[k.Account]; !ok || tr.Now[k].Total() > tr.Now[old].Total() {
				best[k.Account] = k
			}
		}
		for _, k := range best {
			out = append(out, k)
		}
		return out
	}
	peerRef := func(src string, keys []TKey) *Reference {
		// 「现在」能比的上游够不够
		var ids []int64
		for _, k := range keys {
			if a, ok := tr.Now[k]; ok && sampleOK(r, a) {
				if _, vok := m.Value(a); vok {
					ids = append(ids, k.Account)
				}
			}
		}
		if len(ids) < r.MinPeers {
			return nil
		}
		ref := &Reference{Source: src, Peers: ids}
		ref.Label = fmt.Sprintf("%s %d 家的中位数", map[string]string{"peer": "同组同模型", "site": "全站同模型"}[src], len(ids))
		ref.value = func(bucket int64, now bool) (float64, bool) {
			var vs []float64
			for _, k := range keys {
				if a, ok := at(k, bucket, now); ok && sampleOK(r, a) {
					if v, ok := m.Value(a); ok {
						vs = append(vs, v)
					}
				}
			}
			if len(vs) < r.MinPeers {
				return 0, false
			}
			return median(vs)
		}
		return ref
	}
	selfKey := TKey{s.GroupID, s.AccountID, s.Model}
	for _, b := range r.Bases {
		switch b {
		case "peer":
			if ref := peerRef("peer", peerKeys(true)); ref != nil {
				return ref
			}
		case "site":
			if ref := peerRef("site", peerKeys(false)); ref != nil {
				return ref
			}
		case "self":
			ref := &Reference{Source: "self", Label: "它自己过去 7 天同一时段的中位数"}
			ref.value = func(bucket int64, now bool) (float64, bool) {
				if now {
					bucket = tr.NowAt.Truncate(time.Hour).Unix()
				}
				var vs []float64
				for d := int64(1); d <= 7; d++ {
					if a, ok := tr.Hourly[selfKey][bucket-d*86400]; ok && sampleOK(r, a) {
						if v, ok := m.Value(a); ok {
							vs = append(vs, v)
						}
					}
				}
				if len(vs) < 3 {
					return 0, false
				}
				return median(vs)
			}
			if _, ok := ref.value(0, true); ok {
				return ref
			}
		case "fixed":
			if r.Fixed != nil {
				v := *r.Fixed
				return &Reference{Source: "fixed", Label: "固定值 " + m.Format(v), value: func(int64, bool) (float64, bool) { return v, true }}
			}
		}
	}
	return nil
}

func (TrafficDetector) Judge(env *JudgeEnv, item Item, s Subject) Verdict {
	m, r, ok := trafficItem(item)
	if !ok {
		return Verdict{Level: LevelNone, Short: "配置有误", Long: "检测项的指标配置有误"}
	}
	k := TKey{s.GroupID, s.AccountID, s.Model}
	cur := env.Traffic.Now[k]
	if !sampleOK(r, cur) {
		if cur.Total() == 0 {
			return Verdict{Level: LevelFew, Short: "无流量", Long: "近 1 小时没有请求"}
		}
		return Verdict{Level: LevelFew, Short: fmt.Sprintf("%d 次/时", cur.Total()),
			Long: fmt.Sprintf("近 1 小时只有 %d 次请求，少于 %d 次，不下结论", cur.Total(), r.MinSamples)}
	}
	v, vok := m.Value(cur)
	if !vok {
		return Verdict{Level: LevelFew, Short: "无数据", Long: "近 1 小时的请求里没有可用于计算这个指标的数据"}
	}
	ref := ReferenceFor(env, item, s)
	if ref == nil {
		return Verdict{Level: LevelFew, Value: fptr(v), Short: m.Format(v),
			Long: fmt.Sprintf("%s，暂无可比的参照", m.Format(v))}
	}
	rv, _ := ref.Value(0, true)
	lv := r.level(m, v, rv)
	fewEvents := ""
	if n, counted := m.ProblemCount(cur); counted && lv.Problem() && n < int64(r.MinEvents) {
		lv = LevelOK
		fewEvents = fmt.Sprintf("（只出现 %d 次，少于 %d 次不算）", n, r.MinEvents)
	}
	// 持续多久：「现在」这 60 分钟 + 往前连续有问题的小时桶。
	// 只取完全早于「现在」窗口的桶（和窗口重叠的那个桶不重复算），并且不超过账号接入至今的时长。
	dur := 0
	if lv.Problem() {
		dur = 60
		hb := env.Traffic.NowAt.Add(-time.Hour).Truncate(time.Hour).Add(-time.Hour)
		for i := 0; i < 7*24; i++ {
			b := hb.Add(-time.Duration(i) * time.Hour).Unix()
			a, ok := env.Traffic.Hourly[k][b]
			if !ok || !sampleOK(r, a) {
				break
			}
			hv, ok1 := m.Value(a)
			hr, ok2 := ref.Value(b, false)
			if !ok1 || !ok2 || !r.level(m, hv, hr).Problem() {
				break
			}
			dur += 60
		}
		if a := env.Catalog.Account(s.AccountID); a != nil && !a.CreatedAt.IsZero() {
			if age := int(env.Traffic.NowAt.Sub(a.CreatedAt).Minutes()); age >= 0 && age < dur {
				dur = age
			}
		}
		if dur < r.Duration {
			lv = LevelOK
		}
	}
	short := m.Format(v)
	long := fmt.Sprintf("%s，参照 %s（%s），%s%s", m.Format(v), m.Format(rv), ref.Label, r.gapText(m, v, rv, false), fewEvents)
	if lv.Problem() {
		short += " · " + r.gapText(m, v, rv, true)
		long += "，已持续 " + durationText(dur)
	}
	return Verdict{Level: lv, Value: fptr(v), Ref: fptr(rv), Short: short, Long: long, Evidence: map[string]any{
		"metric": m.Key, "reference": ref.Source, "reference_label": ref.Label, "peers": ref.Peers,
		"samples": cur.Total(), "duration_minutes": dur,
	}}
}

func durationText(min int) string {
	h := min / 60
	if h >= 24 {
		return fmt.Sprintf("%d 天 %d 小时", h/24, h%24)
	}
	if h >= 1 {
		return fmt.Sprintf("%d 小时", h)
	}
	return fmt.Sprintf("%d 分钟", min)
}
