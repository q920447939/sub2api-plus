// Package channelstatus — [plus] 编排层：配置 + 分组目录 + 信号来源 + 评分 + 分类 → 用户端快照。
// 只依赖 ports.go 中的接口，具体实现由 Deps 注入。
package channelstatus

import (
	"context"
	"errors"
	"time"
)

var ErrDisabled = errors.New("channelstatus: disabled")

// Deps 依赖注入
type Deps struct {
	Config     ConfigStore
	Catalog    GroupCatalog
	Source     SignalSource  // 被动信号（通常是 CachedSource 包装的 PassiveSource）
	Scorer     Scorer        // 为 nil 时用 DefaultScorer
	Classifier Classifier    // 为 nil 时用 ThresholdClassifier
	Fallback   FallbackJudge // 可选：被动样本不足时的兜底判定
	Location   *time.Location
	Now        func() time.Time
	OnSaved    func() // 配置保存后的回调（如清空缓存）
}

type Service struct{ d Deps }

func NewService(d Deps) *Service {
	if d.Scorer == nil {
		d.Scorer = DefaultScorer{}
	}
	if d.Classifier == nil {
		d.Classifier = ThresholdClassifier{}
	}
	if d.Location == nil {
		d.Location = time.Local
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d}
}

// Meta 用户端轻量信息（侧边栏显隐、默认粒度）
type Meta struct {
	Enabled            bool        `json:"enabled"`
	RefreshSeconds     int         `json:"refresh_seconds"`
	DefaultGranularity Granularity `json:"default_granularity"`
}

func (s *Service) Meta(ctx context.Context) (Meta, error) {
	cfg, _, err := s.d.Config.Load(ctx)
	if err != nil {
		return Meta{}, err
	}
	return Meta{Enabled: cfg.Enabled, RefreshSeconds: cfg.RefreshSeconds, DefaultGranularity: cfg.DefaultGranularity}, nil
}

// visibleRow 一行待计算的分组
type visibleRow struct {
	platform int
	layout   GroupLayout
	info     GroupInfo
}

// Snapshot 用户端快照；granularity 为空时取配置默认值
func (s *Service) Snapshot(ctx context.Context, granularity string) (*Snapshot, error) {
	cfg, _, err := s.d.Config.Load(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, ErrDisabled
	}
	gran := cfg.DefaultGranularity
	if g, ok := ParseGranularity(granularity); ok {
		gran = g
	}
	catalog, err := s.d.Catalog.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]GroupInfo, len(catalog))
	for _, g := range catalog {
		byID[g.ID] = g
	}
	merged := MergeLayout(cfg.Layout, catalog, cfg.AutoShowNewGroups)

	snap := &Snapshot{Granularity: gran, RefreshSeconds: cfg.RefreshSeconds, Platforms: []PlatformView{}}
	var rows []visibleRow
	for _, p := range merged.Layout {
		if !p.Visible {
			continue
		}
		pv := PlatformView{Platform: p.Platform, Name: p.TabName, Groups: []GroupView{}}
		if pv.Name == "" {
			pv.Name = PlatformDisplayName(p.Platform)
		}
		pi := -1
		for _, g := range p.Groups {
			if !g.Visible {
				continue
			}
			if pi < 0 {
				snap.Platforms = append(snap.Platforms, pv)
				pi = len(snap.Platforms) - 1
			}
			rows = append(rows, visibleRow{platform: pi, layout: g, info: byID[g.GroupID]})
		}
	}
	if len(rows) == 0 {
		return snap, nil
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.layout.GroupID
	}

	now := s.d.Now().Truncate(time.Minute) // 对齐到分钟，使缓存键在一分钟内稳定
	tl := BuildTimeline(gran, now, s.d.Location)
	history, err := s.d.Source.Fetch(ctx, ids, tl.Start, tl.End, tl.Bucket)
	if err != nil {
		return nil, err
	}
	current, err := s.currentVerdicts(ctx, cfg, ids, now, time.Duration(cfg.JudgeWindowMinutes)*time.Minute)
	if err != nil {
		return nil, err
	}

	historyRules := cfg.Rules.ForHistory()
	for _, r := range rows {
		gv := GroupView{
			GroupID:  r.layout.GroupID,
			Name:     r.layout.DisplayName,
			Subtitle: r.layout.Subtitle,
			Status:   StatusUnknown,
			History:  make([]Bucket, tl.Count),
		}
		if gv.Name == "" {
			gv.Name = r.info.Name
		}
		if v, ok := current[gv.GroupID]; ok {
			gv.Status, gv.Score = v.Status, roundScore(v.Score)
		}
		hist := history[gv.GroupID]
		for i := 0; i < tl.Count; i++ {
			start := tl.BucketStart(i)
			b := Bucket{Start: start, Status: StatusUnknown}
			// 历史条只用真实流量，不用探测补洞；每格的样本门槛单独配置（通常比当前状态低）
			if sig, ok := hist[start.Unix()]; ok {
				b.Score = roundScore(s.d.Scorer.Score(sig, historyRules))
				b.Status = s.d.Classifier.Classify(b.Score, historyRules)
			}
			gv.History[i] = b
		}
		pv := &snap.Platforms[r.platform]
		pv.Groups = append(pv.Groups, gv)
	}
	return snap, nil
}

// currentVerdicts 判定窗口内的当前状态：被动样本足够则按健康分，否则交给兜底判定，仍无结论则为暂无数据
func (s *Service) currentVerdicts(ctx context.Context, cfg Config, ids []int64, now time.Time, window time.Duration) (map[int64]Verdict, error) {
	start := now.Add(-window)
	sigs, err := s.d.Source.Fetch(ctx, ids, start, now, window)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]Verdict, len(ids))
	lacking := map[int64]int64{}
	for _, id := range ids {
		sig := sigs[id][start.Unix()]
		score := s.d.Scorer.Score(sig, cfg.Rules)
		if score == nil {
			lacking[id] = sig.Requests
			continue
		}
		out[id] = Verdict{Status: s.d.Classifier.Classify(score, cfg.Rules), Score: score}
	}
	if len(lacking) > 0 && s.d.Fallback != nil {
		for id, v := range s.d.Fallback.Judge(ctx, cfg, lacking) {
			out[id] = v
		}
	}
	return out, nil
}

// ─── 管理端 ──────────────────────────────────────────────────────────────

// AdminConfig 管理端读取的配置：编排已与分组目录合并，并附带分组原始信息
type AdminConfig struct {
	Config      Config      `json:"config"`
	Version     int64       `json:"version"`
	Groups      []GroupInfo `json:"groups"`
	NewGroupIDs []int64     `json:"new_group_ids"`
	// ErrorCategories 可选的忽略错误分类（与上游 V2 分类一致）
	ErrorCategories []string `json:"error_categories"`
}

func (s *Service) AdminConfig(ctx context.Context) (*AdminConfig, error) {
	cfg, ver, err := s.d.Config.Load(ctx)
	if err != nil {
		return nil, err
	}
	catalog, err := s.d.Catalog.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	merged := MergeLayout(cfg.Layout, catalog, cfg.AutoShowNewGroups)
	cfg.Layout = merged.Layout
	if catalog == nil {
		catalog = []GroupInfo{}
	}
	return &AdminConfig{Config: cfg, Version: ver, Groups: catalog, NewGroupIDs: merged.NewGroupIDs, ErrorCategories: ErrorCategories}, nil
}

// SaveConfig 校验后乐观锁保存，返回新版本号
func (s *Service) SaveConfig(ctx context.Context, cfg Config, expectVer int64) (int64, error) {
	if err := cfg.Validate(); err != nil {
		return 0, err
	}
	ver, err := s.d.Config.Save(ctx, cfg, expectVer)
	if err != nil {
		return 0, err
	}
	if s.d.OnSaved != nil {
		s.d.OnSaved()
	}
	return ver, nil
}

func (s *Service) invalidate() {
	if s.d.OnSaved != nil {
		s.d.OnSaved()
	}
}

func roundScore(v *float64) *float64 {
	if v == nil {
		return nil
	}
	r := float64(int64(*v + 0.5))
	return &r
}
