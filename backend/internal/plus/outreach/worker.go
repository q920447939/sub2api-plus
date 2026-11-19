// Package outreach — 发送 worker：只处理「执行中」的批次，按限速取收件人发送，失败重试，退信率过高自动熔断。
// 另外每小时清理一次保留期满的已删除批次。
package outreach

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

const (
	tickEvery     = 5 * time.Second
	purgeEvery    = time.Hour
	maxPerTick    = 10 // 单次最多处理条数，避免一次占用太久
	maxAttempts   = 3
	rateBackoff   = 10 * time.Minute
	retryInterval = 5 * time.Minute
)

type Worker struct {
	svc       *Service
	lastPurge time.Time
}

func NewWorker(svc *Service) *Worker { return &Worker{svc: svc} }

func (w *Worker) Start(ctx context.Context) {
	if err := w.svc.deps.Repo.RecoverStuck(ctx); err != nil {
		log.Printf("[plus.outreach] recover stuck: %v", err)
	}
	go func() {
		t := time.NewTicker(tickEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				w.tick(ctx)
				w.maybePurge(ctx)
			}
		}
	}()
}

func (w *Worker) maybePurge(ctx context.Context) {
	if time.Since(w.lastPurge) < purgeEvery {
		return
	}
	w.lastPurge = time.Now()
	b, c, err := w.svc.PurgeDeleted(ctx)
	if err != nil {
		log.Printf("[plus.outreach] purge deleted: %v", err)
		return
	}
	if b > 0 || c > 0 {
		log.Printf("[plus.outreach] purged %d batches, %d campaigns past retention", b, c)
	}
}

type errClass int

const (
	errSoft errClass = iota
	errHardBounce
	errRateLimit
)

// classify 按 SMTP 错误文本粗分类。注意：只能识别发信时同步返回的拒收；
// 服务商接收后再异步退回的邮件（常见于 Brevo 这类中继）这里看不到。
func classify(err error) errClass {
	m := strings.ToLower(err.Error())
	for _, c := range []string{"smtp rcpt: 550", "smtp rcpt: 551", "smtp rcpt: 553"} {
		if strings.Contains(m, c) {
			return errHardBounce
		}
	}
	for _, k := range []string{"user unknown", "no such user", "mailbox not found", "invalid recipient"} {
		if strings.Contains(m, k) {
			return errHardBounce
		}
	}
	for _, k := range []string{"smtp auth", "421", " 450", " 452", "too many", "rate limit", "quota", "exceed", "daily"} {
		if strings.Contains(m, k) {
			return errRateLimit
		}
	}
	return errSoft
}

func (w *Worker) tick(ctx context.Context) {
	s := w.svc
	repo := s.deps.Repo
	ids, err := repo.RunningBatchIDs(ctx)
	if err != nil || len(ids) == 0 {
		if err != nil {
			log.Printf("[plus.outreach] list running: %v", err)
		}
		return
	}
	cfg := s.cfg(ctx)
	perMin, perDay, err := repo.SentCounts(ctx)
	if err != nil {
		log.Printf("[plus.outreach] sent counts: %v", err)
		return
	}
	allowed := min(cfg.PerMinute-perMin, cfg.PerDay-perDay, maxPerTick)
	for _, id := range ids {
		if allowed <= 0 {
			return
		}
		allowed -= w.runBatch(ctx, id, allowed, cfg)
	}
}

// runBatch 处理一个批次的一轮收件人，返回实际占用的额度
func (w *Worker) runBatch(ctx context.Context, id int64, n int, cfg DeliveryConfig) int {
	s := w.svc
	repo := s.deps.Repo
	b, err := repo.GetBatch(ctx, id)
	if err != nil || b.Status != StatusRunning {
		return 0
	}
	leased, err := repo.Lease(ctx, id, n)
	if err != nil {
		log.Printf("[plus.outreach] lease batch %d: %v", id, err)
		return 0
	}
	used := 0
	halted := false
	for _, l := range leased {
		if halted {
			_ = repo.RequeueFree(ctx, l.ID, 0, "")
			continue
		}
		switch w.deliver(ctx, b, l, cfg) {
		case resSent, resFailed:
			used++
		case resHalt:
			halted = true
		}
	}
	if halted {
		return used
	}

	if decided, bounced, err := repo.BounceRate(ctx, id); err == nil &&
		decided >= cfg.BreakerMinSamples && bounced*100/decided >= cfg.BreakerPercent {
		reason := fmt.Sprintf("退信率 %d%%（%d/%d）超过熔断阈值 %d%%，已自动暂停，请确认后手动恢复", bounced*100/decided, bounced, decided, cfg.BreakerPercent)
		if err := repo.SetBatchStatus(ctx, id, StatusPaused, reason, StatusRunning); err == nil {
			log.Printf("[plus.outreach] batch %d paused: %s", id, reason)
			return used
		}
	}
	if left, err := repo.PendingLeft(ctx, id); err == nil && left == 0 {
		_ = repo.SetBatchStatus(ctx, id, StatusDone, "", StatusRunning)
	}
	return used
}

type result int

const (
	resSent result = iota
	resFailed
	resSkipped
	resRetry
	resHalt
)

// deliver 批次投递：投递本身由 Service.deliverOne 完成（与直发共用），这里只把结果映射成队列状态
func (w *Worker) deliver(ctx context.Context, b *Batch, l Leased, _ DeliveryConfig) result {
	s := w.svc
	repo := s.deps.Repo
	p, err := s.deps.Purposes.Lookup(b.Purpose)
	if err != nil {
		_ = repo.RequeueFree(ctx, l.ID, rateBackoff, err.Error())
		_ = repo.SetBatchStatus(ctx, b.ID, StatusPaused, "邮件目的不可用，已自动暂停", StatusRunning)
		return resHalt
	}
	out := s.deliverOne(ctx, batchSpec(p, b), l, true)
	switch {
	case out.Skip != "":
		_ = repo.MarkSkipped(ctx, l.ID, out.Skip)
		return resSkipped
	case out.Halt != nil:
		_ = repo.RequeueFree(ctx, l.ID, rateBackoff, out.Halt.Error())
		_ = repo.SetBatchStatus(ctx, b.ID, StatusPaused, truncate(out.Halt.Error(), 200)+"，已自动暂停", StatusRunning)
		return resHalt
	case out.Retry != nil:
		_ = repo.Requeue(ctx, l.ID, time.Minute, out.Retry.Error())
		return resRetry
	case out.SendErr == nil:
		return resSent
	}

	sendErr := out.SendErr
	switch classify(sendErr) {
	case errHardBounce:
		_ = repo.MarkFailed(ctx, l.ID, true, sendErr.Error())
		if s.deps.Verify != nil {
			_ = s.deps.Verify.MarkInvalid(ctx, l.UserID, l.Email)
		}
		return resFailed
	case errRateLimit:
		_ = repo.RequeueFree(ctx, l.ID, rateBackoff, sendErr.Error())
		reason := "发信服务商限流或认证失败，已自动暂停：" + truncate(sendErr.Error(), 150)
		_ = repo.SetBatchStatus(ctx, b.ID, StatusPaused, reason, StatusRunning)
		log.Printf("[plus.outreach] batch %d paused: %s", b.ID, reason)
		return resHalt
	}
	if l.Attempts >= maxAttempts {
		_ = repo.MarkFailed(ctx, l.ID, false, sendErr.Error())
		return resFailed
	}
	_ = repo.Requeue(ctx, l.ID, time.Duration(l.Attempts)*retryInterval, sendErr.Error())
	return resRetry
}
