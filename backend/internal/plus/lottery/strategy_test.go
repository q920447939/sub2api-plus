package lottery

import (
	"errors"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// seqRNG 按顺序返回预设值，用于确定性测试
type seqRNG struct {
	vals []float64
	i    int
}

func (s *seqRNG) Float64() float64 {
	v := s.vals[s.i%len(s.vals)]
	s.i++
	return v
}

// mathRNG 可复现的伪随机源，用于分布测试
type mathRNG struct{ r *rand.Rand }

func (m mathRNG) Float64() float64 { return m.r.Float64() }

func TestWeightedRange_Distribution(t *testing.T) {
	cfg := DefaultConfig()
	rng := mathRNG{rand.New(rand.NewSource(42))}
	const n = 100000
	counts := make([]int, len(cfg.PrizePoolReal.Tiers))
	for i := 0; i < n; i++ {
		out := WeightedRangeStrategy{}.Draw(&cfg, DrawInput{CodeValue: 1000}, rng)
		if out.TierIndex < 0 {
			t.Fatalf("unexpected degrade: %s", out.Degraded)
		}
		tier := cfg.PrizePoolReal.Tiers[out.TierIndex]
		if out.Amount < tier.Min || out.Amount > tier.Max {
			t.Fatalf("amount %v out of tier [%v,%v]", out.Amount, tier.Min, tier.Max)
		}
		counts[out.TierIndex]++
	}
	for i, tier := range cfg.PrizePoolReal.Tiers {
		got := float64(counts[i]) / n * 100
		if math.Abs(got-float64(tier.Weight)) > 1 {
			t.Errorf("tier %d: got %.2f%%, want ~%d%%", i, got, tier.Weight)
		}
	}
}

func TestWeightedRange_SingleOrderPctFiltersTiers(t *testing.T) {
	cfg := DefaultConfig() // SingleOrderPct = 0.5
	rng := mathRNG{rand.New(rand.NewSource(1))}
	// 订单 10 元 → 上限 5 元：5~10 档的 Min=5 仍可命中，但金额被截到 5
	for i := 0; i < 5000; i++ {
		out := WeightedRangeStrategy{}.Draw(&cfg, DrawInput{CodeValue: 10}, rng)
		if out.Amount > 5 {
			t.Fatalf("amount %v exceeds 50%% of code value", out.Amount)
		}
	}
	// 订单 1 元 → 上限 0.5 元：只剩 0.1~0.2 和 0.5~1（被截到 0.5）
	for i := 0; i < 5000; i++ {
		out := WeightedRangeStrategy{}.Draw(&cfg, DrawInput{CodeValue: 1}, rng)
		if out.Amount > 0.5 || out.TierIndex == 2 {
			t.Fatalf("got %+v for 1 元 code", out)
		}
	}
}

func TestWeightedRange_PoolFilteredFallsBackToFloor(t *testing.T) {
	cfg := DefaultConfig()
	out := WeightedRangeStrategy{}.Draw(&cfg, DrawInput{CodeValue: 0.1}, &seqRNG{vals: []float64{0.5}})
	if out.Degraded != DegradedPoolFiltered || out.Amount != 0.1 || out.TierIndex != -1 {
		t.Fatalf("got %+v", out)
	}
}

func TestWeightedRange_RiskLimits(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RiskControl.UserDailyMax = 10
	cfg.RiskControl.DailyBudget = 100
	// r=0.99 命中最高档 5~10，第二个值决定档内金额
	hi := func() RNG { return &seqRNG{vals: []float64{0.99, 0.5}} }

	out := WeightedRangeStrategy{}.Draw(&cfg, DrawInput{CodeValue: 1000, UserAwardedToday: 8}, hi())
	if out.Degraded != DegradedUserDaily || out.Amount != cfg.RiskControl.FloorAmount {
		t.Fatalf("user daily: got %+v", out)
	}
	out = WeightedRangeStrategy{}.Draw(&cfg, DrawInput{CodeValue: 1000, BudgetUsedToday: 95}, hi())
	if out.Degraded != DegradedDailyBudget {
		t.Fatalf("daily budget: got %+v", out)
	}
	out = WeightedRangeStrategy{}.Draw(&cfg, DrawInput{CodeValue: 1000}, hi())
	if out.Degraded != "" || out.Amount != 7.5 {
		t.Fatalf("no limit: got %+v", out)
	}
}

func TestSampleVirtualAmount_WithinPool(t *testing.T) {
	cfg := DefaultConfig()
	rng := mathRNG{rand.New(rand.NewSource(7))}
	max := cfg.PrizePoolVirtual.MaxAmount()
	for i := 0; i < 10000; i++ {
		v := SampleVirtualAmount(cfg.PrizePoolVirtual, rng)
		if v < 0.5 || v > max {
			t.Fatalf("virtual amount %v out of range", v)
		}
	}
}

func TestChancesForAmount(t *testing.T) {
	cfg := DefaultConfig()
	cases := map[float64]int{0: 0, 9.99: 0, 10: 1, 49.99: 1, 50: 3, 99: 3, 100: 6, 10000: 6}
	for amt, want := range cases {
		if got := cfg.ChancesForAmount(amt); got != want {
			t.Errorf("ChancesForAmount(%v) = %d, want %d", amt, got, want)
		}
	}
}

func TestConfigValidate(t *testing.T) {
	ok := DefaultConfig()
	if err := ok.Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
	cases := []struct {
		name string
		mut  func(*Config)
		want error
	}{
		{"empty tiers", func(c *Config) { c.ChanceTiers = nil }, ErrInvalidChanceTiers},
		{"unsorted tiers", func(c *Config) { c.ChanceTiers[1].Threshold = 5 }, ErrInvalidChanceTiers},
		{"bad prize tier", func(c *Config) { c.PrizePoolReal.Tiers[0].Max = 0.01 }, ErrInvalidPrizePool},
		{"negative weight", func(c *Config) { c.PrizePoolVirtual.Tiers[0].Weight = -1 }, ErrInvalidPrizePool},
		{"all zero weight", func(c *Config) {
			for i := range c.PrizePoolReal.Tiers {
				c.PrizePoolReal.Tiers[i].Weight = 0
			}
		}, ErrInvalidPrizePool},
		{"virtual over promo", func(c *Config) { c.PromoMaxAmount = 50 }, ErrPoolExceedsPromo},
		{"zero floor", func(c *Config) { c.RiskControl.FloorAmount = 0 }, ErrInvalidRiskControl},
		{"pct > 1", func(c *Config) { c.RiskControl.SingleOrderPct = 1.5 }, ErrInvalidRiskControl},
		{"daily target", func(c *Config) { c.VirtualFeed.DailyTarget = 2000 }, ErrInvalidVirtualFeed},
	}
	for _, tc := range cases {
		c := DefaultConfig()
		tc.mut(&c)
		if err := c.Validate(); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestMaskEmail(t *testing.T) {
	cases := map[string]string{
		"alice@gmail.com": "a***@g***.com",
		"b@qq.com":        "b***@q***.com",
		"":                "***",
	}
	for in, want := range cases {
		if got := MaskEmail(in); got != want {
			t.Errorf("MaskEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVirtualMinuteProb_MatchesDailyTarget(t *testing.T) {
	_ = timezone.Init("Asia/Shanghai")
	day := time.Date(2026, 10, 17, 0, 0, 0, 0, timezone.Location())
	f := dayFactor(DayOf(day).Key)
	if f < 0.7 || f > 1.3 {
		t.Fatalf("day factor %v out of [0.7,1.3]", f)
	}
	if dayFactor(DayOf(day).Key) != f {
		t.Fatal("day factor must be deterministic")
	}
	expected := 0.0
	for m := 0; m < 1440; m++ {
		expected += virtualMinuteProb(120, day.Add(time.Duration(m)*time.Minute))
	}
	if math.Abs(expected-120*f) > 0.5 {
		t.Fatalf("expected daily count %.2f, want %.2f", expected, 120*f)
	}
	night := virtualMinuteProb(120, day.Add(3*time.Hour))
	peak := virtualMinuteProb(120, day.Add(21*time.Hour))
	if night*5 > peak {
		t.Fatalf("night %.4f should be far below peak %.4f", night, peak)
	}
}

func TestPickTier_SkipsZeroWeight(t *testing.T) {
	tiers := []PrizeTier{{Min: 1, Max: 2, Weight: 0}, {Min: 3, Max: 4, Weight: 10}, {Min: 5, Max: 6, Weight: 0}}
	rng := mathRNG{rand.New(rand.NewSource(7))}
	for i := 0; i < 2000; i++ {
		if idx, ok := pickTier(tiers, math.Inf(1), rng); !ok || idx != 1 {
			t.Fatalf("pickTier = %d,%v; want 1,true", idx, ok)
		}
	}
	// 可选档全部为 0 权重 → 不可选，交由保底
	if _, ok := pickTier(tiers, 2, rng); ok {
		t.Fatal("zero-weight-only candidates should not be pickable")
	}
}

func TestMaxAmount_IgnoresZeroWeight(t *testing.T) {
	p := PrizePool{Tiers: []PrizeTier{{Min: 1, Max: 2, Weight: 5}, {Min: 50, Max: 100, Weight: 0}}}
	if got := p.MaxAmount(); got != 2 {
		t.Fatalf("MaxAmount = %v, want 2", got)
	}
}

func TestVirtualName_RespectsWhitelist(t *testing.T) {
	s := &Service{rng: mathRNG{rand.New(rand.NewSource(1))}}
	for i := 0; i < 500; i++ {
		name := s.virtualName([]string{"@qq.com", "*.edu.cn"})
		if !strings.HasSuffix(name, "@q***.com") && !strings.HasSuffix(name, "@e***.cn") {
			t.Fatalf("name %q outside whitelist", name)
		}
	}
	for i := 0; i < 200; i++ {
		name := s.virtualName(nil)
		if strings.Count(name, "***") != 2 || !strings.Contains(name, "@") {
			t.Fatalf("bad default name %q", name)
		}
	}
}
