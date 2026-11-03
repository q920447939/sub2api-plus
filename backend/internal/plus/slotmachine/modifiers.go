// Package slotmachine — [plus] 概率修正器：根据用户状态调整本次摇奖的奖档概率
//
// 修正器的参数不在本包存储，由各自的业务配置提供（如连签加成配置在签到里）。
// 新增修正器只需实现 Modifier 并在 NewService 里加入链。所有修正器都只能
// "从谢谢惠顾挪概率给中奖档"，保证合计始终是 100%。
package slotmachine

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/plus/checkin"
)

const ModifierStreakBoost = "streak_boost"

// Boost 生效中的加成（供前端展示）
type Boost struct {
	Type  string      `json:"type"`
	Text  string      `json:"text"`
	Tiers map[int]int `json:"tiers"` // 奖档下标 → 实际增加的万分比
}

// Modifier 一个概率修正器
type Modifier interface {
	Type() string
	// Apply 返回调整后的概率；未生效时返回原概率与 nil
	Apply(ctx context.Context, f Facts, cfg *Config, probs []int) ([]int, *Boost, error)
}

// ApplyModifiers 依次应用修正器链
func ApplyModifiers(ctx context.Context, mods []Modifier, f Facts, cfg *Config) ([]int, []Boost, error) {
	probs := cfg.Probs()
	boosts := []Boost{}
	for _, m := range mods {
		next, b, err := m.Apply(ctx, f, cfg, probs)
		if err != nil {
			return nil, nil, fmt.Errorf("modifier %s: %w", m.Type(), err)
		}
		probs = next
		if b != nil {
			boosts = append(boosts, *b)
		}
	}
	return probs, boosts, nil
}

// shiftFromNone 把 adds 指定的万分比从谢谢惠顾档挪到对应档；谢谢惠顾不够时按比例缩减
func shiftFromNone(cfg *Config, probs []int, adds map[int]int) ([]int, map[int]int) {
	none := cfg.NoneIndex()
	want := 0
	for i, a := range adds {
		if i == none || i < 0 || i >= len(probs) || a <= 0 {
			delete(adds, i)
			continue
		}
		want += a
	}
	if none < 0 || want == 0 || probs[none] == 0 {
		return probs, nil
	}
	out := append([]int(nil), probs...)
	applied := map[int]int{}
	budget := out[none]
	for i, a := range adds {
		if want > budget {
			a = a * budget / want
		}
		if a <= 0 {
			continue
		}
		out[i] += a
		out[none] -= a
		applied[i] = a
	}
	if len(applied) == 0 {
		return probs, nil
	}
	return out, applied
}

// ─── 连签加成 ──────────────────────────────────────────────────────────

// StreakBoostSource 连签加成配置来源（签到模块）
type StreakBoostSource interface {
	StreakBoostConfig(ctx context.Context) (checkin.StreakBoost, error)
}

type streakBoostMod struct{ src StreakBoostSource }

func (m streakBoostMod) Type() string { return ModifierStreakBoost }

func (m streakBoostMod) Apply(ctx context.Context, f Facts, cfg *Config, probs []int) ([]int, *Boost, error) {
	sb, err := m.src.StreakBoostConfig(ctx)
	if err != nil {
		return probs, nil, err
	}
	if !sb.Enabled || len(sb.Tiers) == 0 {
		return probs, nil, nil
	}
	streak, err := f.CheckinStreak(ctx)
	if err != nil {
		return probs, nil, err
	}
	if streak < sb.MinStreak {
		return probs, nil, nil
	}
	adds := map[int]int{}
	for _, t := range sb.Tiers {
		adds[t.Tier] = t.AddBP
	}
	out, applied := shiftFromNone(cfg, probs, adds)
	if applied == nil {
		return probs, nil, nil
	}
	return out, &Boost{
		Type:  ModifierStreakBoost,
		Text:  fmt.Sprintf("🔥 连签 %d 天加成：%d 个奖档中奖概率提升", streak, len(applied)),
		Tiers: applied,
	}, nil
}

// previewFacts 管理端预览"加成全部生效"时使用
type previewFacts struct{ Facts }

func (previewFacts) CheckinStreak(context.Context) (int, error) { return 1 << 20, nil }
