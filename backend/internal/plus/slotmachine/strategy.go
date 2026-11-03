// Package slotmachine — [plus] 随机抽奖策略
package slotmachine

import (
	"crypto/rand"
	"encoding/binary"
)

// RNG 随机源接口，便于测试注入确定性实现
type RNG interface {
	Float64() float64
}

type cryptoRNG struct{}

func (cryptoRNG) Float64() float64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("slotmachine: crypto/rand unavailable: " + err.Error())
	}
	return float64(binary.BigEndian.Uint64(b[:])>>11) / (1 << 53)
}

// DefaultRNG 生产环境使用 crypto/rand
var DefaultRNG RNG = cryptoRNG{}

// PickWeighted 按权重抽取下标；总权重为 0 时返回 0
func PickWeighted(weights []float64, rng RNG) int {
	total := 0.0
	for _, w := range weights {
		if w > 0 {
			total += w
		}
	}
	if total <= 0 {
		return 0
	}
	r := rng.Float64() * total
	acc := 0.0
	last := 0
	for i, w := range weights {
		if w <= 0 {
			continue
		}
		acc += w
		last = i
		if r < acc {
			return i
		}
	}
	return last
}

// PickBP 按万分比概率抽取下标（概率合计应为 FullBP）
func PickBP(probs []int, rng RNG) int {
	w := make([]float64, len(probs))
	for i, p := range probs {
		w[i] = float64(p)
	}
	return PickWeighted(w, rng)
}
