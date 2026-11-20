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
	// 错误率 0 → 100；TTFT 6500ms 位于 3000~10000 中点 → 50；权重 0.75/0.25 → 75+12.5 = 87.5
	s := (DefaultScorer{}).Score(Signal{Requests: 100, TTFTP50Ms: i64(6500), TTFTSamples: 100}, r)
	if s == nil || !approx(*s, 87.5) {
		t.Fatalf("want 87.5, got %v", s)
	}
}

// 缓存不参与评分后，全部失败的分组不再有「白送」的分数
func TestScorer_AllErrorsIsZero(t *testing.T) {
	r := DefaultRules()
	s := (DefaultScorer{}).Score(Signal{Requests: 100, Errors: 100, TTFTP50Ms: i64(1000), TTFTSamples: 100}, r)
	// 错误维度 0 分 × 0.75 + TTFT 满分 × 0.25 = 25
	if s == nil || !approx(*s, 25) {
		t.Fatalf("want 25, got %v", s)
	}
	if got := (ThresholdClassifier{}).Classify(s, r); got != StatusDown {
		t.Fatalf("want down, got %s", got)
	}
}

func TestRules_NormalizeRescalesOldWeights(t *testing.T) {
	cases := []struct{ e, t, we, wt float64 }{
		{0.6, 0.2, 0.75, 0.25}, // 旧默认 60/20/20 → 75/25
		{0.5, 0.3, 0.625, 0.375},
		{0, 0, 0.75, 0.25},   // 旧配置全部权重给了缓存 → 回到默认
		{0.7, 0.3, 0.7, 0.3}, // 已经是新口径，不动
	}
	for _, c := range cases {
		r := DefaultRules()
		r.ErrorWeight, r.TTFTWeight = c.e, c.t
		r.Normalize()
		if !approx(r.ErrorWeight, c.we) || !approx(r.TTFTWeight, c.wt) {
			t.Errorf("%v/%v → want %v/%v, got %v/%v", c.e, c.t, c.we, c.wt, r.ErrorWeight, r.TTFTWeight)
		}
		if err := r.Validate(); err != nil {
			t.Errorf("换算后应通过校验: %v", err)
		}
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
