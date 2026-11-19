// Package outreach — [plus] 模块装配（Composition Root）
package outreach

import (
	"context"
	"database/sql"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Register 装配触达中心并注册路由，启动发送 worker。
// publicGroup 免登录（点击跟踪、退订）；userGroup 需登录（领取权益）；adminGroup 需管理员。
func Register(
	db *sql.DB, withPoints bool,
	publicGroup, userGroup, adminGroup *gin.RouterGroup,
	emailSvc *service.EmailService, settingSvc *service.SettingService,
	verify Verification, // 邮箱验证模块（nil = 不可用）；本包不依赖它的任何类型
) *Service {
	// 注册表是唯一的扩展点：新增目的 / 权益只在这里注册；指标由目的声明，自动收集。
	benefits := NewBenefitRegistry()
	if withPoints {
		benefits.Register(NewPointsBenefit(NewPointsGranter()))
	}
	purposes := NewPurposeRegistry()
	purposes.Register(NewVerifyEmailPurpose(verifyRewardVars{v: verify}))
	purposes.Register(NewRecallPurpose())
	purposes.Register(NewNewsPurpose())
	purposes.Register(NewPromoPurpose())

	svc := NewService(Deps{
		Repo:     NewRepository(db),
		Settings: NewSettingsStore(db),
		Mailer:   NewMailer(emailSvc),
		Site:     NewSiteInfo(settingSvc),
		Purposes: purposes,
		Benefits: benefits,
		Metrics:  NewMetricRegistry(purposes),
		Vars:     NewVariables(benefits),
		Verify:   verify,
	})
	h := NewHandler(svc)
	h.RegisterPublicRoutes(publicGroup)
	h.RegisterUserRoutes(userGroup)
	h.RegisterAdminRoutes(adminGroup)
	NewWorker(svc).Start(context.Background())
	return svc
}
