package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) recipeByID(id string) (Recipe, error) {
	var recipe Recipe
	var outputs string
	var enabled int
	err := h.db.QueryRow(`SELECT id, name, version, outputs, prompt_template_id, estimated_cost_per_job,
		currency, created_by, enabled FROM recipes WHERE id=?`, id).
		Scan(&recipe.ID, &recipe.Name, &recipe.Version, &outputs, &recipe.PromptTemplateID,
			&recipe.EstimatedCostPerJob, &recipe.Currency, &recipe.CreatedBy, &enabled)
	recipe.Enabled = enabled != 0
	_ = json.Unmarshal([]byte(outputs), &recipe.Outputs)
	return recipe, err
}

func (h *Handler) ListRecipes(c *gin.Context) {
	rows, err := h.db.Query(`SELECT id, name, version, outputs, prompt_template_id, estimated_cost_per_job,
		currency, created_by, enabled FROM recipes WHERE enabled=1 ORDER BY name`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取 Recipe 失败"})
		return
	}
	defer rows.Close()
	items := []Recipe{}
	for rows.Next() {
		var recipe Recipe
		var outputs string
		var enabled int
		if err := rows.Scan(&recipe.ID, &recipe.Name, &recipe.Version, &outputs, &recipe.PromptTemplateID,
			&recipe.EstimatedCostPerJob, &recipe.Currency, &recipe.CreatedBy, &enabled); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取 Recipe 失败"})
			return
		}
		recipe.Enabled = enabled != 0
		_ = json.Unmarshal([]byte(outputs), &recipe.Outputs)
		items = append(items, recipe)
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) CreateRecipeRun(c *gin.Context) {
	project, err := h.studioProjectByID(c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "项目不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取项目失败"})
		return
	}
	var request RecipePlanRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "生成计划格式不正确"})
		return
	}
	request.RecipeID = strings.TrimSpace(request.RecipeID)
	request.RequestID = strings.TrimSpace(request.RequestID)
	if request.RecipeID == "" || request.RequestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Recipe 和 requestId 不能为空"})
		return
	}
	if existing, err := h.recipeRunByRequestID(request.RequestID); err == nil {
		if existing.ProjectID != project.ID {
			c.JSON(http.StatusConflict, gin.H{"error": "requestId 已被其他项目使用"})
			return
		}
		c.Header("X-Idempotent-Replay", "true")
		c.JSON(http.StatusOK, existing)
		return
	}
	recipe, err := h.recipeByID(request.RecipeID)
	if errors.Is(err, sql.ErrNoRows) || !recipe.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Recipe 不存在或未启用"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取 Recipe 失败"})
		return
	}
	pack, err := h.loadProductPack(project.ID)
	if err != nil || pack == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先完善 Product Pack"})
		return
	}
	jobs, err := planRecipeJobs(project, *pack, recipe, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	runID := "run_" + uuid.New().String()[:8]
	createdBy := request.CreatedBy
	if createdBy == "" {
		createdBy = project.CreatedBy
	}
	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建生成计划失败"})
		return
	}
	defer tx.Rollback()
	totalCost := float64(len(jobs)) * recipe.EstimatedCostPerJob
	var layoutOriginX float64
	_ = tx.QueryRow(`SELECT COALESCE(MAX(x + width), 0) + 200 FROM nodes WHERE canvas_id=?`, project.CanvasID).Scan(&layoutOriginX)
	if layoutOriginX < 200 {
		layoutOriginX = 200
	}
	_, err = tx.Exec(`INSERT INTO recipe_runs (id, project_id, recipe_id, recipe_version, request_id, status, job_count, total_cost, created_by, layout_origin_x, layout_origin_y)
		VALUES (?, ?, ?, ?, ?, 'queued', ?, ?, ?, ?, 200)`, runID, project.ID, recipe.ID, recipe.Version, request.RequestID, len(jobs), totalCost, createdBy, layoutOriginX)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "相同 requestId 的生成计划已存在"})
		return
	}
	for index := range jobs {
		jobs[index].RecipeRunID = runID
		job := jobs[index]
		_, err = tx.Exec(`INSERT INTO generation_jobs (id, project_id, recipe_run_id, sku_id, output_type, status,
			provider, model, prompt, prompt_version, reference_pack, aspect_ratio, created_by, estimated_cost)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, job.ID, job.ProjectID, runID, job.SKUID,
			job.OutputType, job.Status, job.Provider, job.Model, job.Prompt, job.PromptVersion,
			encodeJSON(job.ReferencePack, "{}"), job.AspectRatio, job.CreatedBy, job.EstimatedCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建生成任务失败"})
			return
		}
	}
	if _, err = tx.Exec(`UPDATE studio_projects SET status='generating', updated_at=datetime('now','localtime') WHERE id=?`, project.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新项目状态失败"})
		return
	}
	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存生成计划失败"})
		return
	}
	run, _ := h.recipeRunByRequestID(request.RequestID)
	h.enqueueRecipeRun(run.ID)
	c.JSON(http.StatusCreated, run)
}

func (h *Handler) GetRecipeRun(c *gin.Context) {
	run, err := h.recipeRunByID(c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "生成批次不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取生成批次失败"})
		return
	}
	c.JSON(http.StatusOK, run)
}

func (h *Handler) ListProjectRecipeRuns(c *gin.Context) {
	rows, err := h.db.Query(`SELECT id FROM recipe_runs WHERE project_id=? ORDER BY created_at DESC LIMIT 20`, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取生成历史失败"})
		return
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	runs := make([]RecipeRun, 0, len(ids))
	for _, id := range ids {
		run, loadErr := h.recipeRunByID(id)
		if loadErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取生成历史失败"})
			return
		}
		runs = append(runs, run)
	}
	c.JSON(http.StatusOK, runs)
}

func (h *Handler) RetryFailedJobs(c *gin.Context) {
	runID := c.Param("id")
	result, err := h.db.Exec(`UPDATE generation_jobs SET status='queued', retry_count=retry_count+1,
		error_code='', error_message='', started_at='', finished_at='' WHERE recipe_run_id=? AND status IN ('failed','rejected')`, runID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重试失败任务失败"})
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有可重试的失败任务"})
		return
	}
	_, _ = h.db.Exec(`UPDATE recipe_runs SET status='queued', finished_at='' WHERE id=?`, runID)
	run, err := h.recipeRunByID(runID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "生成批次不存在"})
		return
	}
	h.enqueueRecipeRun(runID)
	c.JSON(http.StatusOK, gin.H{"retried": count, "run": run})
}

func (h *Handler) RetryGenerationJob(c *gin.Context) {
	job, err := h.generationJobByID(c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "生成任务不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取生成任务失败"})
		return
	}
	if job.Status == "queued" || job.Status == "running" || job.Status == "retrying" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "任务正在处理中"})
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重新生成失败"})
		return
	}
	defer tx.Rollback()
	queuedJobID := job.ID
	if job.Status == "failed" || job.Status == "rejected" {
		_, err = tx.Exec(`UPDATE generation_jobs SET status='queued', retry_count=retry_count+1,
			result_asset_id='', error_code='', error_message='', started_at='', finished_at=''
			WHERE id=?`, job.ID)
	} else {
		queuedJobID = "job_" + uuid.New().String()[:8]
		_, err = tx.Exec(`INSERT INTO generation_jobs (id, project_id, recipe_run_id, sku_id, output_type, status,
			provider, model, prompt, prompt_version, reference_pack, aspect_ratio, created_by, estimated_cost, retry_count)
			VALUES (?, ?, ?, ?, ?, 'queued', ?, ?, ?, ?, ?, ?, ?, ?, ?)`, queuedJobID, job.ProjectID,
			job.RecipeRunID, job.SKUID, job.OutputType, job.Provider, job.Model, job.Prompt, job.PromptVersion,
			encodeJSON(job.ReferencePack, "{}"), job.AspectRatio, job.CreatedBy, job.EstimatedCost, job.RetryCount+1)
		if err == nil {
			_, err = tx.Exec(`UPDATE recipe_runs SET job_count=job_count+1, total_cost=total_cost+?, status='queued', finished_at=''
				WHERE id=?`, job.EstimatedCost, job.RecipeRunID)
		}
	}
	if err == nil && (job.Status == "failed" || job.Status == "rejected") {
		_, err = tx.Exec(`UPDATE recipe_runs SET status='queued', finished_at='' WHERE id=?`, job.RecipeRunID)
	}
	if err != nil || tx.Commit() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重新生成失败"})
		return
	}
	h.enqueueRecipeRun(job.RecipeRunID)
	run, _ := h.recipeRunByID(job.RecipeRunID)
	c.JSON(http.StatusOK, gin.H{"job_id": queuedJobID, "run": run})
}

func (h *Handler) CancelGenerationJob(c *gin.Context) {
	jobID := c.Param("id")
	var status string
	if err := h.db.QueryRow(`SELECT status FROM generation_jobs WHERE id=?`, jobID).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "生成任务不存在"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取生成任务失败"})
		return
	}
	if status != "queued" && status != "running" && status != "retrying" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "只有排队或运行中的任务可以取消"})
		return
	}
	if h.jobQueue != nil {
		h.jobQueue.Cancel(jobID)
	}
	_, err := h.db.Exec(`UPDATE generation_jobs SET status='cancelled', finished_at=datetime('now','localtime'),
		error_code='cancelled', error_message='用户取消' WHERE id=? AND status IN ('queued','running','retrying')`, jobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "取消生成任务失败"})
		return
	}
	h.refreshRecipeRunStatusByJob(jobID)
	c.JSON(http.StatusOK, gin.H{"id": jobID, "status": "cancelled"})
}

func (h *Handler) recipeRunByRequestID(requestID string) (RecipeRun, error) {
	var id string
	if err := h.db.QueryRow(`SELECT id FROM recipe_runs WHERE request_id=?`, requestID).Scan(&id); err != nil {
		return RecipeRun{}, err
	}
	return h.recipeRunByID(id)
}
