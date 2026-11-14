package dispatch

// 存档题作品预览：和「模型智商检测」同一套做法（iq.Signer + iq.ServePreviewHTML）。
// 作品是不可信的 HTML，用 <iframe sandbox="allow-scripts" src=...> 加载；iframe 带不了 Authorization 头，
// 所以由已鉴权的管理端接口签发短期链接，预览接口只认签名。
// 不用 srcdoc：srcdoc 会继承站点的 CSP（只允许带 nonce 的脚本），作品里的动画跑不起来。

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/plus/iq"
	"github.com/gin-gonic/gin"
)

const previewBasePath = "/api/v1/plus/dispatch-preview"

// PreviewStore 预览需要的数据
type PreviewStore interface {
	PreviewSecret(ctx context.Context) ([]byte, error)
	// SavedHTML 一份作品（只认存档成功的记录）
	SavedHTML(ctx context.Context, runID int64) (string, error)
}

// Previewer 签发 / 校验预览链接（密钥在迁移时生成且不变，读一次即可）
type Previewer struct {
	Store  PreviewStore
	mu     sync.Mutex
	secret []byte
}

func (p *Previewer) signer(ctx context.Context) (iq.Signer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.secret == nil {
		s, err := p.Store.PreviewSecret(ctx)
		if err != nil {
			return iq.Signer{}, err
		}
		p.secret = s
	}
	return iq.Signer{Secret: p.secret, Base: previewBasePath}, nil
}

// URL 预览链接；出错时返回空（前端显示「预览不可用」）
func (p *Previewer) URL(ctx context.Context, runID int64) string {
	if p == nil {
		return ""
	}
	s, err := p.signer(ctx)
	if err != nil {
		return ""
	}
	return s.URL(runID, iq.ScopeAdmin)
}

// Register 预览接口不挂 JWT，靠签名鉴权
func (p *Previewer) Register(v1 *gin.RouterGroup) {
	v1.GET("/plus/dispatch-preview/:id", p.serve)
}

func (p *Previewer) serve(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.String(http.StatusNotFound, "not found")
		return
	}
	s, err := p.signer(ctx)
	if err != nil {
		c.String(http.StatusInternalServerError, "internal error")
		return
	}
	if sc, ok := s.Verify(id, c.Query("sc"), c.Query("e"), c.Query("s")); !ok || sc != iq.ScopeAdmin {
		c.String(http.StatusForbidden, "预览链接已过期，请刷新页面")
		return
	}
	html, err := p.Store.SavedHTML(ctx, id)
	if errors.Is(err, ErrNotFound) {
		c.String(http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		c.String(http.StatusInternalServerError, "internal error")
		return
	}
	name := ""
	if c.Query("download") == "1" {
		name = "dispatch-work-" + strconv.FormatInt(id, 10) + ".html"
	}
	iq.ServePreviewHTML(c, html, name)
}

// ---------- 仓储 ----------

func (r *Repository) PreviewSecret(ctx context.Context) ([]byte, error) {
	var s string
	if err := r.db.QueryRowContext(ctx, `SELECT preview_secret FROM plus_dispatch_config WHERE id = 1`).Scan(&s); err != nil {
		return nil, err
	}
	if len(s) < 32 {
		return nil, errors.New("dispatch: preview secret too short")
	}
	return []byte(s), nil
}

func (r *Repository) SavedHTML(ctx context.Context, runID int64) (string, error) {
	var html string
	err := r.db.QueryRowContext(ctx, `SELECT reply FROM plus_dispatch_run WHERE id = $1 AND outcome = $2`, runID, OutcomeSaved).Scan(&html)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return html, err
}

// ---------- 给返回的记录挂上预览链接 ----------

func (s *Service) withPreview(ctx context.Context, r *Run) {
	if r.Outcome == OutcomeSaved {
		r.PreviewURL = s.Preview.URL(ctx, r.ID)
	}
}
