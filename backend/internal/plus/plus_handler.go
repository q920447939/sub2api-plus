// plus_handler.go — [plus] HTTP handlers for plus module endpoints.
package plus

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// MenuSettingKey is the upstream settings key holding the custom menu JSON.
const MenuSettingKey = "custom_menu_items"

// Handler holds all plus module dependencies needed for HTTP handling.
type Handler struct {
	deps Deps
}

// NewHandler constructs a Handler from the upstream service pointers.
func NewHandler(apiKey *service.APIKeyService, sub *service.SubscriptionService, settings *service.SettingService) *Handler {
	return &Handler{deps: Deps{APIKey: apiKey, Subscription: sub, Settings: settings}}
}

// menuItem is the subset of a custom menu item the launcher needs.
type menuItem struct {
	ID         string          `json:"id"`
	Label      string          `json:"label"`
	URL        string          `json:"url"`
	Visibility string          `json:"visibility"`
	PlusConfig json.RawMessage `json:"plus_config"`
}

// HandleKeyLaunch handles POST /api/v1/plus/menu/launch.
// Body: { "menu_id": "<custom menu item id>" }
// The config is read from the server-side settings, never from the request body, so a user
// cannot point the launcher at an arbitrary origin.
func (h *Handler) HandleKeyLaunch(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "not authenticated")
		return
	}

	var req struct {
		MenuID string `json:"menu_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.MenuID) == "" {
		response.BadRequest(c, "menu_id is required")
		return
	}

	item, err := h.findMenuItem(c, req.MenuID, subject.UserID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "failed to load menu item")
		return
	}
	if item == nil {
		response.NotFound(c, "menu item not found")
		return
	}
	if !strings.HasPrefix(strings.TrimSpace(item.URL), "plus:key_launch") {
		response.BadRequest(c, "menu item is not a key_launch module")
		return
	}

	cfg, err := ParseKeyLaunchConfig(item.PlusConfig)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "menu item is misconfigured")
		return
	}

	result, err := Launch(c.Request.Context(), h.deps, subject.UserID, cfg)
	if err != nil {
		var blocked *ErrBlocked
		if errors.As(err, &blocked) {
			c.JSON(http.StatusForbidden, gin.H{"blocked": true, "reason": blocked.Reason})
			return
		}
		response.Error(c, http.StatusInternalServerError, "launch failed")
		return
	}
	response.Success(c, result)
}

// findMenuItem loads the custom menu items from settings and returns the matching entry.
// Admin-only entries are hidden from non-admin users, matching the public settings behaviour.
func (h *Handler) findMenuItem(c *gin.Context, menuID string, userID int64) (*menuItem, error) {
	raw, err := service.GetRawSetting(c.Request.Context(), h.deps.Settings, MenuSettingKey)
	if err != nil {
		return nil, err
	}
	var items []menuItem
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &items); err != nil {
		return nil, err
	}

	isAdmin := false
	if role, ok := middleware.GetUserRoleFromContext(c); ok {
		isAdmin = role == "admin"
	}

	for i := range items {
		if items[i].ID != menuID {
			continue
		}
		if items[i].Visibility == "admin" && !isAdmin {
			return nil, nil
		}
		return &items[i], nil
	}
	return nil, nil
}
