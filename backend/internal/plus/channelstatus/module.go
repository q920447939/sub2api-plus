// Package channelstatus — [plus] 模块装配（Composition Root）：把各实现组装成 Service 并注册路由
package channelstatus

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/gin-gonic/gin"
)

// snapshotCacheTTL 聚合表每分钟刷新一次，缓存 30 秒足够
const snapshotCacheTTL = 30 * time.Second

// Register 装配并注册用户端 / 管理端路由
func Register(db *sql.DB, userGroup, adminGroup *gin.RouterGroup) *Service {
	repo := NewRepository(db)
	passive := NewPassiveSource(db, func(ctx context.Context) []string {
		cfg, _, err := repo.Load(ctx)
		if err != nil {
			return nil
		}
		return cfg.Rules.IgnoredCategories
	})
	// 扣除用户侧业务限制错误（余额不足等），与运维监控 SLA 口径对齐
	excluded := NewExclusionSource(passive, NewDBExcludedCounter(db), func(ctx context.Context) bool {
		cfg, _, err := repo.Load(ctx)
		return err == nil && cfg.Rules.ExcludeUserSideErrors
	})
	cached := NewCachedSource(excluded, snapshotCacheTTL)

	// 兜底探测：只在有人查看且被动样本不足时按需触发（见 probe_policy.go）
	prober := NewProber(ProberDeps{
		Store:    repo,
		Keys:     repo,
		Catalog:  repo,
		Runner:   GatewayRunner{},
		Location: timezone.Location(),
	})

	svc := NewService(Deps{
		Config:   repo,
		Catalog:  repo,
		Source:   cached,
		Fallback: prober,
		Location: timezone.Location(),
		OnSaved:  cached.Invalidate,
	})
	h := NewHandler(svc, prober)
	h.RegisterUserRoutes(userGroup)
	h.RegisterAdminRoutes(adminGroup)

	// 自己驱动被动统计，不依赖上游 channel_monitor_enabled 开关
	NewV2Producer(db, repo).Start()
	NewExcludedProducer(db, repo).Start()
	return svc
}
