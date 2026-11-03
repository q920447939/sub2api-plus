//go:build unit

package slotmachine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/plus/checkin"
)

type stubFacts struct {
	now      time.Time
	recharge float64
	today    int
	last     time.Time
	hasLast  bool
	streak   int
}

func (f *stubFacts) Now() time.Time { return f.now }
func (f *stubFacts) RecentRecharge(context.Context, time.Time) (float64, error) {
	return f.recharge, nil
}
func (f *stubFacts) TodaySpinCount(context.Context) (int, error) { return f.today, nil }
func (f *stubFacts) LastSpinAt(context.Context) (time.Time, bool, error) {
	return f.last, f.hasLast, nil
}
func (f *stubFacts) CheckinStreak(context.Context) (int, error) { return f.streak, nil }

func rulesOf(t *testing.T, cfgs ...RuleConfig) []Rule {
	t.Helper()
	chain, err := BuildRules(cfgs)
	if err != nil {
		t.Fatal(err)
	}
	return chain
}

func TestRuleChainStopsAtFirstFailure(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	chain := rulesOf(t,
		RuleConfig{Type: RuleRecharge, Enabled: true, Params: map[string]any{"window_days": 30.0, "min_amount": 5.0}},
		RuleConfig{Type: RuleDailyLimit, Enabled: true, Params: map[string]any{"max": 3.0}},
		RuleConfig{Type: RuleCooldown, Enabled: true, Params: map[string]any{"seconds": 10.0}},
	)
	cases := []struct {
		name string
		f    stubFacts
		code string
	}{
		{"all pass", stubFacts{now: now, recharge: 5}, ""},
		{"recharge short", stubFacts{now: now, recharge: 4.99, today: 9}, "recharge_required"},
		{"daily limit", stubFacts{now: now, recharge: 10, today: 3}, "daily_limit"},
		{"cooldown", stubFacts{now: now, recharge: 10, last: now.Add(-3 * time.Second), hasLast: true}, "cooldown"},
		{"cooldown elapsed", stubFacts{now: now, recharge: 10, last: now.Add(-10 * time.Second), hasLast: true}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckRules(context.Background(), chain, &tc.f)
			if tc.code == "" {
				if err != nil {
					t.Fatalf("want pass, got %v", err)
				}
				return
			}
			re, ok := AsRuleError(err)
			if !ok || re.Code != tc.code {
				t.Fatalf("want %s, got %v", tc.code, err)
			}
		})
	}
}

func TestCooldownRetryAfter(t *testing.T) {
	now := time.Now()
	chain := rulesOf(t, RuleConfig{Type: RuleCooldown, Enabled: true, Params: map[string]any{"seconds": 10.0}})
	err := CheckRules(context.Background(), chain, &stubFacts{now: now, last: now.Add(-4 * time.Second), hasLast: true})
	re, _ := AsRuleError(err)
	if re == nil || re.RetryAfter != 6*time.Second {
		t.Fatalf("retry after = %v", re)
	}
}

func TestDisabledRuleSkipped(t *testing.T) {
	chain := rulesOf(t, RuleConfig{Type: RuleDailyLimit, Enabled: false, Params: map[string]any{"max": 1.0}})
	if len(chain) != 0 {
		t.Fatalf("disabled rule built: %d", len(chain))
	}
}

func TestBuildRuleValidatesParams(t *testing.T) {
	if _, err := BuildRule(RuleConfig{Type: "nope"}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("unknown type: %v", err)
	}
	if _, err := BuildRule(RuleConfig{Type: RuleDailyLimit, Params: map[string]any{"max": 0.0}}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("out of range: %v", err)
	}
}

func TestPickWeighted(t *testing.T) {
	w := []float64{0, 1, 0, 3}
	for _, tc := range []struct {
		r    float64
		want int
	}{{0, 1}, {0.24, 1}, {0.25, 3}, {0.9999, 3}} {
		if got := PickWeighted(w, fixedRNG(tc.r)); got != tc.want {
			t.Fatalf("r=%v got %d want %d", tc.r, got, tc.want)
		}
	}
}

type stubBoost struct{ sb checkin.StreakBoost }

func (s stubBoost) StreakBoostConfig(context.Context) (checkin.StreakBoost, error) { return s.sb, nil }

func sum(p []int) int {
	t := 0
	for _, v := range p {
		t += v
	}
	return t
}

func TestDefaultConfigValid(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	// 默认奖池：每摇一次两个罐的期望支出都不超过注入
	small, jackpot := cfg.PotExpectation(cfg.Probs())
	inS, inJ := cfg.Pot.Inflow(cfg.CostPerSpin)
	if small > float64(inS) || jackpot > float64(inJ)+1e-9 {
		t.Fatalf("default pool drains pots: small %.2f/%d jackpot %.2f/%d", small, inS, jackpot, inJ)
	}
	if cfg.Pot.BurnPct() != 35 {
		t.Fatalf("burn pct want 35, got %d", cfg.Pot.BurnPct())
	}
}

func TestValidateRejectsBadPools(t *testing.T) {
	cases := map[string]func(c *Config){
		"wrong tier count": func(c *Config) { c.Tiers = c.Tiers[:7] },
		"sum not 100%":     func(c *Config) { c.Tiers[1].ProbBP++ },
		"two none tiers":   func(c *Config) { c.Tiers[1].PrizeType = PrizeTypeNone },
		"no pot":           func(c *Config) { c.Tiers[1].Pot = "" },
		"bad prize":        func(c *Config) { c.Tiers[1].PrizePoints = 0 },
		"pot over 100":     func(c *Config) { c.Pot.SmallPct = 96 },
		"virtual on none":  func(c *Config) { c.Atmosphere.VirtualProbs[3], c.Atmosphere.VirtualProbs[1] = 100, 1400 },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Atmosphere.VirtualProbs = append([]int(nil), cfg.Atmosphere.VirtualProbs...)
			mut(&cfg)
			if err := cfg.Validate(); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("want ErrInvalidConfig, got %v", err)
			}
		})
	}
}

func TestStreakBoostShiftsFromNone(t *testing.T) {
	cfg := DefaultConfig()
	none := cfg.NoneIndex()
	src := stubBoost{checkin.StreakBoost{Enabled: true, MinStreak: 3, Tiers: []checkin.TierBoost{{Tier: 4, AddBP: 100}, {Tier: 6, AddBP: 200}}}}
	mods := []Modifier{streakBoostMod{src: src}}

	probs, boosts, err := ApplyModifiers(context.Background(), mods, &stubFacts{streak: 2}, &cfg)
	if err != nil || len(boosts) != 0 || probs[4] != cfg.Tiers[4].ProbBP {
		t.Fatalf("streak 2 should not boost: %v %v", boosts, err)
	}
	probs, boosts, err = ApplyModifiers(context.Background(), mods, &stubFacts{streak: 3}, &cfg)
	if err != nil || len(boosts) != 1 {
		t.Fatalf("streak 3 should boost: %v %v", boosts, err)
	}
	if probs[4] != cfg.Tiers[4].ProbBP+100 || probs[6] != cfg.Tiers[6].ProbBP+200 {
		t.Fatalf("boosted probs wrong: %v", probs)
	}
	if probs[none] != cfg.Tiers[none].ProbBP-300 || sum(probs) != FullBP {
		t.Fatalf("none should give up 300bp and total stay 100%%: %v", probs)
	}
}

func TestStreakBoostCappedByNone(t *testing.T) {
	cfg := DefaultConfig()
	none := cfg.NoneIndex()
	src := stubBoost{checkin.StreakBoost{Enabled: true, MinStreak: 1, Tiers: []checkin.TierBoost{{Tier: 4, AddBP: 5000}, {Tier: 6, AddBP: 5000}}}}
	probs, _, err := ApplyModifiers(context.Background(), []Modifier{streakBoostMod{src: src}}, &stubFacts{streak: 9}, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if probs[none] < 0 || sum(probs) != FullBP {
		t.Fatalf("boost must not exceed none tier: %v", probs)
	}
}

func TestExcludeByPotsMovesToNone(t *testing.T) {
	cfg := DefaultConfig()
	none := cfg.NoneIndex()
	// 小奖罐只剩 150：200 积分、500 积分、$1(=100积分 可付) → 200/500 被排除；大奖罐 0 → $20 被排除
	probs, excluded := ExcludeByPots(&cfg, cfg.Probs(), map[string]int64{PotSmall: 150, PotJackpot: 0})
	want := map[int]bool{0: true, 4: true, 7: true}
	if len(excluded) != len(want) {
		t.Fatalf("excluded %v", excluded)
	}
	for _, i := range excluded {
		if !want[i] || probs[i] != 0 {
			t.Fatalf("tier %d unexpectedly excluded or still has prob: %v", i, probs)
		}
	}
	moved := cfg.Tiers[0].ProbBP + cfg.Tiers[4].ProbBP + cfg.Tiers[7].ProbBP
	if probs[none] != cfg.Tiers[none].ProbBP+moved || sum(probs) != FullBP {
		t.Fatalf("excluded prob must go to none: %v", probs)
	}
}

func TestPrizeValuePoints(t *testing.T) {
	cfg := DefaultConfig()
	if v := cfg.ValuePoints(cfg.Tiers[0]); v != 2000 {
		t.Fatalf("$20 at 100/$ should be 2000 points, got %d", v)
	}
	if v := cfg.ValuePoints(cfg.Tiers[cfg.NoneIndex()]); v != 0 {
		t.Fatalf("none value should be 0, got %d", v)
	}
}

func TestPickBP(t *testing.T) {
	if got := PickBP([]int{0, 10000, 0}, fixedRNG(0.99)); got != 1 {
		t.Fatalf("want 1, got %d", got)
	}
	if got := PickBP([]int{5000, 0, 5000}, fixedRNG(0.6)); got != 2 {
		t.Fatalf("want 2, got %d", got)
	}
}

type fixedRNG float64

func (f fixedRNG) Float64() float64 { return float64(f) }
