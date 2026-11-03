// Package points — [plus] 后台定时任务：每小时清零过期积分，每 10 分钟补发兑换送积分
package points

import (
	"context"
	"log"
	"time"
)

// Worker 后台定时扫描过期积分
type Worker struct {
	svc            *Service
	interval       time.Duration
	backfillPeriod time.Duration
}

func NewWorker(svc *Service) *Worker {
	return &Worker{svc: svc, interval: time.Hour, backfillPeriod: 10 * time.Minute}
}

// Start 启动后台循环，ctx 取消时退出
func (w *Worker) Start(ctx context.Context) {
	go func() {
		// 启动时立即执行一次，之后按 interval 定时
		w.tick(ctx)
		w.backfill(ctx)
		t := time.NewTicker(w.interval)
		defer t.Stop()
		b := time.NewTicker(w.backfillPeriod)
		defer b.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				w.tick(ctx)
			case <-b.C:
				w.backfill(ctx)
			}
		}
	}()
}

func (w *Worker) tick(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[plus.points] worker panic: %v", r)
		}
	}()
	res, err := w.svc.ExpireStale(ctx)
	if err != nil {
		log.Printf("[plus.points] expire stale: %v", err)
		return
	}
	if res.ExpiredAccounts > 0 {
		log.Printf("[plus.points] expired %d accounts, %d points", res.ExpiredAccounts, res.ExpiredPoints)
	}
}

func (w *Worker) backfill(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[plus.points] backfill panic: %v", r)
		}
	}()
	if _, err := w.svc.BackfillRedeemBonus(ctx); err != nil {
		log.Printf("[plus.points] redeem bonus backfill: %v", err)
	}
}
