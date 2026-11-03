// Package slotmachine — [plus] 后台任务：跑马灯虚拟中奖记录
package slotmachine

import (
	"context"
	"hash/fnv"
	"log"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

var (
	virtualInitials = []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	virtualDomains  = []string{"gmail.com", "qq.com", "163.com", "outlook.com", "hotmail.com", "icloud.com"}
	virtualDomainW  = []float64{40, 25, 12, 9, 7, 7}
)

// Worker 每分钟按概率生成一条虚拟动态；多实例下由 virtual_slot 唯一约束保证幂等
type Worker struct {
	svc      *Service
	interval time.Duration
}

func NewWorker(svc *Service) *Worker { return &Worker{svc: svc, interval: time.Minute} }

func (w *Worker) Start(ctx context.Context) {
	go func() {
		t := time.NewTicker(w.interval)
		defer t.Stop()
		for {
			w.tick(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

func (w *Worker) tick(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[plus.slot] worker panic: %v", r)
		}
	}()
	if err := w.svc.GenerateVirtual(ctx, w.svc.now()); err != nil {
		log.Printf("[plus.slot] virtual feed: %v", err)
	}
}

// 东八区每小时相对活跃度：凌晨低、晚高峰高（与刮刮乐同一曲线）
var hourlyWeights = [24]float64{
	0.45, 0.25, 0.15, 0.1, 0.1, 0.15, 0.35, 0.6, 0.9, 1.1, 1.2, 1.25,
	1.3, 1.15, 1.1, 1.15, 1.2, 1.25, 1.35, 1.55, 1.7, 1.75, 1.5, 0.9,
}

var hourlySum = func() float64 {
	s := 0.0
	for _, v := range hourlyWeights {
		s += v
	}
	return s
}()

// minuteProb 某分钟生成一条虚拟记录的概率 = 日目标 × 日浮动[0.7,1.3] × 小时权重 / 1440
func minuteProb(target int, at time.Time) float64 {
	local := at.In(timezone.Location())
	h := fnv.New32a()
	_, _ = h.Write([]byte("plus-slot:" + local.Format("2006-01-02")))
	day := 0.7 + 0.6*float64(h.Sum32()%10001)/10000
	p := float64(target) * day * (hourlyWeights[local.Hour()] * 24 / hourlySum) / 1440
	return math.Min(p, 1)
}

// GenerateVirtual 按"人气氛围"配置生成一条虚拟中奖记录。奖档按单独配置的虚拟分布抽取，
// 与真实记录写同一张表，用户端无法区分。
func (s *Service) GenerateVirtual(ctx context.Context, now time.Time) error {
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return err
	}
	a := cfg.Atmosphere
	if !cfg.Enabled || !a.MarqueeEnabled || !a.VirtualEnabled || a.VirtualDailyTarget <= 0 ||
		len(a.VirtualProbs) != len(cfg.Tiers) {
		return nil
	}
	if s.rng.Float64() >= minuteProb(a.VirtualDailyTarget, now) {
		return nil
	}
	idx := PickBP(a.VirtualProbs, s.rng)
	t := cfg.Tiers[idx]
	if t.PrizeType == PrizeTypeNone {
		return nil
	}
	whitelist, err := s.repo.EmailSuffixWhitelist(ctx)
	if err != nil {
		return err
	}
	slot := now.Unix() / 60
	// 分钟内随机秒，避免时间戳整齐露出破绽
	at := time.Unix(slot*60+int64(s.rng.Float64()*60), 0)
	return s.repo.InsertFeed(ctx, s.repo.db, FeedItem{
		DisplayName: s.virtualName(whitelist), TierIndex: idx, Label: t.Label, Emoji: t.Emoji, CreatedAt: at,
	}, slot)
}

// virtualName 与真实用户同构的脱敏昵称；注册白名单非空时只用白名单域名
func (s *Service) virtualName(whitelist []string) string {
	first := virtualInitials[int(s.rng.Float64()*float64(len(virtualInitials)))%len(virtualInitials)]
	var domain string
	if len(whitelist) > 0 {
		item := whitelist[int(s.rng.Float64()*float64(len(whitelist)))%len(whitelist)]
		domain = strings.TrimPrefix(strings.TrimPrefix(item, "*."), "@")
	} else {
		domain = virtualDomains[PickWeighted(virtualDomainW, s.rng)]
	}
	return MaskEmail(string(first) + "x@" + domain)
}
