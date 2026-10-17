package lottery

import (
	"context"
	"hash/fnv"
	"log"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// 虚拟昵称素材：最终都经 MaskEmail 输出，与真实用户同构，前端无法区分。
// 注册邮箱域名白名单非空时只用白名单内的域名，为空时用下面的常见域名。
var (
	virtualInitials = []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	virtualDomains  = []string{"gmail.com", "qq.com", "163.com", "outlook.com", "hotmail.com", "icloud.com", "yahoo.com"}
	virtualWeights  = []int{40, 25, 12, 8, 6, 5, 4}
)

// Worker 后台任务：机会同步兜底、虚拟动态生成。
// 多实例部署时各实例都可运行：所有写入都由唯一约束保证幂等。
type Worker struct {
	svc      *Service
	interval time.Duration
}

func NewWorker(svc *Service) *Worker {
	return &Worker{svc: svc, interval: time.Minute}
}

// Start 启动后台循环，ctx 取消时退出
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
			log.Printf("[plus.lottery] worker panic: %v", r)
		}
	}()
	cfg, _, err := w.svc.Config(ctx)
	if err != nil {
		log.Printf("[plus.lottery] load config: %v", err)
		return
	}
	if !cfg.Enabled {
		return
	}
	if err := w.svc.SyncChances(ctx, 0); err != nil {
		log.Printf("[plus.lottery] sync chances: %v", err)
	}
	if err := w.svc.GenerateVirtual(ctx, w.svc.now()); err != nil {
		log.Printf("[plus.lottery] virtual feed: %v", err)
	}
}

// hourlyWeights 东八区每小时的相对活跃度（均值为 1）：凌晨低、午间与晚高峰高
var hourlyWeights = func() [24]float64 {
	raw := [24]float64{
		0.45, 0.25, 0.15, 0.1, 0.1, 0.15, // 0-5 点
		0.35, 0.6, 0.9, 1.1, 1.2, 1.25, // 6-11 点
		1.3, 1.15, 1.1, 1.15, 1.2, 1.25, // 12-17 点
		1.35, 1.55, 1.7, 1.75, 1.5, 0.9, // 18-23 点
	}
	sum := 0.0
	for _, v := range raw {
		sum += v
	}
	var w [24]float64
	for i, v := range raw {
		w[i] = v * 24 / sum
	}
	return w
}()

// dayFactor 当日浮动系数 ∈ [0.7, 1.3]，由日期确定性推导，多实例与重启后保持一致
func dayFactor(dayKey string) float64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte("plus-lottery:" + dayKey))
	return 0.7 + 0.6*float64(h.Sum32()%10001)/10000
}

// virtualMinuteProb 某分钟生成一条虚拟记录的概率 = 日目标 × 日浮动 × 小时权重 / 1440
func virtualMinuteProb(target int, at time.Time) float64 {
	local := at.In(timezone.Location())
	p := float64(target) * dayFactor(DayOf(local).Key) * hourlyWeights[local.Hour()] / 1440
	return math.Min(p, 1)
}

// GenerateVirtual 每分钟调用一次，按 virtualMinuteProb 决定是否生成一条虚拟记录。
// 分钟级 slot 唯一约束保证多实例下同一分钟最多一条。
func (s *Service) GenerateVirtual(ctx context.Context, now time.Time) error {
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return err
	}
	vf := cfg.VirtualFeed
	if !cfg.Enabled || !vf.Enabled || vf.DailyTarget <= 0 {
		return nil
	}
	if s.rng.Float64() >= virtualMinuteProb(vf.DailyTarget, now) {
		return nil
	}
	amount := SampleVirtualAmount(cfg.PrizePoolVirtual, s.rng)
	if amount <= 0 {
		return nil
	}
	slot := now.Unix() / 60
	// 分钟内随机秒，避免时间戳整齐露出破绽
	at := time.Unix(slot*60+int64(s.rng.Float64()*60), 0)
	whitelist, err := s.repo.EmailSuffixWhitelist(ctx)
	if err != nil {
		return err
	}
	_, err = s.repo.InsertVirtual(ctx, slot, s.virtualName(whitelist), amount, at)
	return err
}

// virtualName 生成脱敏后的虚拟昵称。whitelist 为上游归一化后的后缀（"@qq.com" 或 "*.edu.cn"），
// 非空时等概率选一个白名单项；通配项 "*.x" 取其主域 x（上游同样允许 x 本身）。
func (s *Service) virtualName(whitelist []string) string {
	first := s.pickRune(virtualInitials)
	var domain string
	if len(whitelist) > 0 {
		item := whitelist[s.pickIndex(len(whitelist))]
		domain = strings.TrimPrefix(strings.TrimPrefix(item, "*."), "@")
	} else {
		domain = virtualDomains[s.pickWeighted(virtualWeights)]
	}
	return MaskEmail(string(first) + "x@" + domain)
}

func (s *Service) pickIndex(n int) int {
	return int(s.rng.Float64()*float64(n)) % n
}

func (s *Service) pickRune(rs []rune) rune {
	return rs[s.pickIndex(len(rs))]
}

func (s *Service) pickWeighted(weights []int) int {
	total := 0
	for _, w := range weights {
		total += w
	}
	r := s.rng.Float64() * float64(total)
	for i, w := range weights {
		if r < float64(w) {
			return i
		}
		r -= float64(w)
	}
	return 0
}
