//go:build unit

package channelstatus

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// ─── fakes ───────────────────────────────────────────────────────────────

type fakeStore struct {
	cfg Config
	ver int64
}

func (f *fakeStore) Load(context.Context) (Config, int64, error) { return f.cfg, f.ver, nil }
func (f *fakeStore) Save(_ context.Context, c Config, v int64) (int64, error) {
	if v != f.ver {
		return 0, ErrVersionMismatch
	}
	f.cfg, f.ver = c, f.ver+1
	return f.ver, nil
}

type fakeCatalog []GroupInfo

func (f fakeCatalog) ListGroups(context.Context) ([]GroupInfo, error) { return f, nil }

// fakeSource 对任意窗口：current 窗口（bucket < 1h）返回 cur，历史返回 hist
type fakeSource struct {
	cur   map[int64]Signal
	hist  map[int64]Signal // 只放在最后一格
	calls int32
}

func (f *fakeSource) Fetch(_ context.Context, ids []int64, start, end time.Time, bucket time.Duration) (map[int64]map[int64]Signal, error) {
	atomic.AddInt32(&f.calls, 1)
	out := map[int64]map[int64]Signal{}
	for _, id := range ids {
		if bucket < time.Hour {
			if s, ok := f.cur[id]; ok {
				out[id] = map[int64]Signal{start.Unix(): s}
			}
			continue
		}
		if s, ok := f.hist[id]; ok {
			out[id] = map[int64]Signal{end.Add(-bucket).Unix(): s}
		}
	}
	return out, nil
}

type fakeJudge struct{ got map[int64]int64 }

func (f *fakeJudge) Judge(_ context.Context, _ Config, lacking map[int64]int64) map[int64]Verdict {
	f.got = lacking
	s := 100.0
	return map[int64]Verdict{2: {Status: StatusUp, Score: &s}}
}

var testNow = time.Date(2026, 11, 4, 10, 30, 15, 0, time.UTC)

func newTestService(cfg Config, src SignalSource, judge FallbackJudge) *Service {
	return NewService(Deps{
		Config: &fakeStore{cfg: cfg, ver: 1},
		Catalog: fakeCatalog{
			{ID: 1, Name: "Claude Max", Platform: "anthropic"},
			{ID: 2, Name: "Claude 特惠", Platform: "anthropic"},
			{ID: 3, Name: "GPT", Platform: "openai"},
		},
		Source:   src,
		Fallback: judge,
		Location: time.UTC,
		Now:      func() time.Time { return testNow },
	})
}

// ─── tests ───────────────────────────────────────────────────────────────

func TestSnapshot_DisabledReturnsErr(t *testing.T) {
	svc := newTestService(DefaultConfig(), &fakeSource{}, nil)
	if _, err := svc.Snapshot(context.Background(), "hour"); err != ErrDisabled {
		t.Fatalf("want ErrDisabled, got %v", err)
	}
}

func TestSnapshot_LayoutOrderVisibilityAndStatus(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.AutoShowNewGroups = false
	// OpenAI 排在 Claude 前；分组 2 排在 1 前；分组 1 隐藏
	cfg.Layout = []PlatformLayout{
		{Platform: "openai", Visible: true, Groups: []GroupLayout{{GroupID: 3, Visible: true, DisplayName: "GPT 官方", Subtitle: "Responses"}}},
		{Platform: "anthropic", Visible: true, TabName: "Claude 系列", Groups: []GroupLayout{{GroupID: 2, Visible: true}, {GroupID: 1, Visible: false}}},
	}
	src := &fakeSource{
		cur:  map[int64]Signal{3: {Requests: 100, Errors: 30}}, // 错误率 30% → 0 分 → 异常
		hist: map[int64]Signal{2: {Requests: 100}},             // 分组 2 只有历史最后一格有数据
	}
	snap, err := newTestService(cfg, src, nil).Snapshot(context.Background(), "day")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Platforms) != 2 || snap.Platforms[0].Platform != "openai" || snap.Platforms[1].Name != "Claude 系列" {
		t.Fatalf("平台顺序/名称不对: %+v", snap.Platforms)
	}
	gpt := snap.Platforms[0].Groups[0]
	if gpt.Name != "GPT 官方" || gpt.Subtitle != "Responses" || gpt.Status != StatusDown || gpt.Score == nil || *gpt.Score != 0 {
		t.Fatalf("GPT 行不对: %+v", gpt)
	}
	claude := snap.Platforms[1].Groups
	if len(claude) != 1 || claude[0].GroupID != 2 || claude[0].Name != "Claude 特惠" {
		t.Fatalf("隐藏分组不应展示，展示名为空应回退原名: %+v", claude)
	}
	if claude[0].Status != StatusUnknown || claude[0].Score != nil {
		t.Fatalf("当前窗口无流量应为暂无数据: %+v", claude[0])
	}
	h := claude[0].History
	if len(h) != 30 || h[29].Status != StatusUp || h[0].Status != StatusUnknown {
		t.Fatalf("历史条不对: len=%d last=%s first=%s", len(h), h[29].Status, h[0].Status)
	}
}

func TestSnapshot_NewGroupFollowsAutoShow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.AutoShowNewGroups = true
	snap, err := newTestService(cfg, &fakeSource{}, nil).Snapshot(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Granularity != GranHour || len(snap.Platforms) != 2 || len(snap.Platforms[0].Groups) != 2 {
		t.Fatalf("自动展示新分组失败: %+v", snap)
	}
}

func TestSnapshot_FallbackOnlyForLackingGroups(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.AutoShowNewGroups = true
	judge := &fakeJudge{}
	src := &fakeSource{cur: map[int64]Signal{1: {Requests: 100}}}
	snap, err := newTestService(cfg, src, judge).Snapshot(context.Background(), "hour")
	if err != nil {
		t.Fatal(err)
	}
	if _, has1 := judge.got[1]; len(judge.got) != 2 || has1 {
		t.Fatalf("兜底只应收到样本不足的分组，got %v", judge.got)
	}
	if g := snap.Platforms[0].Groups[1]; g.GroupID != 2 || g.Status != StatusUp {
		t.Fatalf("兜底结论应计入当前状态: %+v", g)
	}
	if g := snap.Platforms[1].Groups[0]; g.Status != StatusUnknown {
		t.Fatalf("兜底无结论应为暂无数据: %+v", g)
	}
}

func TestSaveConfig_ValidatesAndCallsHook(t *testing.T) {
	var saved bool
	store := &fakeStore{cfg: DefaultConfig(), ver: 1}
	svc := NewService(Deps{Config: store, Catalog: fakeCatalog{}, Source: &fakeSource{}, OnSaved: func() { saved = true }})

	bad := DefaultConfig()
	bad.Rules.IgnoredCategories = []string{"nope"}
	if _, err := svc.SaveConfig(context.Background(), bad, 1); err == nil {
		t.Fatal("未知错误分类应校验失败")
	}
	if _, err := svc.SaveConfig(context.Background(), DefaultConfig(), 9); err != ErrVersionMismatch {
		t.Fatalf("版本冲突应报错, got %v", err)
	}
	if v, err := svc.SaveConfig(context.Background(), DefaultConfig(), 1); err != nil || v != 2 || !saved {
		t.Fatalf("保存失败 v=%d err=%v saved=%v", v, err, saved)
	}
}

func TestMergeLayout_DropsDeletedAndKeepsMovedGroup(t *testing.T) {
	saved := []PlatformLayout{
		{Platform: "openai", Visible: true, Groups: []GroupLayout{{GroupID: 1, Visible: true}, {GroupID: 99, Visible: true}}},
	}
	cat := []GroupInfo{{ID: 1, Platform: "anthropic"}, {ID: 2, Platform: "anthropic"}}
	m := MergeLayout(saved, cat, false)
	if len(m.Layout) != 2 || len(m.Layout[0].Groups) != 1 || m.Layout[0].Groups[0].GroupID != 1 {
		t.Fatalf("被移动的分组应留在 openai 下、已删除分组应剔除: %+v", m.Layout)
	}
	if m.Layout[1].Platform != "anthropic" || m.Layout[1].Groups[0].Visible || len(m.NewGroupIDs) != 1 || m.NewGroupIDs[0] != 2 {
		t.Fatalf("新分组应追加并默认隐藏: %+v %v", m.Layout, m.NewGroupIDs)
	}
}

func TestBuildTimeline(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 11, 4, 10, 30, 0, 0, loc) // 周三
	cases := []struct {
		g         Granularity
		lastStart time.Time
		count     int
	}{
		{GranHour, time.Date(2026, 11, 4, 10, 0, 0, 0, loc), 48},
		{GranDay, time.Date(2026, 11, 4, 0, 0, 0, 0, loc), 30},
		{GranWeek, time.Date(2026, 11, 2, 0, 0, 0, 0, loc), 12},
	}
	for _, c := range cases {
		tl := BuildTimeline(c.g, now, loc)
		if tl.Count != c.count || !tl.BucketStart(c.count-1).Equal(c.lastStart) || !tl.End.Equal(c.lastStart.Add(tl.Bucket)) {
			t.Errorf("%s: start=%v last=%v end=%v", c.g, tl.Start, tl.BucketStart(c.count-1), tl.End)
		}
	}
}

func TestCachedSource_HitsAndInvalidates(t *testing.T) {
	inner := &fakeSource{}
	c := NewCachedSource(inner, time.Minute)
	ctx := context.Background()
	start := testNow.Add(-time.Hour)
	for i := 0; i < 3; i++ {
		_, _ = c.Fetch(ctx, []int64{2, 1}, start, testNow, time.Hour)
	}
	_, _ = c.Fetch(ctx, []int64{1, 2}, start, testNow, time.Hour) // 顺序不同也命中
	if inner.calls != 1 {
		t.Fatalf("应只查询 1 次, got %d", inner.calls)
	}
	c.Invalidate()
	_, _ = c.Fetch(ctx, []int64{1, 2}, start, testNow, time.Hour)
	if inner.calls != 2 {
		t.Fatalf("失效后应重新查询, got %d", inner.calls)
	}
}

func TestHistPercentile(t *testing.T) {
	p, n := histPercentile(map[int64]int64{1000: 3, 3000: 3, 5000: 4}, 0.5)
	if p == nil || *p != 3000 || n != 10 {
		t.Fatalf("p50 want 3000/10, got %v/%d", p, n)
	}
	if p, n := histPercentile(nil, 0.5); p != nil || n != 0 {
		t.Fatal("空直方图应返回 nil")
	}
}
