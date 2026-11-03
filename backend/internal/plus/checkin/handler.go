// Package checkin — [plus] HTTP 层：签到与状态查询
package checkin

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// Handler HTTP 层
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterUserRoutes 挂载到已鉴权用户路由组
func (h *Handler) RegisterUserRoutes(g *gin.RouterGroup) {
	cg := g.Group("/checkin")
	cg.POST("", h.checkin)
	cg.GET("/status", h.status)
}

// RegisterAdminRoutes 挂载到管理员路由组
func (h *Handler) RegisterAdminRoutes(g *gin.RouterGroup) {
	cg := g.Group("/checkin")
	cg.GET("/config", h.getConfig)
	cg.PUT("/config", h.saveConfig)
	cg.GET("/logs", h.adminLogs)
	cg.GET("/stats", h.adminStats)
}

// ─── 用户端 ──────────────────────────────────────────────────────────────

// checkin POST /plus/checkin
func (h *Handler) checkin(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "not authenticated")
		return
	}
	ip := c.ClientIP()
	res, err := h.svc.Checkin(c.Request.Context(), subject.UserID, ip)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, res)
}

// status GET /plus/checkin/status
func (h *Handler) status(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "not authenticated")
		return
	}
	st, err := h.svc.Status(c.Request.Context(), subject.UserID, c.Query("month"))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "internal error")
		_ = c.Error(err)
		return
	}
	response.Success(c, st)
}

// ─── 管理端 ──────────────────────────────────────────────────────────────

func (h *Handler) getConfig(c *gin.Context) {
	cfg, ver, err := h.svc.repo.LoadConfig(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "internal error")
		_ = c.Error(err)
		return
	}
	response.Success(c, gin.H{"config": cfg, "version": ver})
}

func (h *Handler) saveConfig(c *gin.Context) {
	var req struct {
		Config  Config `json:"config"`
		Version int64  `json:"version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	ver, err := h.svc.SaveConfig(c.Request.Context(), req.Config, req.Version)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidConfig):
			response.BadRequest(c, err.Error())
		case errors.Is(err, ErrVersionMismatch):
			response.Error(c, http.StatusConflict, "配置已被其他人修改，请刷新后重试")
		default:
			response.Error(c, http.StatusInternalServerError, "internal error")
			_ = c.Error(err)
		}
		return
	}
	response.Success(c, gin.H{"version": ver})
}

func (h *Handler) adminLogs(c *gin.Context) {
	page, size := response.ParsePagination(c)
	list, total, page, size, err := h.svc.AdminLogs(c.Request.Context(), LogQuery{
		Date: c.Query("date"), Keyword: c.Query("q"), Page: page, PageSize: size,
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "internal error")
		_ = c.Error(err)
		return
	}
	response.Paginated(c, list, total, page, size)
}

func (h *Handler) adminStats(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	st, err := h.svc.AdminStats(c.Request.Context(), days)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "internal error")
		_ = c.Error(err)
		return
	}
	response.Success(c, st)
}

// ─── 错误映射 ──────────────────────────────────────────────────────────────

func (h *Handler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrActivityDisabled):
		response.Error(c, http.StatusForbidden, "签到活动未开启")
	case errors.Is(err, ErrAlreadyCheckedIn):
		response.Error(c, http.StatusConflict, "今日已签到")
	case errors.Is(err, ErrIPLimitExceeded):
		response.Error(c, http.StatusTooManyRequests, "当前网络签到账号过多，请稍后再试")
	default:
		response.Error(c, http.StatusInternalServerError, "internal error")
		_ = c.Error(err)
	}
}
