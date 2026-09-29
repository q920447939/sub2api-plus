// plus_routes.go — [plus] registers plus module routes onto the main router.
// Called with one line from backend/internal/server/router.go — the only upstream touch.
package plus

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes attaches all plus-module endpoints under /api/v1/plus.
// The jwtAuth middleware ensures the user is authenticated before any plus endpoint runs.
func RegisterRoutes(
	v1 *gin.RouterGroup,
	jwtAuth gin.HandlerFunc,
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
}
