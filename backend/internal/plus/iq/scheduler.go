// Package iq — [plus] 调度器：定时把到期任务放进队列，再按全局并发上限从队列里取出执行。
// 队列落在 plus_iq_run（status = queued），手动触发和定时触发走同一条路；
// 抢占都用 FOR UPDATE SKIP LOCKED，多实例部署不会重复执行。
package iq

import (
	"context"
	"log"
	"sync"
	"time"
)

const (
	schedulerTick  = 10 * time.Second
	claimBatch     = 20
	staleMargin    = 2 * time.Minute
	restartMessage = "服务重启，检测中断"
)

// Scheduler 调度器
type Scheduler struct {
	cfg   ConfigStore
	queue RunQueue
	exec  *Executor
	now   func() time.Time
	wake  chan struct{}
	wg    sync.WaitGroup
}

func NewScheduler(cfg ConfigStore, queue RunQueue, exec *Executor) *Scheduler {
	return &Scheduler{cfg: cfg, queue: queue, exec: exec, now: time.Now, wake: make(chan struct{}, 1)}
}

// Start 启动调度循环。启动时把遗留的运行中记录标为失败（上次进程退出时没跑完的）。
func (s *Scheduler) Start(ctx context.Context) {
	if n, err := s.queue.FailStale(ctx, s.now().Add(time.Second), restartMessage); err != nil {
		log.Printf("[plus.iq] clean stale runs: %v", err)
	} else if n > 0 {
		log.Printf("[plus.iq] marked %d interrupted runs as failed", n)
	}
	go func() {
		t := time.NewTicker(schedulerTick)
		defer t.Stop()
		s.Tick(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			case <-s.wake:
			}
			s.Tick(ctx)
		}
	}()
}

// Wake 手动触发后立即调度一轮，不用等下一个 tick
func (s *Scheduler) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Tick 调度一轮（导出给单测）
func (s *Scheduler) Tick(ctx context.Context) {
	cfg, _, err := s.cfg.LoadConfig(ctx)
	if err != nil {
		log.Printf("[plus.iq] load config: %v", err)
		return
	}
	now := s.now()
	if _, err := s.queue.FailStale(ctx, now.Add(-time.Duration(cfg.TimeoutSeconds)*time.Second-staleMargin), "检测超时未结束（进程可能中断）"); err != nil {
		log.Printf("[plus.iq] fail stale: %v", err)
	}
	if cfg.Enabled {
		due, err := s.queue.ClaimDue(ctx, now, claimBatch)
		if err != nil {
			log.Printf("[plus.iq] claim due: %v", err)
		}
		for _, t := range due {
			if _, err := s.queue.Enqueue(ctx, t, TriggerCron, nil, cfg.PromptVersion); err != nil {
				log.Printf("[plus.iq] enqueue task %d: %v", t.ID, err)
			}
		}
	}
	running, err := s.queue.CountRunning(ctx)
	if err != nil {
		log.Printf("[plus.iq] count running: %v", err)
		return
	}
	runs, err := s.queue.StartQueued(ctx, cfg.Concurrency-running)
	if err != nil {
		log.Printf("[plus.iq] start queued: %v", err)
		return
	}
	for _, r := range runs {
		s.wg.Add(1)
		go func(r Run) {
			defer s.wg.Done()
			defer func() {
				if p := recover(); p != nil {
					log.Printf("[plus.iq] run %d panic: %v", r.ID, p)
				}
			}()
			s.exec.Execute(context.Background(), r)
			s.Wake() // 空出一个并发位，马上取下一条
		}(r)
	}
}

// Wait 等待所有执行中的检测结束（单测用）
func (s *Scheduler) Wait() { s.wg.Wait() }
