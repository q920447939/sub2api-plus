package lottery

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// Handler 刮刮乐 HTTP 层：只负责参数解析与错误映射
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterUserRoutes 挂载到已鉴权的用户路由组
func (h *Handler) RegisterUserRoutes(g *gin.RouterGroup) {
	lg := g.Group("/lottery")
	lg.GET("/overview", h.overview)
	lg.POST("/draw", h.draw)
	lg.GET("/feed", h.feed)
	lg.GET("/leaderboard", h.leaderboard)
	lg.GET("/my-draws", h.myDraws)
	lg.GET("/redeem-grant", h.redeemGrant)
}

// RegisterAdminRoutes 挂载到管理员路由组
func (h *Handler) RegisterAdminRoutes(g *gin.RouterGroup) {
	lg := g.Group("/lottery")
	lg.GET("/config", h.getConfig)
	lg.PUT("/config", h.saveConfig)
	lg.GET("/stats", h.stats)
	lg.GET("/draws", h.draws)
}

// ─── 用户端 ──────────────────────────────────────────────────────────────

func (h *Handler) overview(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	ov, err := h.svc.Overview(c.Request.Context(), uid)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, ov)
}

// draw POST /lottery/draw
// Header: Idempotency-Key（必填，前端每张卡生成一次）；Body: {"chance_id": 0}
func (h *Handler) draw(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	var req struct {
		ChanceID int64 `json:"chance_id"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "invalid request body")
			return
		}
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	res, err := h.svc.Draw(c.Request.Context(), uid, req.ChanceID, key)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, res)
}

func (h *Handler) feed(c *gin.Context) {
	list, err := h.svc.Feed(c.Request.Context(), intQuery(c, "limit", 30))
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, list)
}

func (h *Handler) leaderboard(c *gin.Context) {
	list, err := h.svc.Leaderboard(c.Request.Context(), intQuery(c, "limit", 10))
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, list)
}

func (h *Handler) myDraws(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	list, err := h.svc.MyDraws(c.Request.Context(), uid, intQuery(c, "limit", 50))
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, list)
}

// redeemGrant GET /lottery/redeem-grant
// 兑换成功后前端调用，返回本人最近一次兑换的卡密带来的刮刮卡数，以及当前剩余张数。
func (h *Handler) redeemGrant(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	n, err := h.svc.LatestRedeemGrant(c.Request.Context(), uid)
	if err != nil {
		h.fail(c, err)
		return
	}
	ov, err := h.svc.Overview(c.Request.Context(), uid)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"granted": n, "remaining": ov.Remaining})
}

// ─── 管理端 ──────────────────────────────────────────────────────────────

func (h *Handler) getConfig(c *gin.Context) {
	cfg, ver, err := h.svc.repo.LoadConfig(c.Request.Context())
	if err != nil {
		h.fail(c, err)
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
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"version": ver})
}

func (h *Handler) stats(c *gin.Context) {
	day, err := h.svc.ParseDay(c.Query("day"))
	if err != nil {
		response.BadRequest(c, "day must be YYYY-MM-DD")
		return
	}
	st, err := h.svc.AdminStats(c.Request.Context(), day)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, st)
}

func (h *Handler) draws(c *gin.Context) {
	day, err := h.svc.ParseDay(c.Query("day"))
	if err != nil {
		response.BadRequest(c, "day must be YYYY-MM-DD")
		return
	}
	page, size := intQuery(c, "page", 1), intQuery(c, "page_size", 20)
	list, total, err := h.svc.AdminDraws(c.Request.Context(), day, int64(intQuery(c, "user_id", 0)), page, size)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"items": list, "total": total, "page": page, "page_size": size})
}

// ─── 工具 ──────────────────────────────────────────────────────────────

// fail 领域错误映射为 HTTP 状态码；未知错误统一 500，不泄露内部信息
func (h *Handler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrActivityDisabled):
		response.Error(c, http.StatusForbidden, "活动未开启")
	case errors.Is(err, ErrNoChancesLeft):
		response.Error(c, http.StatusConflict, "没有可用的刮刮卡")
	case errors.Is(err, ErrChanceNotFound):
		response.NotFound(c, "刮刮卡不存在")
	case errors.Is(err, ErrChanceAlreadyUsed):
		response.Error(c, http.StatusConflict, "该刮刮卡已使用")
	case errors.Is(err, ErrChanceExpired):
		response.Error(c, http.StatusConflict, "该刮刮卡已过期")
	case errors.Is(err, ErrInvalidIdempotency):
		response.BadRequest(c, err.Error())
	case errors.Is(err, ErrVersionMismatch):
		response.Error(c, http.StatusConflict, "配置已被其他人修改，请刷新后重试")
	case isConfigError(err):
		response.BadRequest(c, err.Error())
	default:
		response.Error(c, http.StatusInternalServerError, "internal error")
		_ = c.Error(err)
	}
}

func isConfigError(err error) bool {
	for _, e := range []error{ErrInvalidChanceTiers, ErrInvalidPrizePool, ErrPoolExceedsPromo, ErrInvalidRiskControl, ErrInvalidVirtualFeed, ErrInvalidNotesKeyword} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

func userID(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "not authenticated")
		return 0, false
	}
	return subject.UserID, true
}

func intQuery(c *gin.Context, key string, def int) int {
	v, err := strconv.Atoi(c.Query(key))
	if err != nil {
		return def
	}
	return v
}
