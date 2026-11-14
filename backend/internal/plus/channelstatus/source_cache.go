// Package channelstatus — [plus] Decorator：给任意 SignalSource 加 TTL 缓存 + singleflight，
// 防止大量用户同时轮询时把查询打到聚合表上。缓存与数据来源完全解耦。
package channelstatus

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type cacheEntry struct {
	data    map[int64]map[int64]Signal
	expires time.Time
}

// CachedSource 缓存装饰器；返回的 map 被多个调用方共享，调用方不得修改
type CachedSource struct {
	inner SignalSource
	ttl   time.Duration
	now   func() time.Time

	mu    sync.Mutex
	items map[string]cacheEntry
	sf    singleflight.Group
}

func NewCachedSource(inner SignalSource, ttl time.Duration) *CachedSource {
	return &CachedSource{inner: inner, ttl: ttl, now: time.Now, items: map[string]cacheEntry{}}
}

var _ SignalSource = (*CachedSource)(nil)

func (c *CachedSource) Fetch(ctx context.Context, groupIDs []int64, start, end time.Time, bucket time.Duration) (map[int64]map[int64]Signal, error) {
	key := cacheKey(groupIDs, start, end, bucket)
	now := c.now()

	c.mu.Lock()
	if e, ok := c.items[key]; ok && now.Before(e.expires) {
		c.mu.Unlock()
		return e.data, nil
	}
	c.mu.Unlock()

	v, err, _ := c.sf.Do(key, func() (any, error) {
		data, err := c.inner.Fetch(ctx, groupIDs, start, end, bucket)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.evictExpired(now)
		c.items[key] = cacheEntry{data: data, expires: now.Add(c.ttl)}
		c.mu.Unlock()
		return data, nil
	})
	if err != nil {
		return nil, err
	}
	m, _ := v.(map[int64]map[int64]Signal)
	return m, nil
}

// Invalidate 清空缓存（配置变更后调用，使忽略分类等立即生效）
func (c *CachedSource) Invalidate() {
	c.mu.Lock()
	c.items = map[string]cacheEntry{}
	c.mu.Unlock()
}

func (c *CachedSource) evictExpired(now time.Time) {
	for k, e := range c.items {
		if !now.Before(e.expires) {
			delete(c.items, k)
		}
	}
}

func cacheKey(groupIDs []int64, start, end time.Time, bucket time.Duration) string {
	ids := append([]int64(nil), groupIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var b strings.Builder
	fmt.Fprintf(&b, "%d|%d|%d|", start.Unix(), end.Unix(), int64(bucket))
	for _, id := range ids {
		fmt.Fprintf(&b, "%d,", id)
	}
	return b.String()
}
