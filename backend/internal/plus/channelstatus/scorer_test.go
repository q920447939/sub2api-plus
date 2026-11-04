//go:build unit

package channelstatus

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func i64(v int64) *int64 { return &v }

func TestScorer_BelowMinSampleHasNoScore(t *testing.T) {
	r := DefaultRules()
	if got := (DefaultScorer{}).Score(Signal{Requests: r.MinSample - 1}, r); got != nil {
		t.Fatalf("样本不足应无分数，got %v", *got)
	}
}

func TestScorer_ErrorRateOnly(t *testing.T) {
	r := DefaultRules()
	// 错误率 10%，严重阈值 20% → 错误维度 50 分；无 TTFT/缓存维度，按权重归一化后仍是 50
	s := (DefaultScorer{}).Score(Signal{Requests: 100, Errors: 10}, r)
	if s == nil || !approx(*s, 50) {
		t.Fatalf("want 50, got %v", s)
	}
}

func TestScorer_ZeroErrorsIsFull(t *testing.T) {
	s := (DefaultScorer{}).Score(Signal{Requests: 100}, DefaultRules())
	if s == nil || !approx(*s, 100) {
		t.Fatalf("want 100, got %v", s)
	}
}

func TestScorer_TTFTWeighted(t *testing.T) {
	r := DefaultRules()
	// 错误率 0 → 100；TTFT 6500ms 位于 3000~10000 中点 → 50；权重 0.6/0.2 → (60+10)/0.8 = 87.5
	s := (DefaultScorer{}).Score(Signal{Requests: 100, TTFTP50Ms: i64(6500), TTFTSamples: 100}, r)
	if s == nil || !approx(*s, 87.5) {
		t.Fatalf("want 87.5, got %v", s)
	}
}

func TestScorer_CacheNotPenalizedByDefault(t *testing.T) {
	r := DefaultRules() // 缓存阈值 0/0
	s := (DefaultScorer{}).Score(Signal{Requests: 100, CacheRate: 0, CacheDenom: 100}, r)
	if s == nil || !approx(*s, 100) {
		t.Fatalf("默认不惩罚缓存，want 100, got %v", s)
	}
}

func TestClassifier(t *testing.T) {
	r := DefaultRules()
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		score *float64
		want  Status
	}{
		{nil, StatusUnknown},
		{f(100), StatusUp},
		{f(80), StatusUp},
		{f(79.9), StatusWarn},
		{f(50), StatusWarn},
		{f(49.9), StatusDown},
		{f(0), StatusDown},
	}
	for _, c := range cases {
		if got := (ThresholdClassifier{}).Classify(c.score, r); got != c.want {
			t.Errorf("score=%v want %s got %s", c.score, c.want, got)
		}
	}
}
