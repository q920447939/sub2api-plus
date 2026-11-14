package dispatch

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Executor 执行一次主动检测：解析账号直连目标 → 调检测器 → 落库
type Executor struct {
	Runs     RunStore
	Setup    SetupStore
	Registry *Registry
	Channel  DirectChannel
	OnFinish func() // 有新结果：让结论重新计算
}

func (e *Executor) Execute(ctx context.Context, run Run) {
	start := time.Now()
	res := e.execute(ctx, run)
	if err := e.Runs.FinishRun(context.Background(), run.ID, res, int(time.Since(start).Milliseconds())); err != nil {
		log.Printf("[plus.dispatch] finish run %d: %v", run.ID, err)
	}
	if e.OnFinish != nil {
		e.OnFinish()
	}
}

func (e *Executor) execute(ctx context.Context, run Run) ExecResult {
	items, err := e.Setup.ListItems(ctx)
	if err != nil {
		return errResult("", err)
	}
	var item *Item
	for i := range items {
		if items[i].ID == run.ItemID {
			item = &items[i]
		}
	}
	if item == nil {
		return errResult("", fmt.Errorf("检测项已删除"))
	}
	det, ok := e.Registry.Get(item.Type)
	if !ok {
		return errResult("", fmt.Errorf("检测项类型 %s 不支持主动检测", item.Type))
	}
	var plan *Plan
	timeout := 180
	if run.PlanID != nil {
		plans, err := e.Setup.ListPlans(ctx)
		if err != nil {
			return errResult("", err)
		}
		for i := range plans {
			if plans[i].ID == *run.PlanID {
				plan = &plans[i]
				timeout = plans[i].TimeoutSeconds
			}
		}
	}
	target, err := e.Channel.Resolve(ctx, run.AccountID, run.Model)
	if err != nil {
		return errResult("", err)
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	res := det.Execute(cctx, ExecInput{Item: *item, Plan: plan, Target: target, Platform: run.Platform})
	if res.Outcome == OutcomeError && cctx.Err() == context.DeadlineExceeded {
		res.Error = fmt.Sprintf("超过 %d 秒没有完成（%s）", timeout, res.Error)
	}
	if res.Detail == nil {
		res.Detail = map[string]any{}
	}
	if target.Model != run.Model {
		res.Detail["upstream_model"] = target.Model // 账号的模型映射
	}
	return res
}

// Scheduler 定时把到期计划展开成排队记录，再按全局并发取出执行。
// 排队落库（plus_dispatch_run.status = queued），手动触发和定时触发走同一条路；多实例用 SKIP LOCKED 抢占。
type Scheduler struct {
	Config   ConfigStore
	Setup    SetupStore
	Runs     RunStore
	Planner  Planner
	Snap     *Snapshotter
	Exec     *Executor
	wake     chan struct{}
	inflight sync.WaitGroup
}

const (
	schedulerTick = 15 * time.Second
	runRetention  = 30 * 24 * time.Hour
)

func (s *Scheduler) Start(ctx context.Context) {
	s.wake = make(chan struct{}, 1)
	if n, err := s.Runs.FailStale(ctx, time.Now().Add(time.Second), "服务重启，检测中断"); err == nil && n > 0 {
		log.Printf("[plus.dispatch] marked %d interrupted runs as failed", n)
	}
	go func() {
		t := time.NewTicker(schedulerTick)
		defer t.Stop()
		lastPrune := time.Time{}
		for {
			s.Tick(ctx)
			if time.Since(lastPrune) > 6*time.Hour {
				lastPrune = time.Now()
				if err := s.Runs.PruneRuns(ctx, time.Now().Add(-runRetention)); err != nil {
					log.Printf("[plus.dispatch] prune runs: %v", err)
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			case <-s.wake:
			}
		}
	}()
}

// Wake 手动触发后立即调度一轮
func (s *Scheduler) Wake() {
	if s.wake == nil {
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Tick 调度一轮
func (s *Scheduler) Tick(ctx context.Context) {
	cfg, _, err := s.Config.LoadConfig(ctx)
	if err != nil {
		log.Printf("[plus.dispatch] load config: %v", err)
		return
	}
	// 最长的计划超时 + 余量之后还在运行的，认为进程中断过
	if _, err := s.Runs.FailStale(ctx, time.Now().Add(-40*time.Minute), "检测超时未结束（进程可能中断）"); err != nil {
		log.Printf("[plus.dispatch] fail stale: %v", err)
	}
	if cfg.Detect {
		plans, err := s.Runs.ClaimDuePlans(ctx, time.Now(), 50)
		if err != nil {
			log.Printf("[plus.dispatch] claim plans: %v", err)
		}
		if len(plans) > 0 {
			if n, err := s.EnqueuePlans(ctx, plans, "cron", nil); err != nil {
				log.Printf("[plus.dispatch] enqueue: %v", err)
			} else if n > 0 {
				log.Printf("[plus.dispatch] enqueued %d runs from %d plans", n, len(plans))
			}
		}
	}
	running, err := s.Runs.CountRunning(ctx)
	if err != nil {
		return
	}
	runs, err := s.Runs.StartQueued(ctx, cfg.Concurrency-running)
	if err != nil {
		log.Printf("[plus.dispatch] start queued: %v", err)
		return
	}
	for _, r := range runs {
		s.inflight.Add(1)
		go func(r Run) {
			defer s.inflight.Done()
			s.Exec.Execute(context.Background(), r)
			s.Wake() // 空出并发位，马上取下一条
		}(r)
	}
}

// EnqueuePlans 把计划展开成排队记录，返回新排队的条数。manual = 立即执行（不受启用开关和备用号间隔限制）
func (s *Scheduler) EnqueuePlans(ctx context.Context, plans []Plan, trigger string, by *int64) (int, error) {
	items, err := s.Setup.ListItems(ctx)
	if err != nil {
		return 0, err
	}
	cfg, _, err := s.Config.LoadConfig(ctx)
	if err != nil {
		return 0, err
	}
	snap, err := s.Snap.Get(ctx, false)
	if err != nil {
		return 0, err
	}
	last, err := s.Runs.LastRunAt(ctx)
	if err != nil {
		return 0, err
	}
	manual := trigger == "manual"
	n := 0
	for _, p := range plans {
		var item *Item
		for i := range items {
			if items[i].ID == p.ItemID {
				item = &items[i]
			}
		}
		if item == nil || (!item.Enabled && !manual) {
			continue
		}
		for _, t := range s.Planner.Due(ctx, p, *item, snap.Catalog, snap.Traffic, last, time.Duration(cfg.IdleMinutes)*time.Minute, time.Now(), manual) {
			pid := p.ID
			_, ok, err := s.Runs.Enqueue(ctx, Run{PlanID: &pid, ItemID: item.ID, AccountID: t.AccountID, CredKey: t.credKey,
				Platform: p.Platform, Model: t.Model, Trigger: trigger, TriggeredBy: by})
			if err != nil {
				return n, err
			}
			if ok {
				n++
			}
		}
	}
	return n, nil
}

// EnqueueOne 手动检测一个 账号 × 模型
func (s *Scheduler) EnqueueOne(ctx context.Context, item Item, a *CatAccount, model string, by *int64) (bool, error) {
	_, ok, err := s.Runs.Enqueue(ctx, Run{ItemID: item.ID, AccountID: a.ID, CredKey: a.CredKey, Platform: a.Platform, Model: model,
		Trigger: "manual", TriggeredBy: by})
	if ok {
		s.Wake()
	}
	return ok, err
}
