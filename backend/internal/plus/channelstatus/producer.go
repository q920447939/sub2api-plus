// Package channelstatus — [plus] 被动统计的数据生产者。
//
// 渠道状态读取上游 V2 的预聚合表（channel_monitor_v2_*）。上游聚合器只在
// channel_monitor_enabled=true 且 mode=v2 时运行，而打开这个开关会同时露出上游的
// 「渠道状态」菜单和页面。所以这里由 plus 自己起一个上游聚合器实例：
//   - 只读调用上游导出的构造函数和仓储（Adapter），不改上游任何代码；
//   - 开关读取器换成 plus 的实现，只看新渠道状态是否开启，与上游开关解耦；
//   - 上游聚合器每轮都会抢同一把分布式 leader 锁，即便有人误开了上游 V2，
//     两个实例也不会重复计算。
//
// 以后若改为 plus 自有的统计表，只需新增一个 Producer 实现并在 module.go 替换。
package channelstatus

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Producer 被动统计数据的生产者（后台常驻）
type Producer interface {
	Start()
	Stop()
}

// enabledReader 读取「新渠道状态是否开启」，带短 TTL 缓存，避免聚合循环每轮都查库
type enabledReader struct {
	store ConfigStore
	ttl   time.Duration

	mu      sync.Mutex
	value   bool
	fetched time.Time
}

func (r *enabledReader) enabled(ctx context.Context) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.fetched.IsZero() && time.Since(r.fetched) < r.ttl {
		return r.value
	}
	cfg, _, err := r.store.Load(ctx)
	if err == nil {
		r.value = cfg.Enabled
		r.fetched = time.Now()
	}
	// 读取失败沿用上次结果（首次失败为关闭），下一轮重试
	return r.value
}

// plusRuntime 实现上游聚合器所需的开关读取接口（GetChannelMonitorRuntime）。
// 新渠道状态开启 → 告诉聚合器「已开启且为 V2 模式」；否则关闭，聚合器空转。
type plusRuntime struct{ r *enabledReader }

func (p plusRuntime) GetChannelMonitorRuntime(ctx context.Context) service.ChannelMonitorRuntime {
	if !p.r.enabled(ctx) {
		return service.ChannelMonitorRuntime{}
	}
	return service.ChannelMonitorRuntime{Enabled: true, Mode: service.ChannelMonitorModeV2}
}

// NewV2Producer 用上游 V2 聚合器作为数据生产者
func NewV2Producer(db *sql.DB, store ConfigStore) Producer {
	reader := &enabledReader{store: store, ttl: 30 * time.Second}
	return service.NewChannelMonitorV2Aggregator(
		repository.NewChannelMonitorV2Repository(db),
		db,
		plusRuntime{r: reader},
	)
}
