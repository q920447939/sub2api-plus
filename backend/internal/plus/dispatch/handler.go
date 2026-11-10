package dispatch

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// Handler 管理端 HTTP（挂在 /api/v1/plus/admin/dispatch，管理员鉴权）。只给管理员，没有用户端接口。
type Handler struct {
	Setup  *SetupService
	Query  *QueryService
	Adjust *AdjustService
	Audit  AuditStore
	Eval   *Evaluator
}

func (h *Handler) Register(admin *gin.RouterGroup) {
	g := admin.Group("/dispatch")
	g.GET("/meta", h.meta)
	g.GET("/config", h.getConfig)
	g.PUT("/config", h.saveConfig)
	g.POST("/evaluate", h.evaluate)

	g.POST("/dimensions", h.saveDimension)
	g.PUT("/dimensions/:id", h.saveDimension)
	g.DELETE("/dimensions/:id", h.deleteDimension)
	g.PUT("/dimensions-order", h.reorderDimensions)

	g.POST("/items", h.saveItem)
	g.PUT("/items/:id", h.saveItem)
	g.DELETE("/items/:id", h.deleteItem)
	g.POST("/items/try-grade", h.tryGrade)

	g.POST("/plans", h.savePlan)
	g.PUT("/plans/:id", h.savePlan)
	g.DELETE("/plans/:id", h.deletePlan)
	g.POST("/plans/preview", h.previewPlan)
	g.POST("/plans/:id/run", h.runPlan)

	g.GET("/overview", h.overview)
	g.GET("/accounts/:id", h.accountDetail)
	g.POST("/accounts/:id/checkup", h.checkup)
	g.GET("/compare", h.compare)
	g.GET("/runs/board", h.runsBoard)
	g.GET("/runs", h.runsList)
	g.GET("/runs/:id", h.runDetail)
	g.POST("/runs/:id/rerun", h.rerun)
	g.GET("/events", h.events)
	g.POST("/events/:id/state", h.eventState)
	g.GET("/suggestions", h.suggestions)

	g.GET("/adjustments", h.adjustments)
	g.POST("/adjustments", h.applyAdjustment)
	g.POST("/adjustments/preview", h.previewAdjustment)
	g.GET("/adjustments/logs", h.adjustmentLogs)
	g.GET("/adjustments/logs/:id/effect", h.effect)
	g.GET("/adjustments/badges", h.badges)

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

func idParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid id")
		return 0, false
	}
	return id, true
}

func qInt(c *gin.Context, k string) int64 {
	v, _ := strconv.ParseInt(c.Query(k), 10, 64)
	return v
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
	for _, p := range []string{ErrInvalid.Error() + "：", ErrConflict.Error() + "：", ErrInvalid.Error() + "\n"} {
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

// ---------- 配置 ----------

func (h *Handler) meta(c *gin.Context) {
	v, err := h.Query.Meta(c.Request.Context())
	ok(c, v, err)
}

func (h *Handler) getConfig(c *gin.Context) {
	cfg, ver, err := h.Setup.GetConfig(c.Request.Context())
	ok(c, gin.H{"config": cfg, "version": ver}, err)
}

func (h *Handler) saveConfig(c *gin.Context) {
	actor, okA := actorOf(c)
	var req struct {
		Config  Config `json:"config"`
		Version int64  `json:"version"`
	}
	if !okA || !bind(c, &req) {
		return
	}
	ver, err := h.Setup.SaveConfig(c.Request.Context(), actor, req.Config, req.Version)
	ok(c, gin.H{"version": ver}, err)
}

func (h *Handler) evaluate(c *gin.Context) {
	actor, okA := actorOf(c)
	if !okA {
		return
	}
	rep, err := h.Eval.RunNow(c.Request.Context())
	if err == nil {
		Auditor{Store: h.Audit}.Record(c.Request.Context(), actor, "evaluate", "diagnosis", "",
			fmt.Sprintf("重新诊断：%d 个对象，%d 项结论有变化", rep.Subjects, rep.Changed), nil, rep)
	}
	ok(c, gin.H{"at": time.Now(), "report": rep}, err)
}

func (h *Handler) saveDimension(c *gin.Context) {
	actor, okA := actorOf(c)
	var d Dimension
	if !okA || !bind(c, &d) {
		return
	}
	if c.Param("id") != "" {
		id, okI := idParam(c)
		if !okI {
			return
		}
		d.ID = id
	} else {
		d.ID = 0
	}
	v, err := h.Setup.SaveDimension(c.Request.Context(), actor, d)
	ok(c, v, err)
}

func (h *Handler) deleteDimension(c *gin.Context) {
	actor, okA := actorOf(c)
	id, okI := idParam(c)
	if !okA || !okI {
		return
	}
	ok(c, gin.H{}, h.Setup.DeleteDimension(c.Request.Context(), actor, id))
}

func (h *Handler) reorderDimensions(c *gin.Context) {
	actor, okA := actorOf(c)
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if !okA || !bind(c, &req) {
		return
	}
	ok(c, gin.H{}, h.Setup.ReorderDimensions(c.Request.Context(), actor, req.IDs))
}

func (h *Handler) saveItem(c *gin.Context) {
	actor, okA := actorOf(c)
	var it Item
	if !okA || !bind(c, &it) {
		return
	}
	it.ID = 0
	if c.Param("id") != "" {
		id, okI := idParam(c)
		if !okI {
			return
		}
		it.ID = id
	}
	v, err := h.Setup.SaveItem(c.Request.Context(), actor, it)
	ok(c, v, err)
}

func (h *Handler) deleteItem(c *gin.Context) {
	actor, okA := actorOf(c)
	id, okI := idParam(c)
	if !okA || !okI {
		return
	}
	ok(c, gin.H{}, h.Setup.DeleteItem(c.Request.Context(), actor, id))
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
	ok(c, gin.H{"got": got, "pass": pass}, err)
}

func (h *Handler) savePlan(c *gin.Context) {
	actor, okA := actorOf(c)
	var p Plan
	if !okA || !bind(c, &p) {
		return
	}
	p.ID = 0
	if c.Param("id") != "" {
		id, okI := idParam(c)
		if !okI {
			return
		}
		p.ID = id
	}
	v, err := h.Setup.SavePlan(c.Request.Context(), actor, p)
	ok(c, v, err)
}

func (h *Handler) deletePlan(c *gin.Context) {
	actor, okA := actorOf(c)
	id, okI := idParam(c)
	if !okA || !okI {
		return
	}
	ok(c, gin.H{}, h.Setup.DeletePlan(c.Request.Context(), actor, id))
}

func (h *Handler) previewPlan(c *gin.Context) {
	var p Plan
	if !bind(c, &p) {
		return
	}
	v, err := h.Setup.PreviewPlan(c.Request.Context(), p)
	ok(c, v, err)
}

func (h *Handler) runPlan(c *gin.Context) {
	actor, okA := actorOf(c)
	id, okI := idParam(c)
	if !okA || !okI {
		return
	}
	n, err := h.Setup.RunPlanNow(c.Request.Context(), actor, id)
	ok(c, gin.H{"queued": n}, err)
}

// ---------- 查询 ----------

func (h *Handler) overview(c *gin.Context) {
	v, err := h.Query.Overview(c.Request.Context(), c.Query("platform"))
	ok(c, v, err)
}

func (h *Handler) accountDetail(c *gin.Context) {
	id, okI := idParam(c)
	if !okI {
		return
	}
	v, err := h.Query.AccountDetail(c.Request.Context(), qInt(c, "group"), id, c.Query("model"))
	ok(c, v, err)
}

func (h *Handler) checkup(c *gin.Context) {
	actor, okA := actorOf(c)
	id, okI := idParam(c)
	var req CheckupRequest
	if !okA || !okI || !bind(c, &req) {
		return
	}
	req.AccountID = id
	v, err := h.Setup.Checkup(c.Request.Context(), actor, req)
	ok(c, v, err)
}

func (h *Handler) compare(c *gin.Context) {
	v, err := h.Query.Compare(c.Request.Context(), qInt(c, "group"), c.Query("model"), qInt(c, "item"), c.Query("range"), qInt(c, "highlight"))
	ok(c, v, err)
}

func runFilterOf(c *gin.Context) RunFilter {
	return RunFilter{ItemID: qInt(c, "item"), PlanID: qInt(c, "plan"), AccountID: qInt(c, "account"), Model: c.Query("model"),
		Outcome: c.Query("outcome"), BeforeID: qInt(c, "before_id"), Limit: int(qInt(c, "limit"))}
}

func (h *Handler) runsBoard(c *gin.Context) {
	v, err := h.Query.RunsBoard(c.Request.Context(), runFilterOf(c), qInt(c, "group"), int(qInt(c, "n")))
	ok(c, v, err)
}

func (h *Handler) runsList(c *gin.Context) {
	v, err := h.Query.RunsList(c.Request.Context(), runFilterOf(c), qInt(c, "group"))
	ok(c, v, err)
}

func (h *Handler) runDetail(c *gin.Context) {
	id, okI := idParam(c)
	if !okI {
		return
	}
	v, err := h.Query.RunDetail(c.Request.Context(), id)
	ok(c, v, err)
}

func (h *Handler) rerun(c *gin.Context) {
	actor, okA := actorOf(c)
	id, okI := idParam(c)
	if !okA || !okI {
		return
	}
	r, err := h.Query.Runs.GetRun(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	nid, err := h.Setup.RerunOne(c.Request.Context(), actor, *r)
	ok(c, gin.H{"run_id": nid}, err)
}

func (h *Handler) events(c *gin.Context) {
	v, err := h.Query.Events(c.Request.Context(), c.Query("state"))
	ok(c, v, err)
}

func (h *Handler) eventState(c *gin.Context) {
	actor, okA := actorOf(c)
	id, okI := idParam(c)
	var req struct {
		State string `json:"state"`
	}
	if !okA || !okI || !bind(c, &req) {
		return
	}
	v, err := h.Adjust.SetEventState(c.Request.Context(), actor, id, req.State)
	ok(c, v, err)
}

func (h *Handler) suggestions(c *gin.Context) {
	v, err := h.Query.Suggestions(c.Request.Context())
	ok(c, v, err)
}

// ---------- 调整 ----------

func (h *Handler) adjustments(c *gin.Context) {
	v, err := h.Adjust.Active(c.Request.Context())
	ok(c, v, err)
}

func (h *Handler) applyAdjustment(c *gin.Context) {
	actor, okA := actorOf(c)
	var req AdjustRequest
	if !okA || !bind(c, &req) {
		return
	}
	v, err := h.Adjust.Apply(c.Request.Context(), actor, req)
	ok(c, v, err)
}

func (h *Handler) previewAdjustment(c *gin.Context) {
	var req AdjustRequest
	if !bind(c, &req) {
		return
	}
	v, err := h.Adjust.Preview(c.Request.Context(), req)
	ok(c, v, err)
}

func (h *Handler) adjustmentLogs(c *gin.Context) {
	v, err := h.Adjust.Logs(c.Request.Context(), qInt(c, "group"), qInt(c, "account"))
	ok(c, v, err)
}

func (h *Handler) effect(c *gin.Context) {
	id, okI := idParam(c)
	if !okI {
		return
	}
	v, err := h.Adjust.Effect(c.Request.Context(), id)
	ok(c, v, err)
}

func (h *Handler) badges(c *gin.Context) {
	v, err := h.Adjust.Badges(c.Request.Context())
	ok(c, v, err)
}

// ---------- 操作记录 ----------

func (h *Handler) audit(c *gin.Context) {
	f := AuditFilter{OperatorID: qInt(c, "operator"), TargetType: c.Query("target_type"), TargetID: c.Query("target_id"),
		Keyword: strings.TrimSpace(c.Query("q")), Page: int(qInt(c, "page")), PageSize: int(qInt(c, "page_size"))}
	if t, err := time.Parse(time.RFC3339, c.Query("since")); err == nil {
		f.Since = t
	}
	if t, err := time.Parse(time.RFC3339, c.Query("until")); err == nil {
		f.Until = t
	}
	list, total, err := h.Audit.ListAudit(c.Request.Context(), f)
	ok(c, gin.H{"items": list, "total": total}, err)
}
