//go:build unit

package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestMatchers(t *testing.T) {
	cases := []struct {
		match, reply string
		answers      []string
		got          string
		pass         bool
	}{
		{"num", "先拿 12 个西瓜味，再拿 16 个……所以最少要取出 **29** 个。验证：28+1", []string{"29"}, "29", true},
		{"num", "答案是 \\boxed{29}，注意不是 30", []string{"29"}, "29", true},
		{"num", "最少 28 个", []string{"29"}, "28", false},
		{"num", "无法确定", []string{"29"}, "", false},
		{"num", "29.0", []string{"29"}, "29.0", true},
		{"exact", "  巴黎 ", []string{"巴黎"}, "巴黎", true},
		{"contains", "首都是巴黎。", []string{"伦敦", "巴黎"}, "巴黎", true},
		{"regex", "Final: 29 candies", []string{`\b29\b`}, "29", true},
	}
	for _, c := range cases {
		got, pass, err := TryGrade(GradedParams{Prompt: "q", Answers: c.answers, Match: c.match}, c.reply)
		if err != nil || got != c.got || pass != c.pass {
			t.Errorf("%s %q: got=%q pass=%v err=%v, want %q %v", c.match, c.reply, got, pass, err, c.got, c.pass)
		}
	}
	if _, _, err := TryGrade(GradedParams{Prompt: "q", Answers: []string{"("}, Match: "regex"}, "x"); err == nil {
		t.Error("invalid regex must be rejected")
	}
}

func TestTrafficRuleLevels(t *testing.T) {
	ttft, _ := metricByKey("ttft")
	r := ttft.Rule
	if r.level(ttft, 6.5, 4) != LevelWarn || r.level(ttft, 9, 4) != LevelBad || r.level(ttft, 5, 4) != LevelOK {
		t.Fatal("ratio levels")
	}
	if r.level(ttft, 2.5, 1) != LevelOK { // 低于 3 秒的一律算正常
		t.Fatal("floor")
	}
	tps, _ := metricByKey("tps")
	if tps.Rule.level(tps, 40, 100) != LevelBad || tps.Rule.level(tps, 65, 100) != LevelWarn || tps.Rule.level(tps, 90, 100) != LevelOK {
		t.Fatal("pct levels")
	}
	cache, _ := metricByKey("cache")
	if cache.Rule.level(cache, 50, 72) != LevelWarn || cache.Rule.level(cache, 40, 72) != LevelBad {
		t.Fatal("diff levels (higher is better)")
	}
	if got := r.threshold(ttft, 4, false); got != 6 {
		t.Fatalf("threshold %v", got)
	}
}

func TestTrafficNormalizeRejectsInvertedThresholds(t *testing.T) {
	_, _, err := TrafficDetector{}.NormalizeItem(json.RawMessage(`{"metric":"ttft"}`), json.RawMessage(`{"mode":"ratio","warn":2,"bad":1.5}`))
	if err == nil {
		t.Fatal("expected error")
	}
	_, rule, err := TrafficDetector{}.NormalizeItem(json.RawMessage(`{"metric":"err"}`), nil)
	if err != nil || !strings.Contains(string(rule), `"duration_minutes":60`) {
		t.Fatalf("defaults: %s %v", rule, err)
	}
}

// 参照回退：同组只有自己 → 全站同平台同模型
func TestReferenceFallsBackToSite(t *testing.T) {
	now := time.Now()
	cat := &Catalog{Accounts: map[int64]*CatAccount{}, Groups: map[int64]*CatGroup{}, byCred: map[string][]int64{}}
	for _, a := range []*CatAccount{{ID: 1, Platform: "anthropic", CredKey: "a", Groups: []int64{10}}, {ID: 2, Platform: "anthropic", CredKey: "b", Groups: []int64{20}},
		{ID: 3, Platform: "anthropic", CredKey: "c", Groups: []int64{20}}} {
		cat.Accounts[a.ID] = a
		cat.byCred[a.CredKey] = []int64{a.ID}
	}
	cat.Groups[10] = &CatGroup{ID: 10, Accounts: []int64{1}}
	cat.Groups[20] = &CatGroup{ID: 20, Accounts: []int64{2, 3}}
	ts := newTrafficSet(now)
	mk := func(p90 float64) Agg { return Agg{Requests: 100, TTFTN: 100, TTFTP90: p90} }
	ts.Now[TKey{10, 1, "m"}] = mk(9000)
	ts.Now[TKey{20, 2, "m"}] = mk(4000)
	ts.Now[TKey{20, 3, "m"}] = mk(4400)
	item := Item{ID: 1, Type: "traffic", Params: json.RawMessage(`{"metric":"ttft"}`), Rule: json.RawMessage(`{"bases":["peer","site"],"min_peers":2,"min_samples":50,"mode":"ratio","warn":1.5,"bad":2,"duration_minutes":0}`)}
	env := NewJudgeEnv(now, cat, ts, NewRegistry(TrafficDetector{}), nil)
	s := Subject{GroupID: 10, AccountID: 1, Model: "m", Platform: "anthropic", CredKey: "a"}
	ref := ReferenceFor(env, item, s)
	if ref == nil || ref.Source != "site" {
		t.Fatalf("expected site reference, got %+v", ref)
	}
	v := TrafficDetector{}.Judge(env, item, s)
	if v.Level != LevelBad || !strings.Contains(v.Long, "全站同模型 2 家") {
		t.Fatalf("verdict %+v", v)
	}
}

func TestSimulateNotAggressive(t *testing.T) {
	dims := []Dimension{
		{ID: 1, Name: "模型身份", Role: RoleGate, Enabled: true, Gate: Gate{Before: "cut", BeforePct: 3, After: "stop"}},
		{ID: 2, Name: "速度", Role: RoleScore, Enabled: true},
	}
	in := SimInput{Config: DefaultConfig(), Dims: dims, Accounts: []int64{1, 2, 3, 4},
		Shares: map[int64]float64{1: 31, 2: 29, 3: 26, 4: 14},
		DimRes: map[int64]map[int64]DimResult{
			1: {1: {Level: LevelOK}, 2: {Level: LevelOK}},
			2: {1: {Level: LevelOK}, 2: {Level: LevelOK}},
			3: {1: {Level: LevelOK}, 2: {Level: LevelBad}},
			4: {1: {Level: LevelBad}, 2: {Level: LevelOK}},
		},
		Confirm: map[string]string{}, Rates: map[int64]float64{1: 1, 2: 1, 3: 1, 4: 1}}
	s := Simulate(5, in)
	by := map[int64]SuggestRow{}
	total := 0.0
	for _, r := range s.Rows {
		by[r.AccountID] = r
		total += r.After
	}
	if by[4].After > by[4].Before || by[3].After >= by[3].Before {
		t.Fatalf("problem accounts must lose share: %+v", s.Rows)
	}
	if s.Moved > in.Config.MaxMove+0.5 {
		t.Fatalf("max move guard: moved=%v capped=%v", s.Moved, s.Capped)
	}
	if total < 99.5 || total > 100.5 {
		t.Fatalf("shares must sum to 100, got %v", total)
	}
	if by[3].Action != "factor" || by[3].Factor <= 0 || by[3].Factor >= 1 {
		t.Fatalf("suggested adjustment %+v", by[3])
	}
	// 确认后的门槛维度直接停止分配，不受每小时上限限制
	in.Confirm["4:1"] = EventConfirmed
	s = Simulate(5, in)
	for _, r := range s.Rows {
		if r.AccountID == 4 && (r.After != 0 || r.Action != "pause") {
			t.Fatalf("confirmed gate must stop: %+v", r)
		}
	}
	// 误报按正常处理
	in.Confirm["4:1"] = EventFalse
	in.DimRes[3][2] = DimResult{Level: LevelOK}
	s = Simulate(5, in)
	if s.Moved != 0 {
		t.Fatalf("false positive must not move traffic: %+v", s)
	}
}

func TestSimulateNoReceiver(t *testing.T) {
	dims := []Dimension{{ID: 2, Name: "速度", Role: RoleScore, Enabled: true}}
	s := Simulate(1, SimInput{Config: DefaultConfig(), Dims: dims, Accounts: []int64{1}, Shares: map[int64]float64{1: 100},
		DimRes: map[int64]map[int64]DimResult{1: {2: {Level: LevelBad}}}})
	if !s.NoReceiver || s.Rows[0].After != 100 {
		t.Fatalf("single account group must not move: %+v", s)
	}
}

func TestPredictShares(t *testing.T) {
	p := PredictShares(map[int64]float64{1: 50, 2: 50}, map[int64]float64{2: 0.3})
	if p[1] < 76 || p[1] > 77.5 || p[2] < 22.5 || p[2] > 24 {
		t.Fatalf("predict %v", p)
	}
}

func TestNormalizeModel(t *testing.T) {
	for in, want := range map[string]string{"claude-opus-5.5": "claude-opus-5-5", "claude-opus-5-5-20261101": "claude-opus-5-5",
		"openai/gpt-5.6-luna": "gpt-5-6-luna", "GPT-5.6-Luna": "gpt-5-6-luna"} {
		if got := normalizeModel(in); got != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
}

func TestExtractHTML(t *testing.T) {
	h, err := extractHTML("这是作品：\n```html\n<!DOCTYPE html><html><body>hi</body></html>\n```\n说明")
	if err != nil || !strings.HasPrefix(h, "<!DOCTYPE") {
		t.Fatalf("%q %v", h, err)
	}
	if _, err := extractHTML("没有作品"); err == nil {
		t.Fatal("expected error")
	}
}

func TestGradedJudgeWindow(t *testing.T) {
	item := Item{ID: 7, Type: "graded", Rule: json.RawMessage(`{"window":10,"how":"fixed","warn_below":6,"bad_below":3}`)}
	var runs []Run
	for i := 0; i < 10; i++ {
		o := OutcomeFail
		if i%3 == 0 {
			o = OutcomePass
		}
		runs = append(runs, Run{ID: int64(100 - i), ItemID: 7, CredKey: "c", Model: "m", Outcome: o})
	}
	runs = append(runs, Run{ID: 200, ItemID: 7, CredKey: "c", Model: "m", Outcome: OutcomeError})
	cat := &Catalog{Accounts: map[int64]*CatAccount{1: {ID: 1, CredKey: "c"}}, Groups: map[int64]*CatGroup{1: {ID: 1, Accounts: []int64{1}}}}
	env := NewJudgeEnv(time.Now(), cat, newTrafficSet(time.Now()), NewRegistry(GradedDetector{}), runs)
	v := GradedDetector{}.Judge(env, item, Subject{GroupID: 1, AccountID: 1, Model: "m", CredKey: "c"})
	if v.Level != LevelWarn || v.Short != "4/10 答对" || !strings.Contains(v.Long, "最近 1 次请求都出错") {
		t.Fatalf("%+v", v)
	}
}

func TestFingerprintUnknownConfidenceNotCounted(t *testing.T) {
	d := &FingerprintDetector{}
	r := FingerprintRule{BadConsecutive: 3, MinConfidence: 60}
	if got := d.classify(r, true, false, 0); got != OutcomeLow {
		t.Fatalf("名字对上但没有置信度，应不计入，got %s", got)
	}
	if got := d.classify(r, false, false, 0); got != OutcomeLow {
		t.Fatalf("名字不对且没有置信度，应不计入，got %s", got)
	}
	if got := d.classify(r, true, true, 95); got != OutcomePass {
		t.Fatalf("got %s", got)
	}
	if got := d.classify(r, false, true, 80); got != OutcomeFail {
		t.Fatalf("got %s", got)
	}
	if got := d.classify(r, false, true, 40); got != OutcomeLow {
		t.Fatalf("got %s", got)
	}
}

// 报错率：同样是 2.4%，只有 1 次报错不算；够 3 次才算。持续时长不超过账号接入至今的时长，也不重复算和「现在」窗口重叠的小时桶。
func TestTrafficMinEventsAndDuration(t *testing.T) {
	now := time.Date(2026, 11, 10, 10, 20, 0, 0, time.UTC)
	cat := &Catalog{Accounts: map[int64]*CatAccount{}, Groups: map[int64]*CatGroup{}, byCred: map[string][]int64{}}
	for _, a := range []*CatAccount{{ID: 1, CredKey: "a", Groups: []int64{10}, CreatedAt: now.Add(-40 * time.Minute)}, {ID: 2, CredKey: "b", Groups: []int64{10}, CreatedAt: now.Add(-48 * time.Hour)}} {
		cat.Accounts[a.ID] = a
		cat.byCred[a.CredKey] = []int64{a.ID}
	}
	cat.Groups[10] = &CatGroup{ID: 10, Accounts: []int64{1, 2}}
	item := Item{ID: 3, Type: "traffic", Params: json.RawMessage(`{"metric":"err"}`), Rule: json.RawMessage(`{"bases":["peer"],"min_peers":1,"min_samples":20,"mode":"diff","warn":2,"bad":5,"floor":1,"duration_minutes":0}`)}
	s := Subject{GroupID: 10, AccountID: 1, Model: "m", CredKey: "a"}
	judge := func(errs int64) Verdict {
		ts := newTrafficSet(now)
		ts.Now[TKey{10, 1, "m"}] = Agg{Requests: 40 * errs, Errors: errs}
		ts.Now[TKey{10, 2, "m"}] = Agg{Requests: 100}
		// 和「现在」窗口重叠的 10:00 桶、以及 09:00 桶都有问题
		ts.Hourly[TKey{10, 1, "m"}] = map[int64]Agg{now.Truncate(time.Hour).Unix(): {Requests: 40, Errors: 1}, now.Truncate(time.Hour).Add(-time.Hour).Unix(): {Requests: 40, Errors: 1}}
		ts.Hourly[TKey{10, 2, "m"}] = map[int64]Agg{now.Truncate(time.Hour).Unix(): {Requests: 100}, now.Truncate(time.Hour).Add(-time.Hour).Unix(): {Requests: 100}}
		return TrafficDetector{}.Judge(NewJudgeEnv(now, cat, ts, NewRegistry(TrafficDetector{}), nil), item, s)
	}
	if v := judge(1); v.Level != LevelOK || !strings.Contains(v.Long, "只出现 1 次") {
		t.Fatalf("1 次报错不该算：%+v", v)
	}
	v := judge(3)
	if !v.Level.Problem() {
		t.Fatalf("3 次报错应该算：%+v", v)
	}
	if d := v.Evidence["duration_minutes"].(int); d > 40 {
		t.Fatalf("持续时长 %d 分钟，超过了账号接入时长 40 分钟", d)
	}
}

// 已经暂停的账号：建议不再要求挪动流量，标「已调整到位」
func TestSimulateSettledByCurrentAdjustment(t *testing.T) {
	dims := []Dimension{{ID: 1, Name: "模型身份", Role: RoleGate, Enabled: true, Gate: Gate{Before: "cut", BeforePct: 3, After: "stop"}}}
	in := SimInput{Config: DefaultConfig(), Dims: dims, Accounts: []int64{1, 2}, Shares: map[int64]float64{1: 58, 2: 42},
		DimRes:  map[int64]map[int64]DimResult{1: {1: {Level: LevelBad, Long: "识别为别的模型"}}, 2: {1: {Level: LevelOK}}},
		Confirm: map[string]string{}, Rates: map[int64]float64{1: 1, 2: 1}, Names: map[int64]string{1: "a", 2: "b"}}
	if s := Simulate(1, in); s.Moved <= 0 {
		t.Fatalf("没调整时应该建议挪动：%+v", s)
	}
	in.Current = map[int64]float64{1: 0}
	s := Simulate(1, in)
	if s.Moved != 0 || !strings.Contains(s.Summary, "已调整到位") {
		t.Fatalf("已暂停后不该再挪：%+v", s)
	}
	for _, r := range s.Rows {
		if r.AccountID == 1 && !r.Settled {
			t.Fatalf("a 应标已调整到位：%+v", r)
		}
	}
	in.Current = map[int64]float64{1: 0.9} // 调得比建议松：仍然建议
	if s := Simulate(1, in); s.Moved <= 0 {
		t.Fatalf("调整比建议松时仍应建议：%+v", s)
	}
}

// 判分题：有效回答还没攒满窗口时，阈值按比例换算（5 次答对 4 次不该判可疑，5 次答对 2 次应判可疑）
func TestGradedJudgeScalesBeforeWindowFull(t *testing.T) {
	item := Item{ID: 7, Type: "graded", Rule: json.RawMessage(`{"window":10,"how":"fixed","warn_below":6,"bad_below":3}`)}
	cat := &Catalog{Accounts: map[int64]*CatAccount{1: {ID: 1, CredKey: "c"}}, Groups: map[int64]*CatGroup{1: {ID: 1, Accounts: []int64{1}}}}
	judge := func(pass int) Verdict {
		var runs []Run
		for i := 0; i < 5; i++ {
			o := OutcomeFail
			if i < pass {
				o = OutcomePass
			}
			runs = append(runs, Run{ID: int64(100 - i), ItemID: 7, CredKey: "c", Model: "m", Outcome: o})
		}
		env := NewJudgeEnv(time.Now(), cat, newTrafficSet(time.Now()), NewRegistry(GradedDetector{}), runs)
		return GradedDetector{}.Judge(env, item, Subject{GroupID: 1, AccountID: 1, Model: "m", CredKey: "c"})
	}
	if v := judge(4); v.Level != LevelOK {
		t.Fatalf("5 次答对 4 次应正常：%+v", v)
	}
	if v := judge(2); v.Level != LevelWarn || !strings.Contains(v.Long, "换算") {
		t.Fatalf("5 次答对 2 次应可疑：%+v", v)
	}
	if v := judge(1); v.Level != LevelBad {
		t.Fatalf("5 次答对 1 次应异常：%+v", v)
	}
}

// 没有流量的问题账号：不建议暂停 / 降份额（0 → 0 没有意义）
func TestSimulateIdleProblemAccountNoAction(t *testing.T) {
	dims := []Dimension{{ID: 1, Name: "模型身份", Role: RoleGate, Enabled: true, Gate: Gate{Before: "cut", BeforePct: 3, After: "stop"}}}
	in := SimInput{Config: DefaultConfig(), Dims: dims, Accounts: []int64{1, 2}, Shares: map[int64]float64{1: 0, 2: 100},
		DimRes:  map[int64]map[int64]DimResult{1: {1: {Level: LevelWarn}}, 2: {1: {Level: LevelOK}}},
		Confirm: map[string]string{}, Rates: map[int64]float64{1: 1, 2: 1}}
	for _, r := range Simulate(1, in).Rows {
		if r.AccountID == 1 && r.Action != "normal" {
			t.Fatalf("没流量的账号不该有调整建议：%+v", r)
		}
	}
}

// 账号范围规则：多选、命中任意一条就测；没选的规则不生效；排除优先
func TestPlannerScopes(t *testing.T) {
	now := time.Now()
	cat := &Catalog{Accounts: map[int64]*CatAccount{}, Groups: map[int64]*CatGroup{}, byCred: map[string][]int64{}}
	mk := func(id int64, age time.Duration) {
		a := &CatAccount{ID: id, Platform: "openai", Status: "active", Schedulable: true, Direct: true, CredKey: fmt.Sprint("c", id), Groups: []int64{1}, CreatedAt: now.Add(-age)}
		cat.Accounts[id] = a
		cat.byCred[a.CredKey] = []int64{id}
	}
	mk(1, 30*24*time.Hour) // 老账号，有流量
	mk(2, time.Hour)       // 新账号，没流量
	mk(3, 30*24*time.Hour) // 老账号，没流量
	cat.Groups[1] = &CatGroup{ID: 1, Accounts: []int64{1, 2, 3}}
	ts := newTrafficSet(now)
	ts.Now[TKey{1, 1, "m"}] = Agg{Requests: 100}
	ts.Hourly[TKey{1, 1, "m"}] = map[int64]Agg{now.Truncate(time.Hour).Unix(): {Requests: 100}}
	item := Item{ID: 1, Type: "graded"}
	p := Planner{Registry: NewRegistry(GradedDetector{})}
	cover := func(scopes []PlanScope, exclude ...int64) map[int64]bool {
		out := map[int64]bool{}
		for _, r := range p.Cover(context.Background(), Plan{Platform: "openai", Models: []string{"m"}, Scopes: scopes, ExcludeIDs: exclude, Enabled: true}, item, cat, ts) {
			out[r.AccountID] = r.OK
		}
		return out
	}
	traffic := PlanScope{Type: "traffic", Params: map[string]float64{"min_share": 5}}
	newer := PlanScope{Type: "new", Params: map[string]float64{"days": 3}}
	manual := PlanScope{Type: "manual", IDs: []int64{3}}
	if c := cover([]PlanScope{traffic}); !c[1] || c[2] || c[3] {
		t.Fatalf("只选流量：%v", c)
	}
	if c := cover([]PlanScope{traffic, newer}); !c[1] || !c[2] || c[3] {
		t.Fatalf("流量 + 新接入：%v", c)
	}
	if c := cover([]PlanScope{manual}); c[1] || c[2] || !c[3] {
		t.Fatalf("只选指定账号：%v", c)
	}
	if c := cover([]PlanScope{traffic, newer, manual}, 2); !c[1] || c[2] || !c[3] {
		t.Fatalf("排除优先：%v", c)
	}
	if _, err := normalizeScopes(nil); err == nil {
		t.Fatal("一种范围都不选应该报错")
	}
	if _, err := normalizeScopes([]PlanScope{{Type: "manual"}}); err == nil {
		t.Fatal("指定账号不选账号应该报错")
	}
}
