package dispatch

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/plus/channelstatus"
)

// ChannelRules 渠道状态的健康规则（「和渠道状态用同一套规则」时读它）
type ChannelRules interface {
	Load(ctx context.Context) (channelstatus.Config, int64, error)
}

// Service 上游调度台的用例（按关注点拆在 service_*.go）
type Service struct {
	Config   ConfigStore
	Setup    SetupStore
	Runs     RunStore
	Ignores  IgnoreStore
	Rules    RuleStore
	AuditDB  AuditStore
	Channel  ChannelRules
	Registry *Registry
	Planner  Planner
	Snap     *Snapshotter
	Detect   *DetectionLoader
	Sched    *Scheduler
	Library  *FingerprintLibrary
	Preview  *Previewer
	OnRules  func() // 权重设置变了：立即刷新钩子
	LastSeen func(ctx context.Context) map[int64]time.Time
	lastMu   sync.Mutex
	lastAt   time.Time
	lastVal  map[int64]time.Time
}

// state 一次请求要用的只读数据
type state struct {
	now    time.Time
	cfg    Config
	health Health
	cat    *Catalog
	tr     *TrafficSet
	det    *Detection
	rules  []Rule
}

func (s *Service) load(ctx context.Context) (*state, error) {
	cfg, _, err := s.Config.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	snap, err := s.Snap.Get(ctx, false)
	if err != nil {
		return nil, err
	}
	det, err := s.Detect.Get(ctx)
	if err != nil {
		return nil, err
	}
	rules, err := s.Rules.ListRules(ctx)
	if err != nil {
		return nil, err
	}
	return &state{now: snap.Traffic.NowAt, cfg: cfg, health: Health{Rules: s.healthRules(ctx, cfg)}, cat: snap.Catalog, tr: snap.Traffic,
		det: det, rules: rules}, nil
}

// healthRules 健康分规则：默认和渠道状态同一套
func (s *Service) healthRules(ctx context.Context, cfg Config) channelstatus.Rules {
	if cfg.SameAsChannel && s.Channel != nil {
		if c, _, err := s.Channel.Load(ctx); err == nil {
			return c.Rules
		}
	}
	return cfg.Rules
}

// serving 调度开着的账号（按 id）
func (st *state) serving(platform string) []*CatAccount {
	var out []*CatAccount
	for _, a := range st.cat.Accounts {
		if a.Serving() && (platform == "" || a.Platform == platform) {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (st *state) lanes() *Lanes { return NewLanes(st.cat, st.tr, st.rules) }

func (st *state) accountName(id int64) string {
	if a := st.cat.Accounts[id]; a != nil {
		return a.Name
	}
	return "#" + itoa(id)
}

func (st *state) groupName(id int64) string {
	if g := st.cat.Groups[id]; g != nil {
		return g.Name
	}
	return "#" + itoa(id)
}

// lastSeen 账号最近一次请求的时间（近 2 小时内，缓存 30 秒）
func (s *Service) lastSeen(ctx context.Context) map[int64]time.Time {
	if s.LastSeen == nil {
		return nil
	}
	s.lastMu.Lock()
	defer s.lastMu.Unlock()
	if time.Since(s.lastAt) < 30*time.Second && s.lastVal != nil {
		return s.lastVal
	}
	s.lastVal, s.lastAt = s.LastSeen(ctx), time.Now()
	return s.lastVal
}

// audit 写操作日志（kind：weight | detect | config）
func (s *Service) audit(ctx context.Context, actor Actor, kind, action, target, summary string, before, after any) {
	email := actor.Email
	if email == "" && actor.ID > 0 {
		email = s.AuditDB.OperatorEmail(ctx, actor.ID)
	}
	var op *int64
	if actor.ID > 0 {
		id := actor.ID
		op = &id
	}
	if err := s.AuditDB.AddAudit(ctx, AuditEntry{OperatorID: op, Operator: email, Action: action, TargetType: kind, TargetID: target,
		Summary: summary, Before: before, After: after}); err != nil {
		log.Printf("[plus.dispatch] audit: %v", err)
	}
}

func (s *Service) operator(ctx context.Context, actor Actor) string {
	if actor.Email != "" {
		return actor.Email
	}
	if actor.ID > 0 {
		return s.AuditDB.OperatorEmail(ctx, actor.ID)
	}
	return ""
}
