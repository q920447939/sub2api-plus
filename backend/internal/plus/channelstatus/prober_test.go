//go:build unit

package channelstatus

import (
	"context"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 4, 2, 0, 0, 0, time.UTC) // 东八区 10:00

func ptrT(t time.Time) *time.Time { return &t }
func ptrB(b bool) *bool           { return &b }
func ptrI(i int) *int             { return &i }

func pol() ProbePolicy { return ProbePolicy{P: DefaultProbe(), Loc: time.FixedZone("CST", 8*3600)} }

// ─── 策略 ────────────────────────────────────────────────────────────────

func TestPolicy_CooldownBackoff(t *testing.T) {
	p := pol() // 起点 10 分钟，上限 6 小时
	cases := []struct {
		streak  int
		traffic bool
		want    time.Duration
	}{
		{0, false, 10 * time.Minute},
		{1, false, 20 * time.Minute},
		{3, false, 80 * time.Minute},
		{10, false, 6 * time.Hour},
		{10, true, 10 * time.Minute}, // 出现真实流量立即回到起点
	}
	for _, c := range cases {
		if got := p.Cooldown(c.streak, c.traffic); got != c.want {
			t.Errorf("streak=%d traffic=%v want %v got %v", c.streak, c.traffic, c.want, got)
		}
	}
}

func TestPolicy_UsableResult(t *testing.T) {
	p := pol()
	ok := &ProbeState{LastCheckedAt: ptrT(t0), LastOK: ptrB(true)}
	if !p.UsableResult(ok, t0.Add(29*time.Minute)) || p.UsableResult(ok, t0.Add(31*time.Minute)) {
		t.Fatal("成功结果只在 30 分钟保质期内有效，过期后应变为暂无数据")
	}
	bad := &ProbeState{LastCheckedAt: ptrT(t0), LastOK: ptrB(false)}
	if !p.UsableResult(bad, t0.Add(5*time.Hour)) {
		t.Fatal("失败结果应持续保持异常，不应过期变灰")
	}
	if p.UsableResult(nil, t0) {
		t.Fatal("没有探测过不应有结论")
	}
}

func TestPolicy_ShouldProbe(t *testing.T) {
	p := pol()
	timeout := 30 * time.Second
	if !p.ShouldProbe(nil, t0, false, timeout) {
		t.Fatal("从未探测过应触发")
	}
	// 成功、保质期内 → 不探
	st := &ProbeState{LastStartedAt: ptrT(t0), LastCheckedAt: ptrT(t0), LastOK: ptrB(true), IdleStreak: 3, Day: "2026-11-04", DayCount: 1}
	if p.ShouldProbe(st, t0.Add(20*time.Minute), false, timeout) {
		t.Fatal("保质期内不应探测")
	}
	// 过期但仍在退避冷却内（streak 3 → 80 分钟）→ 不探
	if p.ShouldProbe(st, t0.Add(60*time.Minute), false, timeout) {
		t.Fatal("退避冷却内不应探测")
	}
	// 同一时刻若窗口内出现真实流量，冷却回到 10 分钟 → 探
	if !p.ShouldProbe(st, t0.Add(60*time.Minute), true, timeout) {
		t.Fatal("有真实流量时冷却应清零")
	}
	if !p.ShouldProbe(st, t0.Add(81*time.Minute), false, timeout) {
		t.Fatal("冷却结束应探测")
	}
	// 失败 → 按 5 分钟复测
	fail := &ProbeState{LastStartedAt: ptrT(t0), LastCheckedAt: ptrT(t0), LastOK: ptrB(false), IdleStreak: 5, Day: "2026-11-04", DayCount: 1}
	if p.ShouldProbe(fail, t0.Add(4*time.Minute), false, timeout) || !p.ShouldProbe(fail, t0.Add(5*time.Minute), false, timeout) {
		t.Fatal("失败后应按复测间隔重试")
	}
	// 每日上限
	fail.DayCount = 24
	if p.ShouldProbe(fail, t0.Add(time.Hour), false, timeout) {
		t.Fatal("达到每日上限不应探测")
	}
	// 跨天（东八区）后计数重置
	if !p.ShouldProbe(fail, t0.Add(14*time.Hour), false, timeout) {
		t.Fatal("新的一天应恢复探测")
	}
	// 进行中：已开始未完成，2 倍超时内不重复
	running := &ProbeState{LastStartedAt: ptrT(t0.Add(time.Hour)), LastCheckedAt: ptrT(t0), LastOK: ptrB(false)}
	if p.ShouldProbe(running, t0.Add(time.Hour+30*time.Second), false, timeout) {
		t.Fatal("探测进行中不应重复触发")
	}
}

// ─── 探测器 ──────────────────────────────────────────────────────────────

type fakeProbeStore struct {
	states   map[int64]*ProbeState
	claimed  []int64
	finished map[int64]ProbeResult
	streaks  map[int64]int
}

func (f *fakeProbeStore) LoadStates(_ context.Context, ids []int64) (map[int64]*ProbeState, error) {
	out := map[int64]*ProbeState{}
	for _, id := range ids {
		if st, ok := f.states[id]; ok {
			out[id] = st
		}
	}
	return out, nil
}
func (f *fakeProbeStore) Claim(_ context.Context, id int64, _ *time.Time, _ time.Time, _ string, _ int, _ bool) (bool, error) {
	f.claimed = append(f.claimed, id)
	return true, nil
}
func (f *fakeProbeStore) Finish(_ context.Context, id int64, _ time.Time, ok bool, lat int, msg string, streak int) error {
	if f.finished == nil {
		f.finished, f.streaks = map[int64]ProbeResult{}, map[int64]int{}
	}
	f.finished[id] = ProbeResult{OK: ok, LatencyMs: lat, Message: msg}
	f.streaks[id] = streak
	return nil
}

type fakeKeys map[int64]int64 // keyID → groupID

func (f fakeKeys) ResolveKey(_ context.Context, id int64) (string, int64, error) {
	g, ok := f[id]
	if !ok {
		return "", 0, ErrKeyUnavailable
	}
	return "sk-test", g, nil
}
func (f fakeKeys) ListUserKeys(context.Context, int64) ([]ProbeKey, error) { return nil, nil }

type fakeRunner struct {
	res  ProbeResult
	reqs []ProbeRequest
}

func (f *fakeRunner) Run(_ context.Context, r ProbeRequest) ProbeResult {
	f.reqs = append(f.reqs, r)
	return f.res
}

func probeCfg() Config {
	cfg := DefaultConfig()
	cfg.Probe.Enabled = true
	cfg.Probe.Endpoint = "https://api.example.com"
	cfg.Probe.Groups = []GroupProbe{
		{GroupID: 1, Enabled: true, APIKeyID: 11, Model: "claude-haiku-4-5"},
		{GroupID: 2, Enabled: false, APIKeyID: 12, Model: "x"},
		{GroupID: 3, Enabled: true, APIKeyID: 99, Model: "gpt-5-mini"}, // Key 属于别的分组
	}
	return cfg
}

func newTestProber(store *fakeProbeStore, runner *fakeRunner) *Prober {
	return NewProber(ProberDeps{
		Store:    store,
		Keys:     fakeKeys{11: 1, 99: 7},
		Catalog:  fakeCatalog{{ID: 1, Platform: "anthropic"}, {ID: 2, Platform: "openai"}, {ID: 3, Platform: "openai"}},
		Runner:   runner,
		Location: time.FixedZone("CST", 8*3600),
		Now:      func() time.Time { return t0 },
		Dispatch: func(f func()) { f() }, // 同步执行，便于断言
	})
}

func TestProber_OnlyEnabledGroupsAndKeyCheck(t *testing.T) {
	store := &fakeProbeStore{states: map[int64]*ProbeState{}}
	runner := &fakeRunner{res: ProbeResult{OK: true, LatencyMs: 1200}}
	p := newTestProber(store, runner)

	out := p.Judge(context.Background(), probeCfg(), map[int64]int64{1: 0, 2: 0, 3: 3})
	if len(out) != 0 {
		t.Fatalf("首次探测是异步的，本次不应有结论: %+v", out)
	}
	if len(store.claimed) != 2 {
		t.Fatalf("只应探测启用的分组 1、3，got %v", store.claimed)
	}
	if len(runner.reqs) != 1 || runner.reqs[0].Format != FormatAnthropic || runner.reqs[0].Model != "claude-haiku-4-5" {
		t.Fatalf("分组 1 应按 anthropic 格式探测: %+v", runner.reqs)
	}
	if r := store.finished[3]; r.OK || r.Message == "" {
		t.Fatalf("Key 不属于该分组应记为失败: %+v", r)
	}
	if store.streaks[1] != 1 || store.streaks[3] != 0 {
		t.Fatalf("无流量 streak+1，有流量清零: %+v", store.streaks)
	}
}

func TestProber_VerdictFromResult(t *testing.T) {
	store := &fakeProbeStore{states: map[int64]*ProbeState{
		1: {GroupID: 1, LastStartedAt: ptrT(t0.Add(-time.Minute)), LastCheckedAt: ptrT(t0.Add(-time.Minute)), LastOK: ptrB(true), LatencyMs: ptrI(6500)},
		3: {GroupID: 3, LastStartedAt: ptrT(t0.Add(-time.Minute)), LastCheckedAt: ptrT(t0.Add(-time.Minute)), LastOK: ptrB(false)},
	}}
	runner := &fakeRunner{}
	out := newTestProber(store, runner).Judge(context.Background(), probeCfg(), map[int64]int64{1: 0, 3: 0})
	// 成功 + 延迟 6500ms：错误维度 100、TTFT 50 → 87.5 → 正常
	if v := out[1]; v.Status != StatusUp || v.Score == nil || !approx(*v.Score, 87.5) {
		t.Fatalf("成功结果应按同口径评分: %+v", v)
	}
	if v := out[3]; v.Status != StatusDown {
		t.Fatalf("失败结果应为异常: %+v", v)
	}
	if len(runner.reqs) != 0 {
		t.Fatal("结果新鲜 / 失败未到复测间隔时不应再探测")
	}
}

func TestProber_DisabledGlobally(t *testing.T) {
	store := &fakeProbeStore{}
	cfg := probeCfg()
	cfg.Probe.Enabled = false
	if out := newTestProber(store, &fakeRunner{}).Judge(context.Background(), cfg, map[int64]int64{1: 0}); out != nil || len(store.claimed) != 0 {
		t.Fatal("总开关关闭时不应探测")
	}
}

func TestProbe_Validate(t *testing.T) {
	cfg := probeCfg()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("合法配置: %v", err)
	}
	cfg.Probe.Groups[0].Model = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("启用的分组缺模型应报错")
	}
	cfg = probeCfg()
	cfg.Probe.Endpoint = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("启用探测缺网关地址应报错")
	}
	if ResolveFormat(FormatAuto, "openai") != FormatOpenAIResponses || ResolveFormat(FormatAuto, "grok") != FormatOpenAIChat {
		t.Fatal("自动格式映射不对")
	}
}
