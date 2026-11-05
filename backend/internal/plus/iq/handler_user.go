// Package iq — [plus] 用户端 HTTP（挂在 /api/v1/plus，JWT 鉴权）与预览接口（签名鉴权）
package iq

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// UserHandler 用户端「模型画廊」
type UserHandler struct{ svc *UserService }

func NewUserHandler(svc *UserService) *UserHandler { return &UserHandler{svc: svc} }

// RegisterUserRoutes 挂载到已鉴权用户路由组（/plus）
func (h *UserHandler) RegisterUserRoutes(g *gin.RouterGroup) {
	ig := g.Group("/iq")
	ig.GET("/meta", h.meta)
	ig.GET("/gallery", h.gallery)
}

func (h *UserHandler) meta(c *gin.Context) {
	m, err := h.svc.Meta(c.Request.Context())
	if err != nil {
		internalErr(c, err)
		return
	}
	response.Success(c, m)
}

func (h *UserHandler) gallery(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "not authenticated")
		return
	}
	g, err := h.svc.Gallery(c.Request.Context(), subject.UserID)
	if errors.Is(err, ErrDisabled) {
		response.NotFound(c, "模型画廊暂未开放")
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	response.Success(c, g)
}

func internalErr(c *gin.Context, err error) {
	response.Error(c, http.StatusInternalServerError, "internal error")
	_ = c.Error(err)
}

// PreviewHandler 预览接口：返回模型生成的 HTML，供 sandbox iframe 加载
type PreviewHandler struct {
	Runs   RunReader
	Config ConfigStore
	Signer func(ctx context.Context) (Signer, error)
}

// previewCSPHeader sandbox 让文档运行在不透明源里（拿不到本站 Cookie / localStorage），并禁止一切外连；
// frame-ancestors 只允许本站嵌入。
const previewCSPHeader = "sandbox allow-scripts; " + PreviewCSP + "; frame-ancestors 'self'"

// Register 预览接口不挂 JWT（iframe 带不了 Authorization 头），靠签名鉴权
func (h *PreviewHandler) Register(v1 *gin.RouterGroup) {
	v1.GET("/plus/iq-preview/:id", h.serve)
}

func (h *PreviewHandler) serve(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	signer, err := h.Signer(ctx)
	if err != nil {
		c.String(http.StatusInternalServerError, "internal error")
		return
	}
	scope, ok := signer.Verify(id, c.Query("sc"), c.Query("e"), c.Query("s"))
	if !ok {
		c.String(http.StatusForbidden, "预览链接已过期，请刷新页面")
		return
	}
	html, status, _, err := h.Runs.RunHTML(ctx, id)
	if errors.Is(err, ErrNotFound) {
		c.String(http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		c.String(http.StatusInternalServerError, "internal error")
		return
	}
	if scope == ScopeUser {
		cfg, _, err := h.Config.LoadConfig(ctx)
		if err != nil || !cfg.UserEnabled || status != RunSuccess {
			c.String(http.StatusNotFound, "not found")
			return
		}
	}
	c.Header("Content-Security-Policy", previewCSPHeader)
	c.Header("X-Frame-Options", "SAMEORIGIN")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Cache-Control", "private, max-age=3600")
	if c.Query("download") == "1" && scope == ScopeAdmin {
		c.Header("Content-Disposition", "attachment; filename=pelican-"+strconv.FormatInt(id, 10)+".html")
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(WithPreviewCSP(html)))
}
