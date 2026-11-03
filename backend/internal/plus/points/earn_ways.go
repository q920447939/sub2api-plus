// Package points — [plus] 积分获取方式注册表
//
// 各积分来源（签到、兑换卡密送积分……）在启动时注册一个 EarnWayProvider，
// 用户积分不足时由消耗方（如幸运九宫格）调用 EarnWays 列出当前可用的获取方式。
// 新增积分来源只需注册一个 provider，前端按 action 渲染按钮，无需改动。
package points

import (
	"context"
	"log"
	"sort"
	"sync"
)

// EarnWay 一种当前可用的积分获取方式
type EarnWay struct {
	Key   string `json:"key"`   // 来源标识，如 checkin / redeem_bonus
	Label string `json:"label"` // 按钮文案，如"去签到"
	Hint  string `json:"hint,omitempty"`
	// Action 前端动作：tab:<name> 切到积分乐园页内标签；route:<path> 跳转路由
	Action string `json:"action"`
	Order  int    `json:"-"`
}

// EarnWayProvider 返回该来源对某用户当前是否可用；不可用返回 nil
type EarnWayProvider func(ctx context.Context, userID int64) (*EarnWay, error)

type earnWayReg struct {
	mu        sync.RWMutex
	providers []EarnWayProvider
}

// RegisterEarnWay 注册一个积分获取方式
func (s *Service) RegisterEarnWay(p EarnWayProvider) {
	s.earn.mu.Lock()
	s.earn.providers = append(s.earn.providers, p)
	s.earn.mu.Unlock()
}

// EarnWays 当前用户可用的积分获取方式；单个来源出错只记日志，不影响其他来源
func (s *Service) EarnWays(ctx context.Context, userID int64) []EarnWay {
	s.earn.mu.RLock()
	providers := append([]EarnWayProvider(nil), s.earn.providers...)
	s.earn.mu.RUnlock()
	out := []EarnWay{}
	for _, p := range providers {
		w, err := p(ctx, userID)
		if err != nil {
			log.Printf("[plus.points] earn way: %v", err)
			continue
		}
		if w != nil {
			out = append(out, *w)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

// redeemBonusEarnWay 兑换卡密送积分开启时可用
func (s *Service) redeemBonusEarnWay(ctx context.Context, _ int64) (*EarnWay, error) {
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.RedeemBonus.Enabled || len(cfg.RedeemBonus.Tiers) == 0 {
		return nil, nil
	}
	return &EarnWay{Key: ReasonRedeemBonus, Label: "兑换卡密送积分", Action: "route:/redeem", Order: 20}, nil
}
