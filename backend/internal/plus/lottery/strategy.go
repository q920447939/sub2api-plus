package lottery

import (
	"crypto/rand"
	"encoding/binary"
	"math"
)

// RNG 抽象随机源，便于测试注入确定性实现。Float64 返回 [0, 1)。
type RNG interface {
	Float64() float64
}

// cryptoRNG 基于 crypto/rand，避免可预测的开奖结果。
type cryptoRNG struct{}

func (cryptoRNG) Float64() float64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("lottery: crypto/rand unavailable: " + err.Error())
	}
	return float64(binary.BigEndian.Uint64(b[:])>>11) / (1 << 53)
}

// DefaultRNG 生产环境使用的随机源
var DefaultRNG RNG = cryptoRNG{}

// DrawInput 一次开奖所需的上下文，全部由调用方在事务内读取后传入。
type DrawInput struct {
	CodeValue        float64 // 卡片所属卡密的面值
	UserAwardedToday float64 // 该用户今日已中奖金额
	BudgetUsedToday  float64 // 今日全站已发放金额
}

// DrawOutcome 开奖结果
type DrawOutcome struct {
	Amount    float64
	TierIndex int    // 命中的奖档下标，-1 表示保底
	Degraded  string // 降级原因，见 Degraded* 常量
}

// PrizeStrategy 奖金策略接口。新增策略（固定奖品、按比例等）只需实现该接口。
type PrizeStrategy interface {
	Draw(cfg *Config, in DrawInput, rng RNG) DrawOutcome
}

// WeightedRangeStrategy 区间权重策略：按权重命中奖档，在档内均匀取值，再叠加风控兜底。
type WeightedRangeStrategy struct{}

func (WeightedRangeStrategy) Draw(cfg *Config, in DrawInput, rng RNG) DrawOutcome {
	rc := cfg.RiskControl
	floor := roundCents(rc.FloorAmount)
	fallback := func(reason string) DrawOutcome {
		return DrawOutcome{Amount: floor, TierIndex: -1, Degraded: reason}
	}

	// 1. 单笔比例上限：排除最低值已超出上限的奖档
	limit := in.CodeValue * rc.SingleOrderPct
	idx, ok := pickTier(cfg.PrizePoolReal.Tiers, limit, rng)
	if !ok {
		return fallback(DegradedPoolFiltered)
	}
	tier := cfg.PrizePoolReal.Tiers[idx]
	upper := math.Min(tier.Max, limit)
	amount := roundCents(tier.Min + rng.Float64()*(upper-tier.Min))
	if amount < floor {
		amount = floor
	}
	out := DrawOutcome{Amount: amount, TierIndex: idx}

	// 2. 单用户每日上限
	if rc.UserDailyMax > 0 && in.UserAwardedToday+amount > rc.UserDailyMax {
		return fallback(DegradedUserDaily)
	}
	// 3. 每日总预算
	if rc.DailyBudget > 0 && in.BudgetUsedToday+amount > rc.DailyBudget {
		return fallback(DegradedDailyBudget)
	}
	return out
}

// pickTier 在 Min <= limit 的奖档中按权重随机选一个；权重为 0 的档永不选中
func pickTier(tiers []PrizeTier, limit float64, rng RNG) (int, bool) {
	total := 0
	for _, t := range tiers {
		if t.Min <= limit {
			total += t.Weight
		}
	}
	if total == 0 {
		return 0, false
	}
	r := rng.Float64() * float64(total)
	acc := 0.0
	last := -1
	for i, t := range tiers {
		if t.Min > limit || t.Weight <= 0 {
			continue
		}
		last = i
		acc += float64(t.Weight)
		if r < acc {
			return i, true
		}
	}
	return last, true
}

// SampleVirtualAmount 从奖池 B 采样虚拟记录金额（不受风控约束）
func SampleVirtualAmount(pool PrizePool, rng RNG) float64 {
	idx, ok := pickTier(pool.Tiers, math.Inf(1), rng)
	if !ok {
		return 0
	}
	t := pool.Tiers[idx]
	return roundCents(t.Min + rng.Float64()*(t.Max-t.Min))
}

func roundCents(v float64) float64 {
	return math.Round(v*100) / 100
}
