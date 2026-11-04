// Package channelstatus — [plus] 按需兜底探测器，实现 FallbackJudge。
//
// 只在用户打开渠道状态页、且分组被动样本不足时被调用：
//   - 同步：读出最近一次探测结果，按 ProbePolicy 判断能否作为当前状态（不发请求，开销是一次查询）；
//   - 异步：满足触发条件时抢占（多实例 CAS）后在后台发一次探测，结果下次刷新可见。
//
// 探测结果转换成和真实流量同一口径的信号，再交给同一个 Scorer / Classifier，
// 所以用户看到的状态和健康分不区分来源。
package channelstatus

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"
)

// ProberDeps 依赖注入
type ProberDeps struct {
	Store      ProbeStore
	Keys       KeyStore
	Catalog    GroupCatalog
	Runner     ProbeRunner
	Scorer     Scorer
	Classifier Classifier
	Location   *time.Location
	Now        func() time.Time
	// Dispatch 异步执行；为 nil 时用 goroutine。单测可替换为同步执行
	Dispatch func(func())
}

type Prober struct {
	d        ProberDeps
	inflight atomic.Int32
}

var _ FallbackJudge = (*Prober)(nil)

func NewProber(d ProberDeps) *Prober {
	if d.Scorer == nil {
		d.Scorer = DefaultScorer{}
	}
	if d.Classifier == nil {
		d.Classifier = ThresholdClassifier{}
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Dispatch == nil {
		d.Dispatch = func(f func()) { go f() }
	}
	return &Prober{d: d}
}

func (p *Prober) policy(cfg Config) ProbePolicy { return ProbePolicy{P: cfg.Probe, Loc: p.d.Location} }

// Judge 见 FallbackJudge
func (p *Prober) Judge(ctx context.Context, cfg Config, lacking map[int64]int64) map[int64]Verdict {
	if !cfg.Probe.Enabled || cfg.Probe.Endpoint == "" || len(lacking) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(lacking))
	for id := range lacking {
		if g, ok := cfg.Probe.GroupSetting(id); ok && g.Enabled {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	states, err := p.d.Store.LoadStates(ctx, ids)
	if err != nil {
		log.Printf("[plus.channelstatus] load probe states: %v", err)
		return nil
	}
	pol := p.policy(cfg)
	now := p.d.Now()
	timeout := time.Duration(cfg.Probe.TimeoutSeconds) * time.Second
	out := make(map[int64]Verdict, len(ids))
	for _, id := range ids {
		st := states[id]
		if pol.UsableResult(st, now) {
			out[id] = p.verdict(st, cfg.Rules)
		}
		hadTraffic := lacking[id] > 0
		if pol.ShouldProbe(st, now, hadTraffic, timeout) {
			p.dispatch(cfg, id, st, hadTraffic)
		}
	}
	return out
}

// verdict 把探测结果换算成与真实流量同口径的信号再评分：成功 = 0 错误、延迟按 TTFT 打分；失败 = 全错
func (p *Prober) verdict(st *ProbeState, r Rules) Verdict {
	n := r.MinSample
	sig := Signal{Requests: n}
	if st.LastOK != nil && !*st.LastOK {
		sig.Errors = n
	} else if st.LatencyMs != nil {
		lat := int64(*st.LatencyMs)
		sig.TTFTP50Ms, sig.TTFTSamples = &lat, n
	}
	score := p.d.Scorer.Score(sig, r)
	return Verdict{Status: p.d.Classifier.Classify(score, r), Score: score}
}

func (p *Prober) dispatch(cfg Config, groupID int64, st *ProbeState, hadTraffic bool) {
	if int(p.inflight.Add(1)) > cfg.Probe.Concurrency {
		p.inflight.Add(-1)
		return // 本实例并发已满，下次有人查看时再试
	}
	gp, _ := cfg.Probe.GroupSetting(groupID)
	var prevStarted *time.Time
	streak := 0
	if st != nil {
		prevStarted, streak = st.LastStartedAt, st.IdleStreak
	}
	if hadTraffic {
		streak = 0
	} else {
		streak++
	}
	p.d.Dispatch(func() {
		defer p.inflight.Add(-1)
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Probe.TimeoutSeconds+5)*time.Second)
		defer cancel()
		if _, err := p.run(ctx, cfg.Probe, gp, prevStarted, streak, false); err != nil && !errors.Is(err, errNotClaimed) {
			log.Printf("[plus.channelstatus] probe group %d: %v", groupID, err)
		}
	})
}

var errNotClaimed = errors.New("channelstatus: probe not claimed")

// run 抢占 → 解析 Key → 发请求 → 落库。force=true 用于管理端「立即探测」，跳过 CAS 与每日上限
func (p *Prober) run(ctx context.Context, probe Probe, gp GroupProbe, prevStarted *time.Time, streak int, force bool) (*ProbeResult, error) {
	pol := ProbePolicy{P: probe, Loc: p.d.Location}
	now := p.d.Now()
	claimed, err := p.d.Store.Claim(ctx, gp.GroupID, prevStarted, now, pol.Today(now), probe.DailyLimit, force)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, errNotClaimed
	}
	res := p.execute(ctx, probe, gp)
	if err := p.d.Store.Finish(ctx, gp.GroupID, p.d.Now(), res.OK, res.LatencyMs, res.Message, streak); err != nil {
		return &res, err
	}
	return &res, nil
}

func (p *Prober) execute(ctx context.Context, probe Probe, gp GroupProbe) ProbeResult {
	key, keyGroup, err := p.d.Keys.ResolveKey(ctx, gp.APIKeyID)
	if err != nil {
		return ProbeResult{Message: "探测 Key 不可用（已删除、停用或过期）"}
	}
	if keyGroup != gp.GroupID {
		return ProbeResult{Message: "探测 Key 已不属于该分组"}
	}
	platform := ""
	if groups, err := p.d.Catalog.ListGroups(ctx); err == nil {
		for _, g := range groups {
			if g.ID == gp.GroupID {
				platform = g.Platform
			}
		}
	}
	rctx, cancel := context.WithTimeout(ctx, time.Duration(probe.TimeoutSeconds)*time.Second)
	defer cancel()
	return p.d.Runner.Run(rctx, ProbeRequest{
		Endpoint: probe.Endpoint, APIKey: key, Model: gp.Model, Format: ResolveFormat(gp.Format, platform),
	})
}

// RunNow 管理端「立即探测」：用传入的（可能未保存的）分组设置同步探测一次，结果照常落库
func (p *Prober) RunNow(ctx context.Context, probe Probe, gp GroupProbe) (*ProbeResult, error) {
	probe.Endpoint = strings.TrimRight(strings.TrimSpace(probe.Endpoint), "/")
	gp.Model = strings.TrimSpace(gp.Model)
	if probe.Endpoint == "" {
		return nil, invalid("请先填写网关地址")
	}
	if gp.APIKeyID <= 0 || gp.Model == "" {
		return nil, invalid("请先选择 Key 和模型")
	}
	if probe.TimeoutSeconds <= 0 {
		probe.TimeoutSeconds = DefaultProbe().TimeoutSeconds
	}
	streak := 0
	if states, err := p.d.Store.LoadStates(ctx, []int64{gp.GroupID}); err == nil && states[gp.GroupID] != nil {
		streak = states[gp.GroupID].IdleStreak // 手动探测不影响退避计数
	}
	res, err := p.run(ctx, probe, gp, nil, streak, true)
	if err != nil {
		return nil, fmt.Errorf("probe: %w", err)
	}
	return res, nil
}

// States 管理端查看最近一次探测
func (p *Prober) States(ctx context.Context, groupIDs []int64) (map[int64]*ProbeState, error) {
	return p.d.Store.LoadStates(ctx, groupIDs)
}

// Keys 管理端可选的探测 Key（当前管理员自己的 Key，与 V1「使用我的 Key」一致）
func (p *Prober) Keys(ctx context.Context, userID int64) ([]ProbeKey, error) {
	return p.d.Keys.ListUserKeys(ctx, userID)
}
