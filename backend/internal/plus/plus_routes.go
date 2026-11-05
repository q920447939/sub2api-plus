// plus_routes.go — [plus] registers plus module routes onto the main router.
// Called with one line from backend/internal/server/router.go — the only upstream touch.
package plus

import (
	"context"
	"log"

	"github.com/Wei-Shaw/sub2api/internal/plus/channelstatus"
	"github.com/Wei-Shaw/sub2api/internal/plus/checkin"
	"github.com/Wei-Shaw/sub2api/internal/plus/iq"
	"github.com/Wei-Shaw/sub2api/internal/plus/points"
	"github.com/Wei-Shaw/sub2api/internal/plus/slotmachine"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes attaches all plus-module endpoints under /api/v1/plus.
// The jwtAuth middleware ensures the user is authenticated before any plus endpoint runs.
func RegisterRoutes(
	v1 *gin.RouterGroup,
	jwtAuth gin.HandlerFunc,
	adminAuth gin.HandlerFunc,
	apiKey *service.APIKeyService,
	sub *service.SubscriptionService,
	settings *service.SettingService,
) {
	h := NewHandler(apiKey, sub, settings)

	plus := v1.Group("/plus")
	plus.Use(jwtAuth)
	{
		// POST /api/v1/plus/menu/launch  Body: { "menu_id": "<id>" }
		plus.POST("/menu/launch", h.HandleKeyLaunch)
	}

	// 积分乐园：积分、签到、幸运九宫格共用同一个 DB 连接；points 失败则全部跳过
	pointsSvc := registerPoints(v1, plus, adminAuth, sub, apiKey)
	if pointsSvc != nil {
		checkinSvc := registerCheckin(v1, plus, adminAuth, sub, pointsSvc)
		registerSlot(v1, plus, adminAuth, sub, apiKey, pointsSvc, checkinSvc)
	}

	registerChannelStatus(v1, plus, adminAuth, sub)
	registerIQ(v1, plus, adminAuth, sub, apiKey)
}

// registerIQ wires 模型智商检测（鹈鹕画图画廊 + 定时检测）。
func registerIQ(v1, userGroup *gin.RouterGroup, adminAuth gin.HandlerFunc, sub *service.SubscriptionService, apiKey *service.APIKeyService) {
	db, err := service.PlusSQLDB(sub)
	if err != nil {
		log.Printf("[plus.iq] disabled: %v", err)
		return
	}
	admin := v1.Group("/plus/admin")
	admin.Use(adminAuth)
	iq.Register(db, apiKey, v1, userGroup, admin)
}

// registerChannelStatus wires 渠道状态（读上游 V2 预聚合表，独立配置）。
func registerChannelStatus(v1, userGroup *gin.RouterGroup, adminAuth gin.HandlerFunc, sub *service.SubscriptionService) {
	db, err := service.PlusSQLDB(sub)
	if err != nil {
		log.Printf("[plus.channelstatus] disabled: %v", err)
		return
	}
	admin := v1.Group("/plus/admin")
	admin.Use(adminAuth)
	channelstatus.Register(db, userGroup, admin)
}

// registerPoints wires the points module. Returns nil on failure (disables dependents).
func registerPoints(v1, userGroup *gin.RouterGroup, adminAuth gin.HandlerFunc, sub *service.SubscriptionService, _ *service.APIKeyService) *points.Service {
	db, err := service.PlusSQLDB(sub)
	if err != nil {
		log.Printf("[plus.points] disabled: %v", err)
		return nil
	}
	svc := points.NewService(points.NewRepository(db))
	h := points.NewHandler(svc)
	h.RegisterUserRoutes(userGroup)

	admin := v1.Group("/plus/admin")
	admin.Use(adminAuth)
	h.RegisterAdminRoutes(admin)

	points.NewWorker(svc).Start(context.Background())

	// 兑换卡密送积分：上游兑换事务提交后同步回调，失败只记日志，由 points worker 定时补偿
	service.SetPlusRedeemHook(func(ctx context.Context, ev service.PlusRedeemEvent) {
		if err := svc.HandleRedeem(ctx, points.RedeemEvent{
			UserID: ev.UserID, CodeID: ev.CodeID, Code: ev.Code, Type: ev.Type, Value: ev.Value, Source: ev.Source,
		}); err != nil {
			log.Printf("[plus.points] redeem bonus for code %d: %v", ev.CodeID, err)
		}
	})
	return svc
}

// registerCheckin wires the daily check-in module.
func registerCheckin(v1, userGroup *gin.RouterGroup, adminAuth gin.HandlerFunc, sub *service.SubscriptionService, pointsSvc *points.Service) *checkin.Service {
	db, err := service.PlusSQLDB(sub)
	if err != nil {
		log.Printf("[plus.checkin] disabled: %v", err)
		return nil
	}
	svc := checkin.NewService(checkin.NewRepository(db), pointsSvc)
	pointsSvc.RegisterEarnWay(svc.EarnWay)
	h := checkin.NewHandler(svc)
	h.RegisterUserRoutes(userGroup)

	admin := v1.Group("/plus/admin")
	admin.Use(adminAuth)
	h.RegisterAdminRoutes(admin)
	return svc
}

// registerSlot wires the 幸运九宫格 module. checkinSvc 提供连签加成，为 nil 时不启用加成。
func registerSlot(v1, userGroup *gin.RouterGroup, adminAuth gin.HandlerFunc, sub *service.SubscriptionService, apiKey *service.APIKeyService, pointsSvc *points.Service, checkinSvc *checkin.Service) {
	db, err := service.PlusSQLDB(sub)
	if err != nil {
		log.Printf("[plus.slotmachine] disabled: %v", err)
		return
	}
	var boost slotmachine.StreakBoostSource
	if checkinSvc != nil {
		boost = checkinSvc
	}
	svc := slotmachine.NewService(slotmachine.NewRepository(db), pointsSvc, service.PlusBalanceInvalidator{Sub: sub, APIKey: apiKey}, boost)
	h := slotmachine.NewHandler(svc)
	h.RegisterUserRoutes(userGroup)

	admin := v1.Group("/plus/admin")
	admin.Use(adminAuth)
	h.RegisterAdminRoutes(admin)

	slotmachine.NewWorker(svc).Start(context.Background())
}
