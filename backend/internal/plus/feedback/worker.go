// Package feedback — [plus] 后台任务：每小时检查待补充超时、清理未绑定的附件
package feedback

import (
	"context"
	"log"
	"time"
)

// StartWorker 启动后台循环，ctx 取消时退出
func (s *Service) StartWorker(ctx context.Context, interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			s.tick(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

func (s *Service) tick(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[plus.feedback] worker panic: %v", r)
		}
	}()
	if n, err := s.AutoCloseExpired(ctx); err != nil {
		log.Printf("[plus.feedback] auto close: %v", err)
	} else if n > 0 {
		log.Printf("[plus.feedback] auto closed %d awaiting feedback", n)
	}
	s.CleanOrphans(ctx)
}
