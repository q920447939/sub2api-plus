package dispatch

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/plus/dispatch/hook"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Register 装配（Composition Root）：注册检测器、启动后台任务、挂管理端路由、把钩子接到上游选号。
func Register(db *sql.DB, admin *gin.RouterGroup) {
	ctx := context.Background()
	repo := NewRepository(db)
	llm := newIQCompleter()
	library := &FingerprintLibrary{}

	// 检测器注册表：新增一种检测办法只需要在这里加一行
	registry := NewRegistry(
		TrafficDetector{},
		&FingerprintDetector{LLM: llm, Library: library},
		GradedDetector{LLM: llm},
		ArchiveDetector{LLM: llm},
		FieldDetector{},
	)

	catalog := SQLCatalog{DB: db}
	trafficStore := SQLTraffic{DB: db}
	trafficLoader := &TrafficLoader{Store: trafficStore}
	snap := &Snapshotter{Catalog: catalog, Traffic: trafficLoader, TTL: time.Minute}
	planner := Planner{Registry: registry}
	audit := Auditor{Store: repo}

	// 执行层：钩子引擎 + 规则刷新
	engine := hook.New()
	runner := &hook.Runner{Engine: engine, Loader: repo, Sink: repo}
	runner.Start(ctx)
	service.SetPlusDispatchFilter(engine)

	exec := &Executor{Runs: repo, Setup: repo, Registry: registry, Channel: DirectChannel{Catalog: catalog}}
	sched := &Scheduler{Config: repo, Setup: repo, Runs: repo, Planner: planner, Snap: snap, Exec: exec}
	eval := &Evaluator{Config: repo, Setup: repo, Runs: repo, Verdicts: repo, Events: repo, Registry: registry, Planner: planner,
		Snap: snap, Traffic: trafficLoader}
	boards := BoardLoader{Config: repo, Setup: repo, Verdicts: repo, Events: repo, Adjust: repo, Snap: snap, Registry: registry}

	setup := &SetupService{Config: repo, Setup: repo, Registry: registry, Planner: planner, Snap: snap, Sched: sched, Audit: audit,
		OnChange: func() {
			runner.Reload(context.Background())
			snap.Invalidate()
			if err := eval.Run(context.Background()); err != nil {
				log.Printf("[plus.dispatch] evaluate after change: %v", err)
			}
		}}
	query := &QueryService{Boards: boards, Runs: repo, Verdicts: repo, EventDB: repo, Adjust: repo, Traffic: trafficStore,
		Registry: registry, Planner: planner, Eval: eval, Library: library}
	adjust := &AdjustService{Boards: boards, Store: repo, Events: repo, Traffic: trafficStore, DB: db, Audit: audit,
		Reload: func() { runner.Reload(context.Background()) }}

	(&Handler{Setup: setup, Query: query, Adjust: adjust, Audit: repo, Eval: eval}).Register(admin)

	sched.Start(ctx)
	eval.Start(ctx)
	log.Printf("[plus.dispatch] 上游调度台已启动（%d 种检测器）", len(registry.Specs()))
}

var _ service.PlusDispatchFilter = (*hook.Engine)(nil)
