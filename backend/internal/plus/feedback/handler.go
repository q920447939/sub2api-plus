// Package feedback — [plus] HTTP 层：参数解析和错误映射，业务都在 Service
package feedback

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterUserRoutes 挂到 /api/v1/plus（JWT）
func (h *Handler) RegisterUserRoutes(g *gin.RouterGroup) {
	fg := g.Group("/feedback")
	fg.GET("/meta", h.meta)
	fg.GET("/unread", h.unread)
	fg.GET("", h.listMine)
	fg.POST("", h.submit)
	fg.GET("/:id", h.mineDetail)
	fg.POST("/:id/messages", h.supplement)
	fg.POST("/attachments", h.upload)
	fg.GET("/attachments/:id", h.attachment(false))
}

// RegisterAdminRoutes 挂到 /api/v1/plus/admin（管理员）
func (h *Handler) RegisterAdminRoutes(g *gin.RouterGroup) {
	fg := g.Group("/feedback")
	fg.GET("", h.adminList)
	fg.GET("/pending-count", h.pendingCount)
	fg.GET("/tags", h.tags)
	fg.GET("/stats", h.stats)
	fg.GET("/config", h.getConfig)
	fg.PUT("/config", h.saveConfig)
	fg.POST("/batch", h.batch)
	fg.POST("/attachments", h.upload)
	fg.GET("/attachments/:id", h.attachment(true))
	fg.GET("/:id", h.adminDetail)
	fg.PATCH("/:id", h.adminUpdate)
	fg.POST("/:id/messages", h.adminMessage)
	fg.POST("/:id/close", h.close)
}

// ─── 用户端 ──────────────────────────────────────────────────────────────

func (h *Handler) meta(c *gin.Context) {
	m, err := h.svc.Meta(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, m)
}

func (h *Handler) unread(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	n, err := h.svc.Unread(c.Request.Context(), uid)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"count": n})
}

func (h *Handler) listMine(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	p, size := response.ParsePagination(c)
	uq := UserQuery{UserID: uid, Page: p, PageSize: size}
	switch c.Query("state") {
	case "open":
		t := true
		uq.Open = &t
	case "closed":
		f := false
		uq.Open = &f
	}
	if k := Kind(c.Query("kind")); k.Valid() {
		uq.Kind = k
	}
	list, total, p, size, err := h.svc.ListMine(c.Request.Context(), uq)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Paginated(c, list, total, p, size)
}

func (h *Handler) submit(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	var in SubmitInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "请求格式错误")
		return
	}
	f, err := h.svc.Submit(c.Request.Context(), uid, in, ClientInfo{IP: c.ClientIP(), UserAgent: c.Request.UserAgent()})
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"id": f.ID})
}

func (h *Handler) mineDetail(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	d, err := h.svc.MineDetail(c.Request.Context(), uid, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, d)
}

func (h *Handler) supplement(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in MessageInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "请求格式错误")
		return
	}
	m, err := h.svc.Supplement(c.Request.Context(), uid, id, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, m)
}

// upload multipart 字段 file；大小上限由配置决定，这里再加一道 21MB 的硬上限
func (h *Handler) upload(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 21<<20)
	fh, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "图片太大或格式错误")
		return
	}
	f, err := fh.Open()
	if err != nil {
		response.BadRequest(c, "读取图片失败")
		return
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		response.BadRequest(c, "读取图片失败")
		return
	}
	id, err := h.svc.Upload(c.Request.Context(), uid, data)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}

func (h *Handler) attachment(admin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := userID(c)
		if !ok {
			return
		}
		id, ok := pathID(c)
		if !ok {
			return
		}
		rc, mime, err := h.svc.OpenAttachment(c.Request.Context(), uid, admin, id)
		if err != nil {
			h.fail(c, err)
			return
		}
		defer func() { _ = rc.Close() }()
		c.Header("Cache-Control", "private, max-age=86400")
		c.Header("X-Content-Type-Options", "nosniff")
		c.DataFromReader(http.StatusOK, -1, mime, rc, nil)
	}
}

// ─── 管理端 ──────────────────────────────────────────────────────────────

func (h *Handler) adminList(c *gin.Context) {
	p, size := response.ParsePagination(c)
	aq := AdminQuery{
		Status: c.Query("status"), Kind: Kind(c.Query("kind")), Scenario: c.Query("scenario"),
		Severity: c.Query("severity"), Tag: c.Query("tag"),
		Content: c.Query("content"), User: c.Query("user"), ID: c.Query("id"), Page: p, PageSize: size,
	}
	aq.UserID, _ = strconv.ParseInt(c.Query("user_id"), 10, 64)
	if t, err := time.Parse(time.RFC3339, c.Query("from")); err == nil {
		aq.From = &t
	}
	if t, err := time.Parse(time.RFC3339, c.Query("to")); err == nil {
		aq.To = &t
	}
	// days=N：只看近 N 天提交的，与看板口径一致（看板下钻带过来）
	if n, err := strconv.Atoi(c.Query("days")); err == nil && n > 0 && n <= 365 && aq.From == nil {
		t := h.svc.RangeStart(n)
		aq.From = &t
	}
	if v := c.Query("rewarded"); v != "" {
		b := v == "1" || v == "true"
		aq.Rewarded = &b
	}
	ctx := c.Request.Context()
	list, total, p, size, err := h.svc.AdminList(ctx, aq)
	if err != nil {
		h.fail(c, err)
		return
	}
	counts, err := h.svc.StatusCounts(ctx)
	if err != nil {
		h.fail(c, err)
		return
	}
	pages := int((total + int64(size) - 1) / int64(size))
	if pages < 1 {
		pages = 1
	}
	response.Success(c, gin.H{"items": list, "total": total, "page": p, "page_size": size, "pages": pages, "counts": counts})
}

func (h *Handler) pendingCount(c *gin.Context) {
	n, err := h.svc.PendingCount(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"count": n})
}

func (h *Handler) tags(c *gin.Context) {
	t, err := h.svc.Tags(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, t)
}

func (h *Handler) stats(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	st, err := h.svc.Stats(c.Request.Context(), days)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, st)
}

func (h *Handler) getConfig(c *gin.Context) {
	cfg, ver, err := h.svc.AdminConfig(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"config": cfg, "version": ver, "reward_enabled": h.svc.RewardEnabled()})
}

func (h *Handler) saveConfig(c *gin.Context) {
	var req struct {
		Config  Config `json:"config"`
		Version int64  `json:"version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求格式错误")
		return
	}
	ver, err := h.svc.SaveConfig(c.Request.Context(), req.Config, req.Version)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"version": ver})
}

func (h *Handler) adminDetail(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	d, err := h.svc.AdminDetail(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"detail": d, "reward_enabled": h.svc.RewardEnabled()})
}

func (h *Handler) adminUpdate(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in UpdateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "请求格式错误")
		return
	}
	if err := h.svc.AdminUpdate(c.Request.Context(), uid, id, in); err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, nil)
}

func (h *Handler) adminMessage(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in MessageInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "请求格式错误")
		return
	}
	m, err := h.svc.AdminMessage(c.Request.Context(), uid, id, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, m)
}

func (h *Handler) close(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req CloseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求格式错误")
		return
	}
	req.Reason, req.Message = strings.TrimSpace(req.Reason), strings.TrimSpace(req.Message)
	if err := h.svc.Close(c.Request.Context(), uid, id, req); err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, nil)
}

func (h *Handler) batch(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	var in BatchInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "请求格式错误")
		return
	}
	in.AddTag, in.Decline = strings.TrimSpace(in.AddTag), strings.TrimSpace(in.Decline)
	res, err := h.svc.Batch(c.Request.Context(), uid, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, res)
}

// ─── 工具 ──────────────────────────────────────────────────────────────

func userID(c *gin.Context) (int64, bool) {
	s, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "not authenticated")
		return 0, false
	}
	return s.UserID, true
}

func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.NotFound(c, "反馈不存在")
		return 0, false
	}
	return id, true
}

func (h *Handler) fail(c *gin.Context, err error) {
	var (
		ie *InputError
		re *RateError
	)
	switch {
	case errors.As(err, &ie):
		response.BadRequest(c, ie.Msg)
	case errors.As(err, &re):
		response.Error(c, http.StatusTooManyRequests, re.Msg)
	case errors.Is(err, ErrNotFound):
		response.NotFound(c, "反馈不存在")
	case errors.Is(err, ErrClosed):
		response.Error(c, http.StatusConflict, "这条反馈已结案")
	case errors.Is(err, ErrDisabled):
		response.Error(c, http.StatusForbidden, "反馈功能未开启")
	case errors.Is(err, ErrRewardDisabled):
		response.Error(c, http.StatusConflict, "积分功能未启用，不能发放积分")
	case errors.Is(err, ErrVersionMismatch):
		response.Error(c, http.StatusConflict, "配置已被其他人修改，请刷新后重试")
	default:
		response.Error(c, http.StatusInternalServerError, "internal error")
		_ = c.Error(err)
	}
}
