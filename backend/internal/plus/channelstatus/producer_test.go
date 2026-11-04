//go:build unit

package channelstatus

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type stubStore struct {
	cfg   Config
	loads int
}

func (s *stubStore) Load(context.Context) (Config, int64, error) {
	s.loads++
	return s.cfg, 1, nil
}
func (s *stubStore) Save(context.Context, Config, int64) (int64, error) { return 1, nil }

func TestPlusRuntime_FollowsPlusConfigOnly(t *testing.T) {
	store := &stubStore{cfg: Config{Enabled: false}}
	rt := plusRuntime{r: &enabledReader{store: store, ttl: time.Hour}}

	if got := rt.GetChannelMonitorRuntime(context.Background()); got.PassiveAggregationAllowed() {
		t.Fatal("新渠道状态关闭时不应聚合")
	}

	store.cfg.Enabled = true
	rt.r.fetched = time.Time{} // 让缓存过期
	got := rt.GetChannelMonitorRuntime(context.Background())
	if !got.PassiveAggregationAllowed() || got.Mode != service.ChannelMonitorModeV2 {
		t.Fatalf("新渠道状态开启时应允许聚合，got %+v", got)
	}
	if got.ActiveProbesAllowed() {
		t.Fatal("绝不能放行上游 V1 主动探测")
	}
}

func TestEnabledReader_Caches(t *testing.T) {
	store := &stubStore{cfg: Config{Enabled: true}}
	r := &enabledReader{store: store, ttl: time.Hour}
	for i := 0; i < 5; i++ {
		r.enabled(context.Background())
	}
	if store.loads != 1 {
		t.Fatalf("TTL 内只应读一次配置，got %d", store.loads)
	}
}

func TestRules_ForHistoryUsesHistoryMinSample(t *testing.T) {
	r := DefaultRules()
	sig := Signal{Requests: 12} // 不够 50，但够 10
	if (DefaultScorer{}).Score(sig, r) != nil {
		t.Fatal("当前状态门槛 50，12 次请求不应出分")
	}
	if s := (DefaultScorer{}).Score(sig, r.ForHistory()); s == nil || !approx(*s, 100) {
		t.Fatalf("历史条门槛 10，应出分 100，got %v", s)
	}
}

func TestConfig_OldDataGetsHistoryMinSampleDefault(t *testing.T) {
	cfg := DefaultConfig()
	// 模拟升级前保存的配置：rules 里没有 history_min_sample
	old := []byte(`{"enabled":true,"refresh_seconds":60,"judge_window_minutes":15,"default_granularity":"hour",
		"rules":{"up_min":80,"warn_min":50,"min_sample":50,"warning_error_rate":0.05,"critical_error_rate":0.2,
		"target_ttft_ms":3000,"critical_ttft_ms":10000,"error_weight":0.6,"ttft_weight":0.2,"cache_weight":0.2}}`)
	if err := json.Unmarshal(old, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Rules.HistoryMinSample != 10 {
		t.Fatalf("want 10, got %d", cfg.Rules.HistoryMinSample)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("旧配置应仍可通过校验: %v", err)
	}
}

func TestPlanSegments_WeekSplitsAtUTCMidnight(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	tl := BuildTimeline(GranWeek, time.Date(2026, 11, 4, 10, 30, 0, 0, loc), loc)
	segs := planSegments(tl.Start, tl.End, tl.Bucket)
	if len(segs) != 2 || segs[0].tier.rollupSeconds != 86400 || segs[1].tier.rollupSeconds != 3600 {
		t.Fatalf("周视图应拆成 1 天档 + 小时档: %+v", segs)
	}
	split := segs[0].end
	if !split.Equal(segs[1].start) || !split.Equal(split.UTC().Truncate(24*time.Hour)) || tl.End.Sub(split) > 30*24*time.Hour {
		t.Fatalf("分界点应为 30 天内的 UTC 零点且两段相接: %v", split)
	}
	if s := planSegments(tl.Start, tl.End, time.Hour); len(s) != 1 || s[0].tier.rollupSeconds != 3600 {
		t.Fatalf("小时视图不应拆分: %+v", s)
	}
}
