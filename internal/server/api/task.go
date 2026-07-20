package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourname/nscan/internal/server/scheduler"
	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func (h *Handler) ListTasks(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	limit, skip := paginate(c)
	var projectID *primitive.ObjectID
	if pid := c.Query("project_id"); pid != "" {
		id, err := primitive.ObjectIDFromHex(pid)
		if err != nil {
			errResp(c, http.StatusBadRequest, "invalid project_id")
			return
		}
		projectID = &id
	}
	status := c.Query("status")
	keyword := c.Query("keyword")
	list, total, err := h.tasks.ListForUser(c.Request.Context(), uid, projectID, status, keyword, limit, skip)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	listResp(c, list, total)
}

func (h *Handler) CreateTask(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	var req struct {
		ProjectID         string                          `json:"project_id"    binding:"required"`
		Name              string                          `json:"name"          binding:"required"`
		Targets           []string                        `json:"targets"       binding:"required,min=1"`
		Stages            []string                        `json:"stages"`
		Modules           map[string][]models.StagePlugin `json:"modules"`
		Params            map[string]string               `json:"params"`
		TemplateID        string                          `json:"template_id"`
		TemplateName      string                          `json:"template_name"`
		NodeIDs           []string                        `json:"node_ids"`
		AIAnalysisEnabled bool                            `json:"ai_analysis_enabled"`
		AIPentestEnabled  bool                            `json:"ai_pentest_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}

	if len(req.Modules) > 0 {
		req.Stages = []string{}
		if req.Params == nil {
			req.Params = map[string]string{}
		}

		pluginToStage := map[string]string{
			"subfinder":    "subdomain",
			"ksubdomain":   "subdomain",
			"shuffledns":   "shuffledns",
			"bbot":         "bbot",
			"findomain":    "findomain",
			"naabu":        "port",
			"httpx":        "http",
			"fingerprint":  "http",
			"nuclei":       "nuclei",
			"dirscan":      "dir",
			"brutescan":    "brute",
			"onlinesearch": "search",
			"crawler":      "crawler",
			"sensitive":    "sensitive",
		}

		moduleOrder := []string{"search", "subdomain", "port", "http", "crawler", "vuln", "brute", "dir", "sensitive"}
		for _, mod := range moduleOrder {
			plugins, ok := req.Modules[mod]
			if !ok {
				continue
			}
			for _, sp := range plugins {
				if !sp.Enabled {
					continue
				}
				stageName := sp.Name
				if mapped, exists := pluginToStage[sp.Name]; exists {
					stageName = mapped
				}
				found := false
				for _, s := range req.Stages {
					if s == stageName {
						found = true
						break
					}
				}
				if !found {
					req.Stages = append(req.Stages, stageName)
				}
				for k, v := range sp.Params {
					req.Params[stageName+"."+k] = flattenParamValue(v)
				}
				if sp.Name == "brutescan" {
					if svcs, ok := req.Params["brute.services"]; ok && svcs != "" {
						req.Params["brute.protocols"] = svcs
					}
				}
			}
		}
	}

	if len(req.Stages) == 0 {
		errResp(c, http.StatusBadRequest, "stages or modules required")
		return
	}

	projectID, err := primitive.ObjectIDFromHex(req.ProjectID)
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid project_id")
		return
	}
	if _, err := h.projects.GetByIDForUser(c.Request.Context(), projectID, uid); err != nil {
		errResp(c, http.StatusNotFound, "project not found")
		return
	}
	if req.Params == nil {
		req.Params = map[string]string{}
	}
	task := &models.Task{
		UserID:            uid,
		ProjectID:    projectID,
		Name:         req.Name,
		TemplateName: req.TemplateName,
		TemplateID:   req.TemplateID,
		Targets:      req.Targets,
		NodeIDs:      req.NodeIDs,
		Modules:      req.Modules,
		Config: models.TaskConfig{
			Stages: req.Stages,
			Params: req.Params,
		},
		AIAnalysisEnabled: req.AIAnalysisEnabled,
		AIPentestEnabled:  req.AIPentestEnabled,
	}
	if err := h.sched.Submit(c.Request.Context(), task); err != nil {
		if errors.Is(err, scheduler.ErrTaskNameDuplicate) {
			errResp(c, http.StatusConflict, "任务名「"+req.Name+"」已存在，请换一个名字")
			return
		}
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusCreated, task)
}

func (h *Handler) GetTask(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	task, err := h.tasks.GetByIDForUser(c.Request.Context(), id, uid)
	if err == mongo.ErrNoDocuments {
		errResp(c, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (h *Handler) UpdateTask(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	task, err := h.tasks.GetByIDForUser(c.Request.Context(), id, uid)
	if err == mongo.ErrNoDocuments {
		errResp(c, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	if task.Status == models.TaskStatusRunning || task.Status == models.TaskStatusDispatched {
		errResp(c, http.StatusBadRequest, "cannot edit a running task")
		return
	}
	var req struct {
		Name    *string  `json:"name"`
		Targets []string `json:"targets"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	update := bson.M{"updated_at": time.Now()}
	if req.Name != nil && *req.Name != "" {
		update["name"] = *req.Name
	}
	if len(req.Targets) > 0 {
		update["targets"] = req.Targets
	}
	if err := h.tasks.UpdateForUser(c.Request.Context(), id, uid, update); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	updated, _ := h.tasks.GetByIDForUser(c.Request.Context(), id, uid)
	c.JSON(http.StatusOK, updated)
}

func (h *Handler) DeleteTask(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	task, _ := h.tasks.GetByIDForUser(c.Request.Context(), id, uid)
	if task == nil {
		errResp(c, http.StatusNotFound, "task not found")
		return
	}
	if err := h.tasks.DeleteForUser(c.Request.Context(), id, uid); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	h.sched.CancelCleanup(c.Request.Context(), id.Hex())
	if task != nil && (task.Status == models.TaskStatusRunning || task.Status == models.TaskStatusDispatched || task.Status == models.TaskStatusPending || task.Status == models.TaskStatusQueued) {
		// Queue-mode tasks are not pinned to a NodeID; broadcast so the node
		// currently executing a leased subtask cancels its engine context.
		_ = h.nm.SendCancelTask(id.Hex())
	}
	if c.Query("with_assets") == "true" {
		_ = h.assets.DeleteByTaskID(c.Request.Context(), id.Hex())
		if task != nil {
			_ = h.assets.DeleteOrphansByProject(c.Request.Context(), task.ProjectID.Hex())
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (h *Handler) CancelTask(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	task, err := h.tasks.GetByIDForUser(c.Request.Context(), id, uid)
	if err == mongo.ErrNoDocuments {
		errResp(c, http.StatusNotFound, "task not found")
		return
	} else if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}

	if task.Status != models.TaskStatusRunning && task.Status != models.TaskStatusDispatched &&
		task.Status != models.TaskStatusPending && task.Status != models.TaskStatusQueued {
		errResp(c, http.StatusBadRequest, "task is not running")
		return
	}

	h.nm.SendCancelTask(id.Hex())
	h.tasks.UpdateForUser(c.Request.Context(), id, uid, bson.M{
		"status":  models.TaskStatusFailed,
		"error":   "user cancelled",
		"done_at": time.Now(),
	})
	// Clean up Redis state so a subsequent rescan starts fresh.
	h.sched.CancelCleanup(c.Request.Context(), id.Hex())
	c.JSON(http.StatusOK, gin.H{"message": "task cancelled"})
}

func (h *Handler) RescanTask(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := h.tasks.GetByIDForUser(c.Request.Context(), id, uid); err == mongo.ErrNoDocuments {
		errResp(c, http.StatusNotFound, "task not found")
		return
	} else if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.sched.Rescan(c.Request.Context(), id); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	task, _ := h.tasks.GetByIDForUser(c.Request.Context(), id, uid)
	c.JSON(http.StatusOK, task)
}

func (h *Handler) BatchDeleteTasks(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	var req struct {
		IDs        []string `json:"ids"`
		WithAssets bool     `json:"with_assets"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		errResp(c, http.StatusBadRequest, "ids required")
		return
	}
	if checkBatchLimit(c, req.IDs) {
		return
	}
	ctx := c.Request.Context()
	touchedProjects := map[string]struct{}{}
	for _, idStr := range req.IDs {
		id, err := primitive.ObjectIDFromHex(idStr)
		if err != nil {
			continue
		}
		task, _ := h.tasks.GetByIDForUser(ctx, id, uid)
		if task == nil { continue }
		_ = h.tasks.DeleteForUser(ctx, id, uid)
		h.sched.CancelCleanup(ctx, idStr)
		if task != nil && (task.Status == models.TaskStatusRunning || task.Status == models.TaskStatusDispatched || task.Status == models.TaskStatusPending || task.Status == models.TaskStatusQueued) {
			_ = h.nm.SendCancelTask(idStr)
		}
		if req.WithAssets {
			_ = h.assets.DeleteByTaskID(ctx, idStr)
			if task != nil {
				touchedProjects[task.ProjectID.Hex()] = struct{}{}
			}
		}
	}
	if req.WithAssets {
		for pid := range touchedProjects {
			_ = h.assets.DeleteOrphansByProject(ctx, pid)
		}
	}
	c.JSON(http.StatusOK, gin.H{"deleted": len(req.IDs)})
}

// flattenParamValue 将插件参数值扁平化为字符串（送 gRPC map[string]string 用）
func flattenParamValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case []interface{}:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			} else {
				parts = append(parts, fmt.Sprint(item))
			}
		}
		return strings.Join(parts, ",")
	case bool:
		if val {
			return "true"
		}
		return "false"
	case float64:
		return fmt.Sprintf("%g", val)
	case nil:
		return ""
	default:
		return fmt.Sprint(val)
	}
}

func (h *Handler) GetTaskLogs(c *gin.Context) {
	id := c.Param("id")
	logs := h.sched.GetTaskLogs(id)
	c.JSON(http.StatusOK, gin.H{"data": logs})
}

func (h *Handler) ListSubtasks(c *gin.Context) {
	id := c.Param("id")
	subtasks, err := h.sched.ListSubtasks(c.Request.Context(), id)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	if subtasks == nil {
		subtasks = []*models.Subtask{}
	}
	c.JSON(http.StatusOK, gin.H{"data": subtasks})
}

// ListDeadLetterByTask returns dead-lettered subtasks for a task.
// GET /api/v1/tasks/:id/dead-letter
func (h *Handler) ListDeadLetterByTask(c *gin.Context) {
	id := c.Param("id")
	items, err := h.sched.ListDeadLetterByTask(c.Request.Context(), id)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []*models.Subtask{}
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

// RetryDeadLetter re-enqueues a dead-lettered subtask.
// POST /api/v1/dead-letter/:subtaskId/retry
func (h *Handler) RetryDeadLetter(c *gin.Context) {
	subtaskID := c.Param("subtaskId")
	if err := h.sched.RetryDeadLetter(c.Request.Context(), subtaskID); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "requeued"})
}
