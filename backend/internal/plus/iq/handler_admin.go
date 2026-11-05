// Package iq — [plus] 管理端 HTTP（挂在 /api/v1/plus/admin，管理员鉴权）
package iq

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// AdminHandler 管理端「智商检测」
type AdminHandler struct{ svc *AdminService }

func NewAdminHandler(svc *AdminService) *AdminHandler { return &AdminHandler{svc: svc} }

// RegisterAdminRoutes 挂载到管理员路由组（/plus/admin）
func (h *AdminHandler) RegisterAdminRoutes(g *gin.RouterGroup) {
	ig := g.Group("/iq")
	ig.GET("/config", h.getConfig)
	ig.PUT("/config", h.saveConfig)
	ig.GET("/tasks", h.overview)
	ig.POST("/tasks", h.createTask)
	ig.PUT("/tasks/:id", h.updateTask)
	ig.DELETE("/tasks/:id", h.deleteTask)
	ig.POST("/tasks/test", h.testTask)
	ig.POST("/tasks/models", h.models)
	ig.POST("/run", h.run)
	ig.GET("/keys", h.keys)
	ig.GET("/runs", h.listRuns)
	ig.GET("/runs/:id", h.getRun)
	ig.GET("/gallery", h.gallery)
}

func adminID(c *gin.Context) (int64, bool) {
	s, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "not authenticated")
		return 0, false
	}
	return s.UserID, true
}

func idParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid id")
		return 0, false
	}
	return id, true
}

// fail 统一错误映射：校验错误 400，找不到 404，版本冲突 409，其他 500
func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		response.BadRequest(c, err.Error())
	case errors.Is(err, ErrNotFound):
		response.NotFound(c, "记录不存在")
	case errors.Is(err, ErrVersionMismatch):
		response.Error(c, http.StatusConflict, "配置已被他人修改，请刷新后重试")
	default:
		internalErr(c, err)
	}
}

func (h *AdminHandler) getConfig(c *gin.Context) {
	v, err := h.svc.Config(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, v)
}

func (h *AdminHandler) saveConfig(c *gin.Context) {
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
		fail(c, err)
		return
	}
	response.Success(c, gin.H{"version": ver})
}

func (h *AdminHandler) overview(c *gin.Context) {
	v, err := h.svc.Overview(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, v)
}

func (h *AdminHandler) createTask(c *gin.Context) {
	uid, ok := adminID(c)
	if !ok {
		return
	}
	var in TaskInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	id, err := h.svc.CreateTask(c.Request.Context(), in, uid)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}

func (h *AdminHandler) updateTask(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var in TaskInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	if err := h.svc.UpdateTask(c.Request.Context(), id, in); err != nil {
		fail(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}

func (h *AdminHandler) deleteTask(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteTask(c.Request.Context(), id); err != nil {
		fail(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}

func (h *AdminHandler) testTask(c *gin.Context) {
	uid, ok := adminID(c)
	if !ok {
		return
	}
	var in TaskInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	res, err := h.svc.TestTask(c.Request.Context(), in, uid)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, res)
}

func (h *AdminHandler) models(c *gin.Context) {
	uid, ok := adminID(c)
	if !ok {
		return
	}
	var in TaskInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	models, err := h.svc.Models(c.Request.Context(), in, uid)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, models)
}

// run POST /plus/admin/iq/run  Body: {task_id?, group_id?}；都不填表示全部启用的任务
func (h *AdminHandler) run(c *gin.Context) {
	uid, ok := adminID(c)
	if !ok {
		return
	}
	var req struct {
		TaskID  int64 `json:"task_id"`
		GroupID int64 `json:"group_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	queued, skipped, err := h.svc.RunTasks(c.Request.Context(), req.TaskID, req.GroupID, uid)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, gin.H{"queued": queued, "skipped": skipped})
}

func (h *AdminHandler) keys(c *gin.Context) {
	uid, ok := adminID(c)
	if !ok {
		return
	}
	gid, _ := strconv.ParseInt(c.Query("group_id"), 10, 64)
	v, err := h.svc.KeyChoices(c.Request.Context(), gid, uid)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, v)
}

func (h *AdminHandler) listRuns(c *gin.Context) {
	q := func(k string) int64 { v, _ := strconv.ParseInt(c.Query(k), 10, 64); return v }
	f := RunFilter{
		GroupID: q("group_id"), TaskID: q("task_id"), AccountID: q("account_id"),
		Status: RunStatus(c.Query("status")), Trigger: Trigger(c.Query("trigger")),
		Page: int(q("page")), PageSize: int(q("page_size")),
	}
	runs, total, err := h.svc.ListRuns(c.Request.Context(), f)
	if err != nil {
		fail(c, err)
		return
	}
	if f.PageSize <= 0 || f.PageSize > 100 {
		f.PageSize = 20
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	response.Paginated(c, runs, total, f.Page, f.PageSize)
}

func (h *AdminHandler) getRun(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	v, err := h.svc.GetRun(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, v)
}

func (h *AdminHandler) gallery(c *gin.Context) {
	gid, _ := strconv.ParseInt(c.Query("group_id"), 10, 64)
	n, _ := strconv.Atoi(c.Query("per_task"))
	v, err := h.svc.Gallery(c.Request.Context(), gid, n)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, v)
}
