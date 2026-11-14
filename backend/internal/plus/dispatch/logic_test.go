//go:build unit

package dispatch

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/plus/channelstatus"
)

// ---------- 测试数据：分组 1 里账号 10、11；10 有模型 a、b，11 只有 a ----------

func testCatalog() *Catalog {
	mk := func(id int64, models ...string) *CatAccount {
		mm := map[string]any{}
		for _, m := range models {
			mm[m] = m
		}
		return &CatAccount{ID: id, Name: "acc" + itoa(id), Platform: "anthropic", Type: "apikey", Status: "active", Schedulable: true,
			Groups: []int64{1}, creds: map[string]any{"model_mapping": mm}, memo: &sync.Map{}, Models: models, CredKey: "k" + itoa(id)}
	}
	cat := &Catalog{Accounts: map[int64]*CatAccount{10: mk(10, "a", "b"), 11: mk(11, "a")},
		Groups: map[int64]*CatGroup{1: {ID: 1, Name: "g", Platform: "anthropic", Accounts: []int64{10, 11}}}, GroupIDs: []int64{1}}
	return cat
}

func testTraffic(now time.Time, rows ...TRow) *TrafficSet {
	b := now.Truncate(time.Hour).Unix()
	for i := range rows {
		if rows[i].Bucket == 0 {
			rows[i].Bucket = b
		}
	}
	return NewTrafficSet(now, rows)
}

func row(acc int64, model string, n int64) TRow {
	return TRow{TKey: TKey{Group: 1, Account: acc, Model: model}, Agg: Agg{Requests: n}}
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.6 }

func TestGroupViewObservedAndLocked(t *testing.T) {
	now := time.Now()
	// 线路 a：10 跑 60、11 跑 40；线路 b：只有 10 能跑，100
	l := NewLanes(testCatalog(), testTraffic(now, row(10, "a", 60), row(11, "a", 40), row(10, "b", 100)), nil)
	v := l.GroupView(10, "", 1, nil)
	if !near(v.Pct, 80) || !near(v.Locked, 50) || !near(v.Reach, 100) || len(v.Alone) != 1 || v.Alone[0] != "b" {
		t.Fatalf("%+v", v)
	}
	sum := 0.0
	for _, r := range v.Rows {
		sum += r.Pct
	}
	if !near(sum, 100) {
		t.Fatalf("rows must add up to 100: %v", v.Rows)
	}
}

func TestPauseKeepsOnlyLaneUnlessHard(t *testing.T) {
	now := time.Now()
	l := NewLanes(testCatalog(), testTraffic(now, row(10, "a", 60), row(11, "a", 40), row(10, "b", 100)), nil)
	zero := 0.0
	soft := setRules(nil, []RuleChange{{AccountID: 10, GroupID: 1, Share: &zero}})
	v := l.GroupView(10, "", 1, soft)
	if !near(v.Pct, 50) || len(v.Kept) != 1 || len(v.Dead) != 0 {
		t.Fatalf("soft pause keeps lane b: %+v", v)
	}
	hard := setRules(nil, []RuleChange{{AccountID: 10, GroupID: 1, Share: &zero, Hard: true}})
	v = l.GroupView(10, "", 1, hard)
	if !near(v.Pct, 0) || len(v.Dead) != 1 || v.Dead[0] != "b" {
		t.Fatalf("hard pause kills lane b: %+v", v)
	}
}

func TestShareMovesTrafficToOthers(t *testing.T) {
	now := time.Now()
	l := NewLanes(testCatalog(), testTraffic(now, row(10, "a", 60), row(11, "a", 40)), nil)
	rules := l.setShare(nil, 10, "", 1, 0.2)
	v := l.GroupView(10, "", 1, rules)
	if !near(v.Pct, 20) {
		t.Fatalf("got %+v", v)
	}
	for _, r := range v.Rows {
		if r.ID == 11 && !near(r.Pct, 80) {
			t.Fatalf("receiver %+v", r)
		}
	}
}

func TestSetShareRescalesOtherTargets(t *testing.T) {
	now := time.Now()
	l := NewLanes(testCatalog(), testTraffic(now, row(10, "a", 60), row(11, "a", 40)), nil)
	rules := l.setShare(nil, 11, "", 1, 0.5)
	rules = l.setShare(rules, 10, "", 1, 0.8) // 只剩 0.2 的空间：11 让出来
	r := exactRule(rules, 11, 1, "")
	if r == nil || !near(r.Share*100, 20) {
		t.Fatalf("other target must shrink to fit: %+v", rules)
	}
}

func TestModelRuleBeatsAllModels(t *testing.T) {
	zero, one := 0.0, 1.0
	rules := setRules(nil, []RuleChange{{AccountID: 10, GroupID: 1, Share: &zero}, {AccountID: 10, GroupID: 1, Model: "a", Share: &one}})
	if r := ruleOf(rules, 1, 10, "a"); r == nil || r.Share != 1 {
		t.Fatal("model rule must win")
	}
	if r := ruleOf(rules, 1, 10, "b"); r == nil || r.Share != 0 {
		t.Fatal("all-models rule applies to others")
	}
}

// ---------- 检测结论 ----------

func runsOf(outcomes ...string) []Run {
	out := make([]Run, len(outcomes))
	for i, o := range outcomes {
		out[i] = Run{ID: int64(len(outcomes) - i), Outcome: o, Detail: map[string]any{"prediction": "x"}}
	}
	return out // 新 → 旧
}

func TestFingerprintVerdict(t *testing.T) {
	d := &FingerprintDetector{}
	item := Item{Rule: json.RawMessage(`{"window":10,"bad_min":3,"min_confidence":60,"min_valid":3}`)}
	if v := d.Judge(item, runsOf("fail", "pass", "fail", "low", "fail")); v.Level != LevelBad || v.Short != "像 x" {
		t.Fatalf("%+v", v)
	}
	if v := d.Judge(item, runsOf("fail", "pass", "pass", "pass")); v.Level != LevelOK || v.Bad != 1 {
		t.Fatalf("%+v", v)
	}
	if v := d.Judge(item, runsOf("pass", "low", "error")); v.Level != LevelFew {
		t.Fatalf("%+v", v)
	}
	// 被标为误报的不算
	if v := d.Judge(item, runsOf("ignored", "ignored", "ignored", "pass", "pass", "pass")); v.Level != LevelOK || !v.Ignored {
		t.Fatalf("%+v", v)
	}
}

func TestGradedVerdictScalesBeforeWindowFull(t *testing.T) {
	d := GradedDetector{}
	item := Item{Rule: json.RawMessage(`{"window":10,"bad_below":3,"min_valid":5}`)}
	if v := d.Judge(item, runsOf("pass", "pass", "pass", "pass", "pass")); v.Level != LevelOK {
		t.Fatalf("5/5 must pass: %+v", v)
	}
	if v := d.Judge(item, runsOf("fail", "fail", "fail", "fail", "pass")); v.Level != LevelBad {
		t.Fatalf("1/5 must fail: %+v", v)
	}
}

func TestFaultVerdict(t *testing.T) {
	rs := runsOf("error", "error", "error", "pass")
	rs[0].Error = "HTTP 404"
	if v := faultVerdict(&Verdict{Level: LevelFew}, rs, 3); v.Level != LevelFault {
		t.Fatalf("%+v", v)
	}
	if v := faultVerdict(&Verdict{Level: LevelBad}, rs, 3); v.Level != LevelBad {
		t.Fatal("bad must not be overwritten")
	}
}

func TestSettleIgnoresOnlyMarkedRuns(t *testing.T) {
	reg := NewRegistry(GradedDetector{})
	it := Item{Type: "graded"}
	ig := Ignore{UpToRun: 5}
	if r := settle(reg, it, Run{ID: 5, Outcome: OutcomeFail}, ig); r.Outcome != OutcomeIgnored {
		t.Fatal("marked fail must be ignored")
	}
	if r := settle(reg, it, Run{ID: 6, Outcome: OutcomeFail}, ig); r.Outcome != OutcomeFail {
		t.Fatal("new fail must count again")
	}
}

// ---------- 健康分 ----------

func TestHealthCurrentWidensWindow(t *testing.T) {
	now := time.Now()
	nb := now.Truncate(time.Hour).Unix()
	tr := NewTrafficSet(now, []TRow{
		{Bucket: nb, TKey: TKey{1, 10, "a"}, Agg: Agg{Requests: 20}},
		{Bucket: nb - 3*3600, TKey: TKey{1, 10, "a"}, Agg: Agg{Requests: 40, Errors: 10}},
	})
	h := Health{Rules: channelstatus.DefaultRules()}
	c := h.Current(tr, TFilter{Account: 10})
	if c.Hours != 4 || c.Score == nil || c.Agg.Total() != 70 {
		t.Fatalf("%+v", c)
	}
	bars := h.Timeline(tr, TFilter{Account: 10}, GranHour, 24)
	if len(bars) != 24 || bars[23].N != 20 || bars[20].N != 50 {
		t.Fatalf("bars %+v", bars[20:])
	}
}

func TestCachedModelsReadsBundleHeader(t *testing.T) {
	bundle := []byte(`{"models":[{"id":"gemini-4-argon"},{"id":"claude-fable-5.2"}]}`)
	buf := make([]byte, 4, 4+len(bundle)+8)
	binary.LittleEndian.PutUint32(buf, uint32(len(bundle)))
	buf = append(append(buf, bundle...), []byte("detector")...)
	p := filepath.Join(t.TempDir(), "data.bin")
	if err := os.WriteFile(p, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	ms := cachedModels(p)
	if ms["claude-fable-5-2"] != "claude-fable-5.2" || ms["gemini-4-argon"] != "gemini-4-argon" || len(ms) != 2 {
		t.Fatalf("%v", ms)
	}
	if cachedModels(filepath.Join(t.TempDir(), "missing")) != nil {
		t.Fatal("missing file should be nil")
	}
}

// 「调用失败多」只看最近 6 小时，最近一个有请求的小时已经不失败了就不挂（真实情况：tk image 17 点那波 404 已恢复，标签却挂到第二天）
func TestFailModelOnlyRecentAndNotRecovered(t *testing.T) {
	now := time.Date(2026, 11, 14, 21, 30, 0, 0, time.UTC)
	nb := now.Truncate(time.Hour).Unix()
	acc := &CatAccount{ID: 10}
	row := func(hoursAgo int64, ok, errs int64) TRow {
		return TRow{Bucket: nb - hoursAgo*3600, TKey: TKey{Group: 1, Account: 10, Model: "a"}, Agg: Agg{Requests: ok, Errors: errs}}
	}
	st := func(rows ...TRow) *state {
		return &state{tr: NewTrafficSet(now, rows), health: Health{Rules: channelstatus.DefaultRules()}}
	}
	if f := st(row(4, 5, 60), row(1, 4, 6)).failModel(acc, "a"); f == nil || f.Failed != 66 || f.Total != 75 || f.Hours != failWindowHours {
		t.Fatalf("still failing: %+v", f)
	}
	if f := st(row(4, 5, 60), row(1, 4, 0)).failModel(acc, "a"); f != nil {
		t.Fatalf("recovered in the latest hour: %+v", f)
	}
	if f := st(row(10, 5, 60), row(1, 45, 5)).failModel(acc, "a"); f != nil { // 算上 10 小时前那波就是异常，只看 6 小时只是降级
		t.Fatalf("failures older than 6 hours do not count: %+v", f)
	}
}
