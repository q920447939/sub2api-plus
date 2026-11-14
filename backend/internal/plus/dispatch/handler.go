package dispatch

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// Handler 管理端 HTTP（挂在 /api/v1/plus/admin/dispatch，管理员鉴权）。只给管理员，没有用户端接口。
type Handler struct{ S *Service }

func (h *Handler) Register(admin *gin.RouterGroup) {
	g := admin.Group("/dispatch")
	g.GET("/meta", h.meta)
	g.GET("/config", h.getConfig)
	g.PUT("/config", h.saveConfig)

	g.GET("/board", h.board)
	g.GET("/accounts/:id", h.account)
	g.GET("/accounts/:id/model", h.model)
	g.GET("/accounts/:id/probes/:item", h.probe)
	g.POST("/accounts/:id/probes/:item/retest", h.retest)
	g.POST("/accounts/:id/probes/:item/ignore", h.ignore)

	g.GET("/weights/sheet", h.sheet)
	g.POST("/weights/preview", h.sheetPreview)
	g.POST("/weights", h.sheetSave)
	g.GET("/changes", h.changes)
	g.POST("/rules/restore", h.restore)
	g.POST("/records/:id/undo", h.undo)
	g.GET("/badges", h.badges)

	g.GET("/runs/board", h.runsBoard)
	g.GET("/runs", h.runsList)
	g.GET("/runs/:id", h.run)

	g.GET("/items", h.items)
	g.POST("/items", h.saveItem)
	g.PUT("/items/:id", h.saveItem)
	g.DELETE("/items/:id", h.deleteItem)
	g.POST("/items/:id/toggle", h.toggleItem)
	g.POST("/items/try-grade", h.tryGrade)
	g.POST("/plans/preview", h.previewPlan)
	g.POST("/plans/:id/run", h.runPlan)
	g.POST("/plans/:id/toggle", h.togglePlan)

	g.GET("/audit", h.audit)
}

func actorOf(c *gin.Context) (Actor, bool) {
	s, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "not authenticated")
		return Actor{}, false
	}
	return Actor{ID: s.UserID}, true
}

func idParam(c *gin.Context, k string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(k), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid "+k)
		return 0, false
	}
	return id, true
}

func qInt(c *gin.Context, k string) int64 {
	v, _ := strconv.ParseInt(c.Query(k), 10, 64)
	return v
}

func gran(c *gin.Context) Gran {
	if c.Query("gran") == string(GranDay) {
		return GranDay
	}
	return GranHour
}

// fail 错误映射：校验 400、不存在 404、冲突 409，其他 500（不把内部错误细节返回给前端）
func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		response.BadRequest(c, cleanErr(err))
	case errors.Is(err, ErrNotFound):
		response.NotFound(c, "记录不存在")
	case errors.Is(err, ErrVersionMismatch):
		response.Error(c, http.StatusConflict, "配置已被他人修改，请刷新后重试")
	case errors.Is(err, ErrConflict):
		response.Error(c, http.StatusConflict, cleanErr(err))
	default:
		log.Printf("[plus.dispatch] %s %s: %v", c.Request.Method, c.FullPath(), err)
		response.InternalError(c, "服务器内部错误")
	}
}

func cleanErr(err error) string {
	s := err.Error()
	for _, p := range []string{ErrInvalid.Error() + "：", ErrConflict.Error() + "："} {
		s = strings.ReplaceAll(s, p, "")
	}
	return s
}

func bind(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		response.BadRequest(c, "请求格式不正确："+err.Error())
		return false
	}
	return true
}

func ok(c *gin.Context, v any, err error) {
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, v)
}

// ---------- 配置 / 元数据 ----------

func (h *Handler) meta(c *gin.Context) {
	v, err := h.S.Meta(c.Request.Context())
	ok(c, v, err)
}

func (h *Handler) getConfig(c *gin.Context) {
	v, err := h.S.GetConfig(c.Request.Context())
	ok(c, v, err)
}

func (h *Handler) saveConfig(c *gin.Context) {
	actor, okA := actorOf(c)
	if !okA {
		return
	}
	var req struct {
		Config  Config `json:"config"`
		Version int64  `json:"version"`
	}
	if !bind(c, &req) {
		return
	}
	v, err := h.S.SaveConfig(c.Request.Context(), req.Config, req.Version, actor)
	ok(c, v, err)
}

// ---------- 上游账号 ----------

func (h *Handler) board(c *gin.Context) {
	v, err := h.S.Board(c.Request.Context(), gran(c))
	ok(c, v, err)
}

func (h *Handler) account(c *gin.Context) {
	id, okI := idParam(c, "id")
	if !okI {
		return
	}
	v, err := h.S.Account(c.Request.Context(), id, gran(c))
	ok(c, v, err)
}

func (h *Handler) model(c *gin.Context) {
	id, okI := idParam(c, "id")
	if !okI {
		return
	}
	m := strings.TrimSpace(c.Query("model"))
	if m == "" {
		response.BadRequest(c, "model 不能为空")
		return
	}
	v, err := h.S.ModelDetail(c.Request.Context(), id, m)
	ok(c, v, err)
}

func (h *Handler) probe(c *gin.Context) {
	id, okI := idParam(c, "id")
	item, okJ := idParam(c, "item")
	if !okI || !okJ {
		return
	}
	v, err := h.S.ProbeSheet(c.Request.Context(), id, item)
	ok(c, v, err)
}

func (h *Handler) retest(c *gin.Context) {
	actor, okA := actorOf(c)
	id, okI := idParam(c, "id")
	item, okJ := idParam(c, "item")
	if !okA || !okI || !okJ {
		return
	}
	var req struct {
		Models []string `json:"models"`
	}
	if !bind(c, &req) {
		return
	}
	n, err := h.S.Retest(c.Request.Context(), id, item, req.Models, actor)
	ok(c, gin.H{"queued": n}, err)
}

func (h *Handler) ignore(c *gin.Context) {
	actor, okA := actorOf(c)
	id, okI := idParam(c, "id")
	item, okJ := idParam(c, "item")
	if !okA || !okI || !okJ {
		return
	}
	var req struct {
		Model string `json:"model"`
		On    bool   `json:"on"`
	}
	if !bind(c, &req) {
		return
	}
	ok(c, gin.H{}, h.S.SetIgnore(c.Request.Context(), id, item, req.Model, req.On, actor))
}

// ---------- 权重 ----------

func (h *Handler) sheet(c *gin.Context) {
	v, err := h.S.SheetOpen(c.Request.Context(), qInt(c, "account_id"), strings.TrimSpace(c.Query("model")))
	ok(c, v, err)
}

func (h *Handler) sheetPreview(c *gin.Context) {
	var req SheetReq
	if !bind(c, &req) {
		return
	}
	v, err := h.S.SheetPreview(c.Request.Context(), req)
	ok(c, v, err)
}

func (h *Handler) sheetSave(c *gin.Context) {
	actor, okA := actorOf(c)
	if !okA {
		return
	}
	var req SheetReq
	if !bind(c, &req) {
		return
	}
	v, err := h.S.SheetSave(c.Request.Context(), req, actor)
	ok(c, v, err)
}

func (h *Handler) changes(c *gin.Context) {
	v, err := h.S.Changes(c.Request.Context(), c.Query("window"))
	ok(c, v, err)
}

func (h *Handler) restore(c *gin.Context) {
	actor, okA := actorOf(c)
	if !okA {
		return
	}
	var req struct {
		AccountID int64  `json:"account_id"`
		GroupID   int64  `json:"group_id"`
		Model     string `json:"model"`
	}
	if !bind(c, &req) {
		return
	}
	ok(c, gin.H{}, h.S.Restore(c.Request.Context(), req.AccountID, req.GroupID, req.Model, actor))
}

func (h *Handler) undo(c *gin.Context) {
	actor, okA := actorOf(c)
	id, okI := idParam(c, "id")
	if !okA || !okI {
		return
	}
	ok(c, gin.H{}, h.S.Undo(c.Request.Context(), id, actor))
}

func (h *Handler) badges(c *gin.Context) {
	v, err := h.S.Badges(c.Request.Context())
	ok(c, v, err)
}

// ---------- 检测记录 ----------

func runsQuery(c *gin.Context) RunsQuery {
	return RunsQuery{ItemID: qInt(c, "item_id"), Platform: c.Query("platform"), Result: c.Query("result"), Keyword: c.Query("q")}
}

func (h *Handler) runsBoard(c *gin.Context) {
	v, err := h.S.RunsBoard(c.Request.Context(), runsQuery(c))
	ok(c, v, err)
}

func (h *Handler) runsList(c *gin.Context) {
	v, err := h.S.RunsList(c.Request.Context(), runsQuery(c), qInt(c, "before_id"))
	ok(c, v, err)
}

func (h *Handler) run(c *gin.Context) {
	id, okI := idParam(c, "id")
	if !okI {
		return
	}
	v, err := h.S.Run(c.Request.Context(), id)
	ok(c, v, err)
}

// ---------- 检测项 / 计划 ----------

func (h *Handler) items(c *gin.Context) {
	v, err := h.S.Items(c.Request.Context())
	ok(c, v, err)
}

func (h *Handler) saveItem(c *gin.Context) {
	actor, okA := actorOf(c)
	if !okA {
		return
	}
	var req ItemReq
	if !bind(c, &req) {
		return
	}
	if p := c.Param("id"); p != "" {
		id, okI := idParam(c, "id")
		if !okI {
			return
		}
		req.Item.ID = id
	} else {
		req.Item.ID = 0
	}
	id, err := h.S.SaveItem(c.Request.Context(), req, actor)
	ok(c, gin.H{"id": id}, err)
}

func (h *Handler) deleteItem(c *gin.Context) {
	actor, okA := actorOf(c)
	id, okI := idParam(c, "id")
	if !okA || !okI {
		return
	}
	ok(c, gin.H{}, h.S.DeleteItem(c.Request.Context(), id, actor))
}

func (h *Handler) toggleItem(c *gin.Context) {
	actor, okA := actorOf(c)
	id, okI := idParam(c, "id")
	if !okA || !okI {
		return
	}
	var req struct {
		On bool `json:"on"`
	}
	if !bind(c, &req) {
		return
	}
	ok(c, gin.H{}, h.S.ToggleItem(c.Request.Context(), id, req.On, actor))
}

func (h *Handler) tryGrade(c *gin.Context) {
	var req struct {
		Params GradedParams `json:"params"`
		Reply  string       `json:"reply"`
	}
	if !bind(c, &req) {
		return
	}
	got, pass, err := TryGrade(req.Params, req.Reply)
	ok(c, gin.H{"answer": got, "pass": pass}, err)
}

func (h *Handler) previewPlan(c *gin.Context) {
	var req struct {
		Type string `json:"type"`
		Plan Plan   `json:"plan"`
	}
	if !bind(c, &req) {
		return
	}
	v, err := h.S.PreviewPlan(c.Request.Context(), req.Type, req.Plan)
	ok(c, v, err)
}

func (h *Handler) runPlan(c *gin.Context) {
	actor, okA := actorOf(c)
	id, okI := idParam(c, "id")
	if !okA || !okI {
		return
	}
	n, err := h.S.RunPlan(c.Request.Context(), id, actor)
	ok(c, gin.H{"queued": n}, err)
}

func (h *Handler) togglePlan(c *gin.Context) {
	actor, okA := actorOf(c)
	id, okI := idParam(c, "id")
	if !okA || !okI {
		return
	}
	var req struct {
		On bool `json:"on"`
	}
	if !bind(c, &req) {
		return
	}
	ok(c, gin.H{}, h.S.TogglePlan(c.Request.Context(), id, req.On, actor))
}

// ---------- 操作日志 ----------

func (h *Handler) audit(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	size, _ := strconv.Atoi(c.Query("page_size"))
	v, err := h.S.Audit(c.Request.Context(), AuditFilter{TargetType: c.Query("kind"), Keyword: strings.TrimSpace(c.Query("q")), Page: page, PageSize: size})
	ok(c, v, err)
}
