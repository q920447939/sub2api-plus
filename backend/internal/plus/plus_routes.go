// plus_routes.go — [plus] registers plus module routes onto the main router.
// Called with one line from backend/internal/server/router.go — the only upstream touch.
package plus

import (
	"context"
	"log"

	"github.com/Wei-Shaw/sub2api/internal/plus/lottery"
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

	registerLottery(v1, plus, adminAuth, apiKey, sub)
}

// registerLottery wires the scratch-card module. A DB wiring failure only disables the module.
func registerLottery(v1, userGroup *gin.RouterGroup, adminAuth gin.HandlerFunc, apiKey *service.APIKeyService, sub *service.SubscriptionService) {
	db, err := service.PlusSQLDB(sub)
	if err != nil {
		log.Printf("[plus.lottery] disabled: %v", err)
		return
	}
	svc := lottery.NewService(lottery.NewRepository(db), service.PlusBalanceInvalidator{Sub: sub, APIKey: apiKey})
	h := lottery.NewHandler(svc)
	h.RegisterUserRoutes(userGroup)

	admin := v1.Group("/plus/admin")
	admin.Use(adminAuth)
	h.RegisterAdminRoutes(admin)

	lottery.NewWorker(svc).Start(context.Background())
}
