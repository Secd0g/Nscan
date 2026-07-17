package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourname/nscan/internal/server/cron"
	"github.com/yourname/nscan/internal/server/cronjob"
	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func (h *Handler) ListScheduled(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	limit, skip := paginate(c)
	list, total, err := h.scheduled.ListForUser(c.Request.Context(), uid, limit, skip)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	listResp(c, list, total)
}

func (h *Handler) CreateScheduled(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	var req struct {
		Name         string                          `json:"name"        binding:"required"`
		ProjectID    string                          `json:"project_id"  binding:"required"`
		Cron         string                          `json:"cron"        binding:"required"`
		Targets      []string                        `json:"targets"     binding:"required,min=1"`
		Stages       []string                        `json:"stages"`
		Params       map[string]string               `json:"params"`
		Modules      map[string][]models.StagePlugin `json:"modules"`
		TemplateID   string                          `json:"template_id"`
		TemplateName string                          `json:"template_name"`
		NodeIDs      []string                        `json:"node_ids"`
		Enabled      *bool                           `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Stages) == 0 && len(req.Modules) == 0 {
		errResp(c, http.StatusBadRequest, "stages or modules required")
		return
	}
	projectID, err := primitive.ObjectIDFromHex(req.ProjectID)
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid project_id")
		return
	}
	if _, err := cron.Parse(req.Cron); err != nil {
		errResp(c, http.StatusBadRequest, "cron 表达式无效: "+err.Error())
		return
	}
	if req.Params == nil {
		req.Params = map[string]string{}
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	projectName := ""
	if p, err := h.projects.GetByIDForUser(c.Request.Context(), projectID, uid); err == nil {
		projectName = p.Name
	} else {
		errResp(c, http.StatusNotFound, "project not found")
		return
	}

	job := &models.ScheduledJob{
		UserID:       uid,
		Name:         req.Name,
		ProjectID:    projectID,
		ProjectName:  projectName,
		Cron:         req.Cron,
		Targets:      req.Targets,
		Stages:       req.Stages,
		Params:       req.Params,
		Modules:      req.Modules,
		TemplateID:   req.TemplateID,
		TemplateName: req.TemplateName,
		NodeIDs:      req.NodeIDs,
		Enabled:      enabled,
	}
	if enabled {
		job.NextRun = cronjob.NextRun(req.Cron, time.Now())
	}
	if err := h.scheduled.Create(c.Request.Context(), job); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusCreated, job)
}

func (h *Handler) UpdateScheduled(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Name         *string                         `json:"name"`
		ProjectID    *string                         `json:"project_id"`
		Cron         *string                         `json:"cron"`
		Targets      *[]string                       `json:"targets"`
		Stages       *[]string                       `json:"stages"`
		Params       *map[string]string              `json:"params"`
		Modules      map[string][]models.StagePlugin `json:"modules"`
		TemplateID   *string                         `json:"template_id"`
		TemplateName *string                         `json:"template_name"`
		NodeIDs      *[]string                       `json:"node_ids"`
		Enabled      *bool                           `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	ctx := c.Request.Context()
	existing, err := h.scheduled.GetByIDForUser(ctx, id, uid)
	if err == mongo.ErrNoDocuments {
		errResp(c, http.StatusNotFound, "scheduled job not found")
		return
	}
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}

	fields := bson.M{}
	if req.Name != nil {
		fields["name"] = *req.Name
	}
	if req.ProjectID != nil {
		pid, err := primitive.ObjectIDFromHex(*req.ProjectID)
		if err != nil {
			errResp(c, http.StatusBadRequest, "invalid project_id")
			return
		}
		fields["project_id"] = pid
		if p, err := h.projects.GetByIDForUser(ctx, pid, uid); err == nil {
			fields["project_name"] = p.Name
		}
	}
	if req.Targets != nil {
		fields["targets"] = *req.Targets
	}
	if req.Stages != nil {
		fields["stages"] = *req.Stages
	}
	if req.Params != nil {
		fields["params"] = *req.Params
	}
	if req.Modules != nil {
		fields["modules"] = req.Modules
	}
	if req.NodeIDs != nil {
		fields["node_ids"] = *req.NodeIDs
	}
	if req.TemplateID != nil {
		fields["template_id"] = *req.TemplateID
	}
	if req.TemplateName != nil {
		fields["template_name"] = *req.TemplateName
	}

	cronExpr := existing.Cron
	if req.Cron != nil {
		if _, err := cron.Parse(*req.Cron); err != nil {
			errResp(c, http.StatusBadRequest, "cron 表达式无效: "+err.Error())
			return
		}
		fields["cron"] = *req.Cron
		cronExpr = *req.Cron
	}
	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
		fields["enabled"] = enabled
	}
	if req.Cron != nil || req.Enabled != nil {
		if enabled {
			fields["next_run"] = cronjob.NextRun(cronExpr, time.Now())
		} else {
			fields["next_run"] = nil
		}
	}

	if err := h.scheduled.UpdateForUser(ctx, id, uid, fields); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	job, _ := h.scheduled.GetByIDForUser(ctx, id, uid)
	c.JSON(http.StatusOK, job)
}

func (h *Handler) DeleteScheduled(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.scheduled.DeleteForUser(c.Request.Context(), id, uid); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// RunScheduledNow 立即触发一次定时任务（不影响其 next_run 计划）。
func (h *Handler) RunScheduledNow(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	ctx := c.Request.Context()
	job, err := h.scheduled.GetByIDForUser(ctx, id, uid)
	if err == mongo.ErrNoDocuments {
		errResp(c, http.StatusNotFound, "scheduled job not found")
		return
	}
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	task := &models.Task{
		UserID:       uid,
		ProjectID:    job.ProjectID,
		Name:         job.Name + " (手动触发)",
		TemplateID:   job.TemplateID,
		TemplateName: job.TemplateName,
		Targets:      job.Targets,
		NodeIDs:      job.NodeIDs,
		Modules:      job.Modules,
		Config:       models.TaskConfig{Stages: job.Stages, Params: job.Params},
	}
	if err := h.sched.Submit(ctx, task); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"task_id": task.ID.Hex()})
}
