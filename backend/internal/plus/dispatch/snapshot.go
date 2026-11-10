package dispatch

import (
	"context"
	"sync"
	"time"
)

// Snapshot 一次读取的目录 + 流量（各用例共用，避免每个请求都扫流量表）
type Snapshot struct {
	At      time.Time
	Catalog *Catalog
	Traffic *TrafficSet
}

// Snapshotter 带过期时间的快照缓存
type Snapshotter struct {
	Catalog CatalogSource
	Traffic *TrafficLoader
	TTL     time.Duration

	mu   sync.Mutex
	last *Snapshot
}

// Get 返回不超过 TTL 的快照；fresh=true 强制重新读取
func (s *Snapshotter) Get(ctx context.Context, fresh bool) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ttl := s.TTL
	if ttl <= 0 {
		ttl = time.Minute
	}
	if !fresh && s.last != nil && time.Since(s.last.At) < ttl {
		return s.last, nil
	}
	now := time.Now()
	cat, err := s.Catalog.LoadCatalog(ctx)
	if err != nil {
		return nil, err
	}
	tr, err := s.Traffic.Load(ctx, now)
	if err != nil {
		return nil, err
	}
	s.last = &Snapshot{At: now, Catalog: cat, Traffic: tr}
	return s.last, nil
}

// Invalidate 配置变更后让下一次读取重新加载
func (s *Snapshotter) Invalidate() {
	s.mu.Lock()
	s.last = nil
	s.mu.Unlock()
}
