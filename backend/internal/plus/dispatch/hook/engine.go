// Package hook — [plus] 上游调度台的执行层：在请求选号的热路径上按「当前调整」改写候选名单。
//
// 只有两种动作：
//   - pause：从候选名单里拿掉（粘性会话也会换号）
//   - factor：每次按 1-factor 的概率拿掉；粘住的会话不受影响（保住缓存）
//
// 设计约束：
//   - 热路径零 IO：规则表是不可变快照，由后台定时整体替换（atomic.Pointer）
//   - 名单被拿空时原样返回，绝不因为调整导致请求无号可用
//   - 总开关关闭时直接返回，规则保留，重新打开后照原样生效
//   - 不依赖上游 service 包，只处理账号 ID（由 service/plus_dispatch_hook.go 适配）
package hook

import (
	"math/rand/v2"
	"sync"
	"sync/atomic"
)

// Action 调整动作
type Action string

const (
	ActionPause  Action = "pause"
	ActionFactor Action = "factor"
)

// Rule 某个分组里某个账号的当前调整
type Rule struct {
	GroupID   int64
	AccountID int64
	Action    Action
	Factor    float64 // 仅 ActionFactor 使用，(0,1)
}

type table map[int64]map[int64]Rule // group → account → rule

// Engine 规则快照 + 介入计数
type Engine struct {
	enabled atomic.Bool
	rules   atomic.Pointer[table]
	rnd     func() float64

	mu   sync.Mutex
	hits map[HitKey]int64
}

// HitKey 介入计数的维度
type HitKey struct {
	GroupID   int64
	AccountID int64
	Action    Action
}

// New 创建引擎（默认关闭、无规则）
func New() *Engine {
	e := &Engine{rnd: rand.Float64, hits: map[HitKey]int64{}}
	empty := table{}
	e.rules.Store(&empty)
	return e
}

// SetRandom 单测注入随机源
func (e *Engine) SetRandom(f func() float64) { e.rnd = f }

// SetEnabled 总开关
func (e *Engine) SetEnabled(on bool) { e.enabled.Store(on) }

// Enabled 总开关状态
func (e *Engine) Enabled() bool { return e.enabled.Load() }

// Replace 整体替换规则表
func (e *Engine) Replace(rules []Rule) {
	t := table{}
	for _, r := range rules {
		if r.Action != ActionPause && r.Action != ActionFactor {
			continue
		}
		if r.Action == ActionFactor && (r.Factor <= 0 || r.Factor >= 1) {
			continue
		}
		g := t[r.GroupID]
		if g == nil {
			g = map[int64]Rule{}
			t[r.GroupID] = g
		}
		g[r.AccountID] = r
	}
	e.rules.Store(&t)
}

func (e *Engine) groupRules(groupID int64) map[int64]Rule {
	if groupID <= 0 || !e.enabled.Load() {
		return nil
	}
	t := e.rules.Load()
	if t == nil {
		return nil
	}
	return (*t)[groupID]
}

// Filter 决定候选名单里哪些账号要拿掉。返回 nil 表示不改动。
// stickyAccountID：本次请求已粘住的账号（未知为 0），factor 调整不拿掉它。
func (e *Engine) Filter(groupID int64, ids []int64, stickyAccountID int64) (drop []bool) {
	rules := e.groupRules(groupID)
	if len(rules) == 0 || len(ids) == 0 {
		return nil
	}
	drop = make([]bool, len(ids))
	any, kept := false, 0
	type hit struct {
		id     int64
		action Action
	}
	var hits []hit
	for i, id := range ids {
		r, ok := rules[id]
		switch {
		case !ok:
		case r.Action == ActionPause:
			drop[i] = true
		case r.Action == ActionFactor && id != stickyAccountID && e.rnd() >= r.Factor:
			drop[i] = true
		}
		if drop[i] {
			any = true
			hits = append(hits, hit{id, r.Action})
		} else {
			kept++
		}
	}
	if !any {
		return nil
	}
	if kept == 0 {
		// 安全阀：调整不能让请求无号可用
		e.count(HitKey{GroupID: groupID, AccountID: 0, Action: "fallback"}, 1)
		return nil
	}
	for _, h := range hits {
		e.count(HitKey{GroupID: groupID, AccountID: h.id, Action: h.action}, 1)
	}
	return drop
}

// Paused 粘性会话路径用：这个账号在这个分组是否被暂停
func (e *Engine) Paused(groupID, accountID int64) bool {
	// 这里不计数：上游可能对同一个账号反复校验，计数会虚高；介入次数以候选名单层为准
	r, ok := e.groupRules(groupID)[accountID]
	return ok && r.Action == ActionPause
}

func (e *Engine) count(k HitKey, n int64) {
	e.mu.Lock()
	e.hits[k] += n
	e.mu.Unlock()
}

// DrainHits 取出并清零介入计数（由后台定时落库）
func (e *Engine) DrainHits() map[HitKey]int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.hits) == 0 {
		return nil
	}
	out := e.hits
	e.hits = map[HitKey]int64{}
	return out
}
