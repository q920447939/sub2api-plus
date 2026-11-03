// Package slotmachine — [plus] HTTP 层：概览、摇奖、记录、跑马灯、管理端配置与奖金罐
package slotmachine

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"

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
	sg := g.Group("/slot")
	sg.GET("/status", h.status)
	sg.GET("/overview", h.overview)
	sg.POST("/spin", h.spin)
	sg.GET("/history", h.history)
	sg.GET("/feed", h.feed)
}

// RegisterAdminRoutes 挂载到管理员路由组
func (h *Handler) RegisterAdminRoutes(g *gin.RouterGroup) {
	sg := g.Group("/slot")
	sg.GET("/config", h.getConfig)
	sg.PUT("/config", h.saveConfig)
	sg.POST("/pots/adjust", h.adjustPot)
	sg.GET("/pots/ledger", h.potLedger)
	sg.GET("/spins", h.adminSpins)
	sg.GET("/feed", h.adminFeed)
	sg.GET("/stats", h.adminStats)
}

// ─── 用户端 ──────────────────────────────────────────────────────────────

func (h *Handler) overview(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	ov, err := h.svc.Overview(c.Request.Context(), uid)
	if err != nil {
		internalError(c, err)
		return
	}
	response.Success(c, ov)
}

// status 只返回活动开关，供侧边栏入口轮询（不跑规则链，开销小）
func (h *Handler) status(c *gin.Context) {
	cfg, _, err := h.svc.Config(c.Request.Context())
	if err != nil {
		internalError(c, err)
		return
	}
	response.Success(c, gin.H{"enabled": cfg.Enabled})
}

// spin POST /plus/slot/spin，Header Idempotency-Key 必填（每次摇机生成一次）
func (h *Handler) spin(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	res, err := h.svc.Spin(c.Request.Context(), uid, c.GetHeader("Idempotency-Key"))
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, res)
}

func (h *Handler) history(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	page, size := response.ParsePagination(c)
	list, total, page, size, err := h.svc.History(c.Request.Context(), uid, c.Query("win") == "1", page, size)
	if err != nil {
		internalError(c, err)
		return
	}
	response.Paginated(c, list, total, page, size)
}

func (h *Handler) feed(c *gin.Context) {
	list, err := h.svc.Feed(c.Request.Context())
	if err != nil {
		internalError(c, err)
		return
	}
	response.Success(c, list)
}

// ─── 管理端 ──────────────────────────────────────────────────────────────

func (h *Handler) getConfig(c *gin.Context) {
	st, err := h.svc.AdminState(c.Request.Context())
	if err != nil {
		internalError(c, err)
		return
	}
	response.Success(c, st)
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
	ver, err := h.svc.AdminSaveConfig(c.Request.Context(), req.Config, req.Version)
	if err != nil {
		h.adminFail(c, err)
		return
	}
	response.Success(c, gin.H{"version": ver})
}

func (h *Handler) adjustPot(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	var req struct {
		Pot   string `json:"pot"`
		Delta int64  `json:"delta"`
		Note  string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	bal, err := h.svc.AdminAdjustPot(c.Request.Context(), req.Pot, req.Delta, uid, req.Note)
	if err != nil {
		if errors.Is(err, ErrPotInsufficient) {
			response.BadRequest(c, "奖金罐余额不足，无法抽出这么多")
			return
		}
		h.adminFail(c, err)
		return
	}
	response.Success(c, gin.H{"balance": bal})
}

func (h *Handler) potLedger(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	list, err := h.svc.AdminPotLedger(c.Request.Context(), c.Query("manual") == "1", limit)
	if err != nil {
		internalError(c, err)
		return
	}
	response.Success(c, list)
}

func (h *Handler) adminSpins(c *gin.Context) {
	page, size := response.ParsePagination(c)
	q := SpinQuery{Keyword: c.Query("q"), PrizeType: c.Query("prize_type"), Page: page, PageSize: size}
	if d, err := time.ParseInLocation("2006-01-02", c.Query("date"), timezone.Location()); err == nil {
		q.Since, q.Until = d, d.AddDate(0, 0, 1)
	}
	list, total, page, size, err := h.svc.AdminSpins(c.Request.Context(), q)
	if err != nil {
		internalError(c, err)
		return
	}
	response.Paginated(c, list, total, page, size)
}

func (h *Handler) adminFeed(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	list, err := h.svc.AdminFeed(c.Request.Context(), limit)
	if err != nil {
		internalError(c, err)
		return
	}
	response.Success(c, list)
}

func (h *Handler) adminStats(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	st, err := h.svc.AdminStats(c.Request.Context(), days)
	if err != nil {
		internalError(c, err)
		return
	}
	response.Success(c, st)
}

// ─── 错误映射 ──────────────────────────────────────────────────────────────

func (h *Handler) fail(c *gin.Context, err error) {
	if re, ok := AsRuleError(err); ok {
		status := http.StatusForbidden
		if re.Code == "cooldown" {
			status = http.StatusTooManyRequests
		}
		response.ErrorWithDetails(c, status, re.Message, re.Code, nil)
		return
	}
	switch {
	case errors.Is(err, ErrActivityDisabled):
		response.ErrorWithDetails(c, http.StatusForbidden, "幸运九宫格未开放", "disabled", nil)
	case errors.Is(err, ErrInsufficientPoints):
		response.ErrorWithDetails(c, http.StatusConflict, "积分不足，暂时无法摇奖", "insufficient_points", nil)
	case errors.Is(err, ErrIdempotencyKey):
		response.BadRequest(c, "Idempotency-Key 格式不正确")
	default:
		internalError(c, err)
	}
}

func (h *Handler) adminFail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidConfig):
		response.BadRequest(c, err.Error())
	case errors.Is(err, ErrVersionMismatch):
		response.Error(c, http.StatusConflict, "配置已被其他人修改，请刷新后重试")
	default:
		internalError(c, err)
	}
}

func internalError(c *gin.Context, err error) {
	response.Error(c, http.StatusInternalServerError, "internal error")
	_ = c.Error(err)
}

func userID(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "not authenticated")
		return 0, false
	}
	return subject.UserID, true
}
