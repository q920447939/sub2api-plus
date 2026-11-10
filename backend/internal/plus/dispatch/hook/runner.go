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

// HitSink 介入计数落库
type HitSink interface {
	AddHits(ctx context.Context, bucket time.Time, hits map[HitKey]int64) error
}

// Runner 后台刷新规则（多实例下各自从库里读）、定时把介入计数落库
type Runner struct {
	Engine  *Engine
	Loader  RulesLoader
	Sink    HitSink
	Refresh time.Duration // 默认 10 秒
	Flush   time.Duration // 默认 1 分钟
	wake    chan struct{}
}

// Start 启动后台循环
func (r *Runner) Start(ctx context.Context) {
	if r.Refresh <= 0 {
		r.Refresh = 10 * time.Second
	}
	if r.Flush <= 0 {
		r.Flush = time.Minute
	}
	r.wake = make(chan struct{}, 1)
	r.Reload(ctx)
	go func() {
		rt := time.NewTicker(r.Refresh)
		ft := time.NewTicker(r.Flush)
		defer rt.Stop()
		defer ft.Stop()
		for {
			select {
			case <-ctx.Done():
				r.flush(context.Background())
				return
			case <-rt.C:
				r.Reload(ctx)
			case <-r.wake:
				r.Reload(ctx)
			case <-ft.C:
				r.flush(ctx)
			}
		}
	}()
}

// Wake 本实例刚改了调整，立即刷新
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

func (r *Runner) flush(ctx context.Context) {
	hits := r.Engine.DrainHits()
	if len(hits) == 0 || r.Sink == nil {
		return
	}
	if err := r.Sink.AddHits(ctx, time.Now().Truncate(time.Hour), hits); err != nil {
		log.Printf("[plus.dispatch] flush hook hits: %v", err)
	}
}
