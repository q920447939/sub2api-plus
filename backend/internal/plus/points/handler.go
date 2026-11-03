// Package points — [plus] HTTP 层：积分账户概览、流水查询、管理端配置
package points

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// Handler HTTP 层，只负责参数解析与错误映射
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterUserRoutes 挂载到已鉴权用户路由组
func (h *Handler) RegisterUserRoutes(g *gin.RouterGroup) {
	pg := g.Group("/points")
	pg.GET("/overview", h.overview)
	pg.GET("/ledger", h.ledger)
	pg.GET("/redeem-bonus/latest", h.latestRedeemBonus)
}

// RegisterAdminRoutes 挂载到管理员路由组
func (h *Handler) RegisterAdminRoutes(g *gin.RouterGroup) {
	pg := g.Group("/points")
	pg.GET("/config", h.getConfig)
	pg.PUT("/config", h.saveConfig)
	pg.GET("/accounts", h.adminAccounts)
	pg.GET("/ledger", h.adminLedger)
	pg.POST("/adjust", h.adminAdjust)
	pg.GET("/stats", h.adminStats)
}

// ─── 用户端 ──────────────────────────────────────────────────────────────

func (h *Handler) overview(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	ov, err := h.svc.Overview(c.Request.Context(), uid)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "internal error")
		_ = c.Error(err)
		return
	}
	response.Success(c, ov)
}

func (h *Handler) ledger(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	reason := c.Query("reason")
	list, total, err := h.svc.Ledger(c.Request.Context(), uid, page, pageSize, reason)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "internal error")
		_ = c.Error(err)
		return
	}
	response.Success(c, gin.H{"items": list, "total": total, "page": page, "page_size": pageSize})
}

// latestRedeemBonus 兑换页在兑换成功后调用：返回最近 5 分钟内的一次兑换送积分，没有则 data=null
func (h *Handler) latestRedeemBonus(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	g, err := h.svc.LatestRedeemGrant(c.Request.Context(), uid, 5*time.Minute)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "internal error")
		_ = c.Error(err)
		return
	}
	response.Success(c, g)
}

// ─── 管理端 ──────────────────────────────────────────────────────────────

func (h *Handler) getConfig(c *gin.Context) {
	cfg, ver, err := h.svc.AdminGetConfig(c.Request.Context())
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
	ver, err := h.svc.AdminSaveConfig(c.Request.Context(), req.Config, req.Version)
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

func (h *Handler) adminAccounts(c *gin.Context) {
	page, size := response.ParsePagination(c)
	list, total, err := h.svc.AdminListAccounts(c.Request.Context(), AccountQuery{
		Keyword: c.Query("q"), Sort: c.Query("sort"), Page: page, PageSize: size,
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "internal error")
		_ = c.Error(err)
		return
	}
	page, size = normPage(page, size)
	response.Paginated(c, list, total, page, size)
}

func (h *Handler) adminLedger(c *gin.Context) {
	page, size := response.ParsePagination(c)
	uid, _ := strconv.ParseInt(c.Query("user_id"), 10, 64)
	list, total, err := h.svc.AdminLedger(c.Request.Context(), LedgerQuery{
		UserID: uid, Keyword: c.Query("q"), Reason: c.Query("reason"), Page: page, PageSize: size,
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "internal error")
		_ = c.Error(err)
		return
	}
	page, size = normPage(page, size)
	response.Paginated(c, list, total, page, size)
}

func (h *Handler) adminAdjust(c *gin.Context) {
	opID, ok := userID(c)
	if !ok {
		return
	}
	var req struct {
		UserID int64  `json:"user_id"`
		Delta  int64  `json:"delta"`
		Remark string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.UserID <= 0 {
		response.BadRequest(c, "invalid request body")
		return
	}
	bal, err := h.svc.AdminAdjust(c.Request.Context(), req.UserID, req.Delta, opID, req.Remark)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidInput):
			response.BadRequest(c, strings.TrimPrefix(err.Error(), ErrInvalidInput.Error()+": "))
		case errors.Is(err, ErrUserNotFound):
			response.Error(c, http.StatusNotFound, "用户不存在")
		case errors.Is(err, ErrInsufficientPoints):
			response.BadRequest(c, "扣减数量超过用户当前积分余额")
		default:
			response.Error(c, http.StatusInternalServerError, "internal error")
			_ = c.Error(err)
		}
		return
	}
	response.Success(c, gin.H{"balance": bal})
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

// ─── 工具 ──────────────────────────────────────────────────────────────

func userID(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "not authenticated")
		return 0, false
	}
	return subject.UserID, true
}
