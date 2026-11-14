package hook

import (
	"context"
	"log"
	"time"
)

// RulesLoader 规则来源（数据库）
type RulesLoader interface {
	LoadRules(ctx context.Context) (enabled bool, rules []Rule, err error)
}

// Runner 后台刷新规则（多实例下各自从库里读）
type Runner struct {
	Engine  *Engine
	Loader  RulesLoader
	Refresh time.Duration // 默认 10 秒
	wake    chan struct{}
}

// Start 启动后台循环
func (r *Runner) Start(ctx context.Context) {
	if r.Refresh <= 0 {
		r.Refresh = 10 * time.Second
	}
	r.wake = make(chan struct{}, 1)
	r.Reload(ctx)
	go func() {
		t := time.NewTicker(r.Refresh)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.Reload(ctx)
			case <-r.wake:
				r.Reload(ctx)
			}
		}
	}()
}

// Wake 本实例刚改了设置，立即刷新
func (r *Runner) Wake() {
	if r.wake == nil {
		return
	}
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Reload 立即从库里刷新一次
func (r *Runner) Reload(ctx context.Context) {
	enabled, rules, err := r.Loader.LoadRules(ctx)
	if err != nil {
		log.Printf("[plus.dispatch] load rules: %v（沿用上一份规则）", err)
		return
	}
	r.Engine.Replace(rules)
	r.Engine.SetEnabled(enabled)
}
