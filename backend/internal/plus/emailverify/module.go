// Package emailverify — [plus] 模块装配（Composition Root）
package emailverify

import (
	"context"
	"database/sql"
	"log"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Register 装配邮箱验证模块、注册路由，并订阅上游的「邮箱已验证」事件。
// withPoints=false 时不发奖励（积分模块不可用）。
func Register(
	db *sql.DB,
	withPoints bool,
	publicGroup, userGroup, adminGroup *gin.RouterGroup,
	userSvc *service.UserService,
	emailSvc *service.EmailService,
	settingSvc *service.SettingService,
) *Service {
	repo := NewRepository(db)
	deps := Deps{
		Repo:       repo,
		DB:         db,
		Config:     NewConfigStore(db),
		Users:      NewUserReader(userSvc),
		CodeSender: NewCodeSender(emailSvc),
		SiteName:   NewSiteNameReader(settingSvc),
	}
	if withPoints {
		deps.Rewarder = NewRewarder()
	}
	svc := NewService(deps)

	h := NewHandler(svc)
	h.RegisterUserRoutes(userGroup)
	h.RegisterPublicRoutes(publicGroup)
	h.RegisterAdminRoutes(adminGroup)

	// 上游注册 / 绑定邮箱 / 第三方登录时已经验证过邮箱：补记验证记录
	service.SetPlusEmailVerifiedHook(func(ctx context.Context, ev service.PlusEmailVerifiedEvent) {
		if err := svc.RecordExternal(ctx, ev.UserID, ev.Email, ev.Source); err != nil {
			log.Printf("[plus.emailverify] record %s verification for user %d: %v", ev.Source, ev.UserID, err)
		}
	})

	go func() {
		if err := repo.PurgeExpiredTokens(context.Background()); err != nil {
			log.Printf("[plus.emailverify] purge expired tokens: %v", err)
		}
	}()
	return svc
}
