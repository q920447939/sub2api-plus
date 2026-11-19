// Package outreach — HTTP 层：参数解析与错误映射，业务都在 Service
package outreach

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/plus/plusconfig"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterPublicRoutes 免登录：用户从邮件点进来通常没有登录态，token 本身就是凭证
func (h *Handler) RegisterPublicRoutes(g *gin.RouterGroup) {
	o := g.Group("/outreach")
	o.GET("/r/:token", h.click)
	o.POST("/unsub", h.unsub)
}

// RegisterUserRoutes 挂到 /api/v1/plus（JWT）
func (h *Handler) RegisterUserRoutes(g *gin.RouterGroup) {
	o := g.Group("/outreach")
	o.GET("/claim", h.claimInfo)
	o.POST("/claim", h.claim)
}

// RegisterAdminRoutes 挂到 /api/v1/plus/admin
func (h *Handler) RegisterAdminRoutes(g *gin.RouterGroup) {
	o := g.Group("/outreach")
	o.GET("/settings", h.getSettings)
	o.PUT("/settings", h.putSettings)
	o.GET("/meta", func(c *gin.Context) { response.Success(c, h.svc.Meta()) })
	o.POST("/audience/preview", h.preview)
	o.POST("/render", h.render)

	lg := o.Group("/layouts")
	lg.GET("", h.listLayouts)
	lg.POST("", h.createLayout)
	lg.GET("/:id", h.getLayout)
	lg.PUT("/:id", h.updateLayout)
	lg.DELETE("/:id", h.deleteLayout)
	lg.POST("/:id/default", h.setDefaultLayout)

	tg := o.Group("/templates")
	tg.GET("", h.listTemplates)
	tg.POST("", h.createTemplate)
	tg.GET("/:id", h.getTemplate)
	tg.PUT("/:id", h.updateTemplate)
	tg.DELETE("/:id", h.deleteTemplate)

	cg := o.Group("/campaigns")
	cg.GET("", h.list)
	cg.POST("", h.create)
	cg.GET("/:id", h.get)
	cg.PUT("/:id", h.update)
	cg.DELETE("/:id", h.remove)
	cg.POST("/:id/duplicate", h.duplicate)
	cg.POST("/:id/test", h.test)
	cg.GET("/:id/stats", h.campaignStats)
	cg.GET("/:id/batches", h.listBatches)
	cg.POST("/:id/batches", h.createBatch)

	bg := o.Group("/batches")
	bg.GET("/:id", h.getBatch)
	bg.DELETE("/:id", h.deleteBatch)
	bg.POST("/:id/start", h.startBatch)
	bg.POST("/:id/pause", h.pauseBatch)
	bg.POST("/:id/resume", h.resumeBatch)
	bg.POST("/:id/cancel", h.cancelBatch)
	bg.GET("/:id/stats", h.batchStats)
	bg.GET("/:id/recipients", h.recipients)
	bg.GET("/:id/recipients/:rid/preview", h.recipientPreview)
}

// ─── 设置 ────────────────────────────────────────────────────────────────────

func (h *Handler) getSettings(c *gin.Context) {
	cfg, ver, err := h.svc.GetSettings(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"config": cfg, "version": ver})
}

func (h *Handler) putSettings(c *gin.Context) {
	var req struct {
		Config  DeliveryConfig `json:"config"`
		Version int64          `json:"version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求格式错误")
		return
	}
	ver, err := h.svc.SaveSettings(c.Request.Context(), req.Config, req.Version)
	switch {
	case errors.Is(err, plusconfig.ErrVersionMismatch):
		response.Error(c, http.StatusConflict, "配置已被他人修改，请刷新后重试")
	case err != nil:
		// 校验错误直接把文案给管理员
		response.BadRequest(c, err.Error())
	default:
		response.Success(c, gin.H{"version": ver})
	}
}

// ─── 编辑器辅助 ──────────────────────────────────────────────────────────────

func (h *Handler) preview(c *gin.Context) {
	var req struct {
		Purpose    string  `json:"purpose"`
		Segment    Segment `json:"segment"`
		CampaignID int64   `json:"campaign_id"` // 可选：附带本活动已发送 / 排队情况
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求格式错误")
		return
	}
	p, err := h.svc.Preview(c.Request.Context(), req.Purpose, req.Segment, req.CampaignID)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, p)
}

func (h *Handler) render(c *gin.Context) {
	var req RenderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求格式错误")
		return
	}
	subject, body, err := h.svc.RenderPreview(c.Request.Context(), req)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"subject": subject, "html": body})
}

// ─── 外壳 ────────────────────────────────────────────────────────────────────

func (h *Handler) listLayouts(c *gin.Context) {
	items, err := h.svc.ListLayouts(c.Request.Context())
	h.reply(c, items, err)
}

func (h *Handler) getLayout(c *gin.Context) {
	if id, ok := pathID(c, "id"); ok {
		l, err := h.svc.GetLayout(c.Request.Context(), id)
		h.reply(c, l, err)
	}
}

func (h *Handler) createLayout(c *gin.Context) {
	var in LayoutInput
	if !bind(c, &in) {
		return
	}
	l, err := h.svc.CreateLayout(c.Request.Context(), in)
	h.reply(c, l, err)
}

func (h *Handler) updateLayout(c *gin.Context) {
	id, ok := pathID(c, "id")
	var in LayoutInput
	if !ok || !bind(c, &in) {
		return
	}
	l, err := h.svc.UpdateLayout(c.Request.Context(), id, in)
	h.reply(c, l, err)
}

func (h *Handler) deleteLayout(c *gin.Context) {
	h.simple(c, func(id int64) error { return h.svc.DeleteLayout(c.Request.Context(), id) })
}

func (h *Handler) setDefaultLayout(c *gin.Context) {
	h.simple(c, func(id int64) error { return h.svc.SetDefaultLayout(c.Request.Context(), id) })
}

// ─── 模板 ────────────────────────────────────────────────────────────────────

func (h *Handler) listTemplates(c *gin.Context) {
	items, err := h.svc.ListTemplates(c.Request.Context(), c.Query("purpose"))
	h.reply(c, items, err)
}

func (h *Handler) getTemplate(c *gin.Context) {
	if id, ok := pathID(c, "id"); ok {
		t, err := h.svc.GetTemplate(c.Request.Context(), id)
		h.reply(c, t, err)
	}
}

func (h *Handler) createTemplate(c *gin.Context) {
	var in TemplateInput
	if !bind(c, &in) {
		return
	}
	t, err := h.svc.CreateTemplate(c.Request.Context(), in)
	h.reply(c, t, err)
}

func (h *Handler) updateTemplate(c *gin.Context) {
	id, ok := pathID(c, "id")
	var in TemplateInput
	if !ok || !bind(c, &in) {
		return
	}
	t, err := h.svc.UpdateTemplate(c.Request.Context(), id, in)
	h.reply(c, t, err)
}

func (h *Handler) deleteTemplate(c *gin.Context) {
	h.simple(c, func(id int64) error { return h.svc.DeleteTemplate(c.Request.Context(), id) })
}

// ─── 活动 ────────────────────────────────────────────────────────────────────

func (h *Handler) list(c *gin.Context) {
	page, size := response.ParsePagination(c)
	items, total, err := h.svc.List(c.Request.Context(), page, size)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

func (h *Handler) create(c *gin.Context) {
	var in CampaignInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "请求格式错误")
		return
	}
	uid, ok := userID(c)
	if !ok {
		return
	}
	camp, err := h.svc.Create(c.Request.Context(), in, uid)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, camp)
}

func (h *Handler) get(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	camp, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, camp)
}

func (h *Handler) update(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var in CampaignInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "请求格式错误")
		return
	}
	camp, err := h.svc.Update(c.Request.Context(), id, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, camp)
}

func (h *Handler) duplicate(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	uid, ok := userID(c)
	if !ok {
		return
	}
	camp, err := h.svc.Duplicate(c.Request.Context(), id, uid)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, camp)
}

func (h *Handler) remove(c *gin.Context) {
	h.simple(c, func(id int64) error { return h.svc.Delete(c.Request.Context(), id) })
}

func (h *Handler) test(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	uid, ok := userID(c)
	if !ok {
		return
	}
	var req struct {
		SampleEmail string `json:"sample_email"` // 可选：用这个用户的真实数据渲染
	}
	_ = c.ShouldBindJSON(&req)
	to, err := h.svc.SendTest(c.Request.Context(), id, uid, req.SampleEmail)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"sent_to": to})
}

func (h *Handler) campaignStats(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	st, err := h.svc.CampaignStats(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, st)
}

// ─── 批次 ────────────────────────────────────────────────────────────────────

func (h *Handler) listBatches(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	page, size := response.ParsePagination(c)
	items, total, err := h.svc.ListBatches(c.Request.Context(), id, page, size)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

func (h *Handler) createBatch(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var req BatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求格式错误")
		return
	}
	uid, ok := userID(c)
	if !ok {
		return
	}
	res, err := h.svc.CreateBatch(c.Request.Context(), id, req, uid)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, res)
}

func (h *Handler) getBatch(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	b, err := h.svc.GetBatch(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, b)
}

func (h *Handler) deleteBatch(c *gin.Context) {
	h.simple(c, func(id int64) error { return h.svc.DeleteBatch(c.Request.Context(), id) })
}
func (h *Handler) startBatch(c *gin.Context) {
	h.simple(c, func(id int64) error { return h.svc.StartBatch(c.Request.Context(), id) })
}
func (h *Handler) pauseBatch(c *gin.Context) {
	h.simple(c, func(id int64) error { return h.svc.PauseBatch(c.Request.Context(), id) })
}
func (h *Handler) resumeBatch(c *gin.Context) {
	h.simple(c, func(id int64) error { return h.svc.ResumeBatch(c.Request.Context(), id) })
}
func (h *Handler) cancelBatch(c *gin.Context) {
	h.simple(c, func(id int64) error { return h.svc.CancelBatch(c.Request.Context(), id) })
}

func (h *Handler) simple(c *gin.Context, fn func(id int64) error) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := fn(id); err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}

func (h *Handler) batchStats(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	st, err := h.svc.BatchStats(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, st)
}

func (h *Handler) recipients(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	page, size := response.ParsePagination(c)
	items, total, err := h.svc.Recipients(c.Request.Context(), id, c.Query("status"), c.Query("q"), page, size)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

func (h *Handler) recipientPreview(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	rid, ok := pathID(c, "rid")
	if !ok {
		return
	}
	p, err := h.svc.RecipientPreview(c.Request.Context(), id, rid)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, p)
}

// ─── 用户侧 ──────────────────────────────────────────────────────────────────

// click GET /plus/outreach/r/:token → 302。无效链接不暴露细节，统一回站点首页。
func (h *Handler) click(c *gin.Context) {
	target, err := h.svc.Click(c.Request.Context(), c.Param("token"))
	if err != nil {
		if !errors.Is(err, ErrTokenInvalid) {
			log.Printf("[plus.outreach] click: %v", err)
		}
		c.Redirect(http.StatusFound, "/")
		return
	}
	c.Redirect(http.StatusFound, target)
}

func (h *Handler) unsub(c *gin.Context) {
	var req struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" {
		response.BadRequest(c, "链接无效")
		return
	}
	label, unsubscribed, err := h.svc.Unsubscribe(c.Request.Context(), req.Token)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"category": label, "unsubscribed": unsubscribed})
}

func (h *Handler) claimInfo(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	info, err := h.svc.ClaimInfo(c.Request.Context(), c.Query("token"), uid)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, info)
}

func (h *Handler) claim(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" {
		response.BadRequest(c, "链接无效")
		return
	}
	items, err := h.svc.Claim(c.Request.Context(), req.Token, uid)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}

// ─── 工具 ────────────────────────────────────────────────────────────────────

func (h *Handler) fail(c *gin.Context, err error) {
	var ie *InputError
	switch {
	case errors.As(err, &ie):
		response.BadRequest(c, ie.Msg)
	case errors.Is(err, ErrNotFound):
		response.NotFound(c, "不存在或已被删除")
	case errors.Is(err, ErrTokenInvalid):
		response.NotFound(c, "链接无效或已失效")
	case errors.Is(err, ErrAnotherRunning):
		response.Error(c, http.StatusConflict, "已有批次正在执行，请等它完成或先暂停后再开始（共用发信额度，同一时间只执行一个批次）")
	case errors.Is(err, ErrBadState):
		response.Error(c, http.StatusConflict, "当前状态不允许该操作")
	case errors.Is(err, ErrAlreadyClaimed):
		response.Error(c, http.StatusConflict, "已经领取过了")
	case errors.Is(err, ErrClaimExpired):
		response.Error(c, http.StatusGone, "领取已过期")
	default:
		log.Printf("[plus.outreach] %s %s: %v", c.Request.Method, c.FullPath(), err)
		response.InternalError(c, "服务异常，请稍后重试")
	}
}

func (h *Handler) reply(c *gin.Context, data any, err error) {
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, data)
}

func bind(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		response.BadRequest(c, "请求格式错误")
		return false
	}
	return true
}

func userID(c *gin.Context) (int64, bool) {
	s, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "not authenticated")
		return 0, false
	}
	return s.UserID, true
}

func pathID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.NotFound(c, "不存在或已被删除")
		return 0, false
	}
	return id, true
}
