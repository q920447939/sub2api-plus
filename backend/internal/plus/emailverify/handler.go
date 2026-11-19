// Package emailverify — [plus] HTTP 处理层：只做参数解析与错误映射，统一 {code, data} 响应
package emailverify

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/plus/plusconfig"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterUserRoutes 用户端
func (h *Handler) RegisterUserRoutes(user *gin.RouterGroup) {
	g := user.Group("/email-verify")
	g.GET("/status", h.GetStatus)
	g.POST("/send-code", h.SendCode)
	g.POST("/confirm", h.ConfirmCode)
}

// RegisterPublicRoutes 免登录：用户从邮件点进来时通常没有登录态，token 本身就是凭证。
func (h *Handler) RegisterPublicRoutes(public *gin.RouterGroup) {
	public.POST("/email-verify/invite", h.ConsumeInviteToken)
}

// RegisterAdminRoutes 管理端：概览 / 验证记录 / 设置 / 单发邀请
func (h *Handler) RegisterAdminRoutes(admin *gin.RouterGroup) {
	g := admin.Group("/email-verify")
	g.GET("/overview", h.AdminOverview)
	g.GET("/records", h.AdminRecords)
	g.GET("/settings", h.GetSettings)
	g.PUT("/settings", h.PutSettings)
	g.POST("/invite/:user_id", h.AdminSendInvite)
}

// ─── 用户端 ───────────────────────────────────────────────────────────────────

func (h *Handler) GetStatus(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	view, err := h.svc.GetStatusView(c.Request.Context(), uid)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, view)
}

func (h *Handler) SendCode(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	if err := h.svc.SendCode(c.Request.Context(), uid, c.GetHeader("Accept-Language")); err != nil {
		fail(c, err)
		return
	}
	response.Success(c, gin.H{"sent": true})
}

func (h *Handler) ConfirmCode(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
		response.BadRequest(c, "缺少验证码")
		return
	}
	res, err := h.svc.ConfirmCode(c.Request.Context(), uid, req.Code)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, verifyResult(res))
}

// ConsumeInviteToken POST /plus/email-verify/invite  body: {"token": "..."}
// 不用 GET：带副作用的 GET 会被邮件安全网关、浏览器预取提前触发而烧掉一次性 token。
func (h *Handler) ConsumeInviteToken(c *gin.Context) {
	var req struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" {
		response.BadRequest(c, "缺少 token")
		return
	}
	res, err := h.svc.ConsumeInviteToken(c.Request.Context(), req.Token)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, verifyResult(res))
}

func verifyResult(r *VerifyResult) gin.H {
	return gin.H{"status": r.Status, "reward_granted": r.RewardGranted, "reward_points": r.RewardPoints}
}

// ─── 管理端 ───────────────────────────────────────────────────────────────────

func (h *Handler) AdminOverview(c *gin.Context) {
	st, err := h.svc.GetOverviewStats(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, st)
}

func (h *Handler) AdminRecords(c *gin.Context) {
	page, size := response.ParsePagination(c)
	items, total, err := h.svc.ListRecords(c.Request.Context(), RecordQuery{Page: page, PageSize: size, Q: c.Query("q"), Source: c.Query("source")})
	if err != nil {
		fail(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

func (h *Handler) GetSettings(c *gin.Context) {
	cfg, ver, err := h.svc.GetConfig(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, gin.H{"config": cfg, "version": ver})
}

func (h *Handler) PutSettings(c *gin.Context) {
	var req struct {
		Config  Config `json:"config"`
		Version int64  `json:"version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求格式错误")
		return
	}
	ver, err := h.svc.SaveConfig(c.Request.Context(), req.Config, req.Version)
	switch {
	case errors.Is(err, plusconfig.ErrVersionMismatch):
		response.Error(c, http.StatusConflict, "设置已被其他人修改，请刷新后重试")
	case err != nil:
		response.BadRequest(c, err.Error())
	default:
		response.Success(c, gin.H{"version": ver})
	}
}

func (h *Handler) AdminSendInvite(c *gin.Context) {
	uid, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil || uid <= 0 {
		response.BadRequest(c, "无效的用户 ID")
		return
	}
	if err := h.svc.SendInviteToUser(c.Request.Context(), uid); err != nil {
		fail(c, err)
		return
	}
	response.Success(c, gin.H{"sent": true})
}

// ─── 辅助 ─────────────────────────────────────────────────────────────────────

func userID(c *gin.Context) (int64, bool) {
	s, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "未登录")
		return 0, false
	}
	return s.UserID, true
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrAdminExempt):
		response.BadRequest(c, "管理员无需验证邮箱")
	case errors.Is(err, ErrAlreadyVerified):
		response.Error(c, http.StatusConflict, "邮箱已验证")
	case errors.Is(err, ErrEmailInvalid):
		response.BadRequest(c, "这个邮箱收不到信（曾经退信），请先更换邮箱")
	case errors.Is(err, ErrMailerUnavailable):
		response.Error(c, http.StatusServiceUnavailable, "触达中心不可用，暂时不能发送邀请")
	case errors.Is(err, ErrCodeInvalid):
		response.BadRequest(c, "验证码错误或已过期，请检查后重新输入")
	case errors.Is(err, ErrCodeMaxAttempts):
		response.Error(c, http.StatusTooManyRequests, "错误次数过多，请重新获取验证码")
	case errors.Is(err, ErrCodeTooFrequent):
		response.Error(c, http.StatusTooManyRequests, "发送太频繁，请稍后再试")
	case errors.Is(err, ErrTokenNotFound):
		response.BadRequest(c, "链接无效或已过期")
	default:
		response.ErrorFrom(c, err) // 验证码错误等上游错误自带状态码；5xx 由它记日志
	}
}
