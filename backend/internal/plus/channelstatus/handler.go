// Package channelstatus — [plus] HTTP 层
package channelstatus

import (
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc    *Service
	prober *Prober
}

func NewHandler(svc *Service, prober *Prober) *Handler { return &Handler{svc: svc, prober: prober} }

// RegisterUserRoutes 挂载到已鉴权用户路由组（/plus）
func (h *Handler) RegisterUserRoutes(g *gin.RouterGroup) {
	cg := g.Group("/channel-status")
	cg.GET("", h.snapshot)
	cg.GET("/meta", h.meta)
}

// RegisterAdminRoutes 挂载到管理员路由组（/plus/admin）
func (h *Handler) RegisterAdminRoutes(g *gin.RouterGroup) {
	cg := g.Group("/channel-status")
	cg.GET("/config", h.getConfig)
	cg.PUT("/config", h.saveConfig)
	cg.GET("/probe/keys", h.probeKeys)
	cg.GET("/probe/states", h.probeStates)
	cg.POST("/probe/run", h.probeRun)
}

// snapshot GET /plus/channel-status?granularity=hour|day|week
func (h *Handler) snapshot(c *gin.Context) {
	snap, err := h.svc.Snapshot(c.Request.Context(), c.Query("granularity"))
	if errors.Is(err, ErrDisabled) {
		response.NotFound(c, "channel status disabled")
		return
	}
	if err != nil {
		h.internal(c, err)
		return
	}
	response.Success(c, snap)
}

// meta GET /plus/channel-status/meta
func (h *Handler) meta(c *gin.Context) {
	m, err := h.svc.Meta(c.Request.Context())
	if err != nil {
		h.internal(c, err)
		return
	}
	response.Success(c, m)
}

// getConfig GET /plus/admin/channel-status/config
func (h *Handler) getConfig(c *gin.Context) {
	cfg, err := h.svc.AdminConfig(c.Request.Context())
	if err != nil {
		h.internal(c, err)
		return
	}
	response.Success(c, cfg)
}

type saveConfigReq struct {
	Config  Config `json:"config"`
	Version int64  `json:"version"`
}

// saveConfig PUT /plus/admin/channel-status/config
func (h *Handler) saveConfig(c *gin.Context) {
	var req saveConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	ver, err := h.svc.SaveConfig(c.Request.Context(), req.Config, req.Version)
	switch {
	case errors.Is(err, ErrInvalidConfig):
		response.BadRequest(c, err.Error())
		return
	case errors.Is(err, ErrVersionMismatch):
		response.Error(c, http.StatusConflict, "配置已被他人修改，请刷新后重试")
		return
	case err != nil:
		h.internal(c, err)
		return
	}
	response.Success(c, gin.H{"version": ver})
}

// probeKeys GET /plus/admin/channel-status/probe/keys —— 当前管理员自己的可用 Key
func (h *Handler) probeKeys(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "not authenticated")
		return
	}
	keys, err := h.prober.Keys(c.Request.Context(), subject.UserID)
	if err != nil {
		h.internal(c, err)
		return
	}
	response.Success(c, keys)
}

// probeStates GET /plus/admin/channel-status/probe/states —— 各分组最近一次探测
func (h *Handler) probeStates(c *gin.Context) {
	groups, err := h.svc.d.Catalog.ListGroups(c.Request.Context())
	if err != nil {
		h.internal(c, err)
		return
	}
	ids := make([]int64, len(groups))
	for i, g := range groups {
		ids[i] = g.ID
	}
	states, err := h.prober.States(c.Request.Context(), ids)
	if err != nil {
		h.internal(c, err)
		return
	}
	list := make([]*ProbeState, 0, len(states))
	for _, st := range states {
		list = append(list, st)
	}
	response.Success(c, list)
}

type probeRunReq struct {
	Probe Probe      `json:"probe"` // 用页面上的草稿（未保存也能测）
	Group GroupProbe `json:"group"`
}

// probeRun POST /plus/admin/channel-status/probe/run —— 立即探测，跳过冷却与每日上限
func (h *Handler) probeRun(c *gin.Context) {
	var req probeRunReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Group.GroupID <= 0 {
		response.BadRequest(c, "invalid request body")
		return
	}
	res, err := h.prober.RunNow(c.Request.Context(), req.Probe, req.Group)
	if errors.Is(err, ErrInvalidConfig) {
		response.BadRequest(c, err.Error())
		return
	}
	if err != nil {
		h.internal(c, err)
		return
	}
	h.svc.invalidate()
	response.Success(c, gin.H{"ok": res.OK, "latency_ms": res.LatencyMs, "message": res.Message})
}

func (h *Handler) internal(c *gin.Context, err error) {
	response.Error(c, http.StatusInternalServerError, "internal error")
	_ = c.Error(err)
}
