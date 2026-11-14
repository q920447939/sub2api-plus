package dispatch

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/plus/channelstatus"
	"github.com/Wei-Shaw/sub2api/internal/plus/dispatch/hook"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Register 装配（Composition Root）：注册检测器、启动后台任务、挂管理端路由、把钩子接到上游选号。
func Register(db *sql.DB, v1, admin *gin.RouterGroup) {
	ctx := context.Background()
	repo := NewRepository(db)
	llm := newIQCompleter()
	library := &FingerprintLibrary{}

	// 检测器注册表：新增一种检测办法只需要在这里加一行
	registry := NewRegistry(
		&FingerprintDetector{LLM: llm, Library: library},
		GradedDetector{LLM: llm},
		ArchiveDetector{LLM: llm},
	)

	catalog := SQLCatalog{DB: db}
	traffic := &TrafficLoader{Store: SQLTraffic{DB: db}}
	snap := &Snapshotter{Catalog: catalog, Traffic: traffic, TTL: time.Minute}
	planner := Planner{Registry: registry}

	// 执行层：钩子引擎 + 规则刷新
	engine := hook.New()
	runner := &hook.Runner{Engine: engine, Loader: repo}
	runner.Start(ctx)
	service.SetPlusDispatchFilter(dispatchFilter{engine})

	detect := &DetectionLoader{Config: repo, Setup: repo, Runs: repo, Ignores: repo, Reg: registry}
	exec := &Executor{Runs: repo, Setup: repo, Registry: registry, Channel: DirectChannel{Catalog: catalog},
		OnFinish: detect.Invalidate}
	sched := &Scheduler{Config: repo, Setup: repo, Runs: repo, Planner: planner, Snap: snap, Exec: exec, Audit: repo}

	preview := &Previewer{Store: repo}
	svc := &Service{Config: repo, Setup: repo, Runs: repo, Ignores: repo, Rules: repo, AuditDB: repo,
		Channel: channelstatus.NewRepository(db), Registry: registry, Planner: planner, Snap: snap, Detect: detect, Sched: sched,
		Library: library, Preview: preview, OnRules: runner.Wake, LastSeen: lastSeenQuery(db)}
	(&Handler{S: svc}).Register(admin)
	preview.Register(v1)

	traffic.Ignored = func(ctx context.Context) []string {
		cfg, _, err := repo.LoadConfig(ctx)
		if err != nil {
			cfg = DefaultConfig()
		}
		return svc.healthRules(ctx, cfg).IgnoredCategories
	}

	sched.Start(ctx)
	library.Start(ctx)
	go refreshTraffic(ctx, traffic, snap)
	log.Printf("[plus.dispatch] 上游调度台已启动（%d 种检测方式）", len(registry.Specs()))
}

// dispatchFilter 把钩子引擎接到上游选号（service 和 hook 互不依赖，在这里转一下请求结构）
type dispatchFilter struct{ *hook.Engine }

func (f dispatchFilter) FilterAccounts(ctx context.Context, q service.PlusDispatchQuery, ids []int64, supports []bool) ([]bool, bool) {
	return f.Engine.FilterAccounts(ctx, hook.Request{GroupID: q.GroupID, Model: q.Model, Key: q.Key, Bound: q.Bound}, ids, supports)
}

var _ service.PlusDispatchFilter = dispatchFilter{}

// refreshTraffic 每 2 分钟重算最近两个小时的流量小时桶（第一次回填 8 天）
func refreshTraffic(ctx context.Context, tl *TrafficLoader, snap *Snapshotter) {
	t := time.NewTicker(2 * time.Minute)
	defer t.Stop()
	for {
		if err := tl.Refresh(ctx, time.Now()); err != nil {
			log.Printf("[plus.dispatch] refresh traffic: %v", err)
		} else {
			snap.Invalidate()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// lastSeenQuery 各账号近 2 小时内最近一次请求的时间
func lastSeenQuery(db *sql.DB) func(ctx context.Context) map[int64]time.Time {
	return func(ctx context.Context) map[int64]time.Time {
		out := map[int64]time.Time{}
		rows, err := db.QueryContext(ctx, `SELECT account_id, MAX(created_at) FROM usage_logs
			WHERE created_at >= NOW() - INTERVAL '2 hours' AND account_id > 0 GROUP BY 1`)
		if err != nil {
			log.Printf("[plus.dispatch] last seen: %v", err)
			return out
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var (
				id int64
				at time.Time
			)
			if rows.Scan(&id, &at) == nil {
				out[id] = at
			}
		}
		return out
	}
}
