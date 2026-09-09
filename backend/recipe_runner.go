package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func (h *Handler) resumeQueuedRecipeJobs() {
	if h.jobQueue == nil {
		return
	}
	rows, err := h.db.Query(`SELECT DISTINCT recipe_run_id FROM generation_jobs WHERE status IN ('queued','retrying')`)
	if err != nil {
		h.log.Error("resume queued recipe jobs", "error", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var runID string
		if rows.Scan(&runID) == nil {
			h.enqueueRecipeRun(runID)
		}
	}
}

func (h *Handler) enqueueRecipeRun(runID string) {
	if h.jobQueue == nil || runID == "" {
		return
	}
	rows, err := h.db.Query(`SELECT id FROM generation_jobs WHERE recipe_run_id=? AND status IN ('queued','retrying')`, runID)
	if err != nil {
		h.log.Error("load queued recipe jobs", "run_id", runID, "error", err)
		return
	}
	defer rows.Close()
	var jobIDs []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			jobIDs = append(jobIDs, id)
		}
	}
	for _, jobID := range jobIDs {
		result, queueErr := h.jobQueue.Enqueue(context.Background(), QueueJob{ID: jobID, Execute: func(ctx context.Context) error {
			return h.executeRecipeJob(ctx, jobID)
		}})
		if queueErr != nil && !errors.Is(queueErr, errJobAlreadyQueued) {
			h.log.Error("enqueue recipe job", "job_id", jobID, "error", queueErr)
			continue
		}
		if result != nil {
			go func(id string, done <-chan error) {
				if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
					h.log.Warn("recipe job finished with error", "job_id", id, "error", err)
				}
			}(jobID, result)
		}
	}
}

func (h *Handler) executeRecipeJob(ctx context.Context, jobID string) error {
	job, err := h.generationJobByID(jobID)
	if err != nil {
		return err
	}
	started := time.Now()
	result, err := h.db.Exec(`UPDATE generation_jobs SET status='running', started_at=datetime('now','localtime'),
		error_code='', error_message='' WHERE id=? AND status IN ('queued','retrying')`, jobID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return nil
	}
	_, _ = h.db.Exec(`UPDATE recipe_runs SET status='running', started_at=CASE WHEN started_at='' THEN datetime('now','localtime') ELSE started_at END WHERE id=?`, job.RecipeRunID)
	_, _ = h.db.Exec(`UPDATE studio_projects SET status='generating', updated_at=datetime('now','localtime') WHERE id=?`, job.ProjectID)

	assets, generationErr := h.generateRecipeJobImages(ctx, job)
	if generationErr != nil {
		if errors.Is(generationErr, context.Canceled) {
			_, _ = h.db.Exec(`UPDATE generation_jobs SET status='cancelled', finished_at=datetime('now','localtime'), duration_ms=?, error_code='cancelled', error_message='用户取消' WHERE id=?`, time.Since(started).Milliseconds(), jobID)
		} else {
			_, _ = h.db.Exec(`UPDATE generation_jobs SET status='failed', finished_at=datetime('now','localtime'), duration_ms=?, error_code='provider_error', error_message=? WHERE id=?`, time.Since(started).Milliseconds(), friendlyGenerationError(generationErr), jobID)
		}
		h.refreshRecipeRunStatus(job.RecipeRunID, job.ProjectID)
		return generationErr
	}
	if len(assets) == 0 {
		return h.failRecipeJob(job, started, "empty_result", errors.New("生成服务没有返回图片"))
	}
	if err := ctx.Err(); err != nil {
		_, _ = h.db.Exec(`UPDATE generation_jobs SET status='cancelled', finished_at=datetime('now','localtime'), duration_ms=?, error_code='cancelled', error_message='用户取消' WHERE id=?`, time.Since(started).Milliseconds(), jobID)
		h.refreshRecipeRunStatus(job.RecipeRunID, job.ProjectID)
		return err
	}
	asset := assets[0]
	_, _ = h.db.Exec(`UPDATE generation_jobs SET status='generated' WHERE id=?`, jobID)
	metadata := map[string]any{
		"projectId": job.ProjectID, "skuId": job.SKUID, "recipeRunId": job.RecipeRunID,
		"generationJobId": job.ID, "provider": job.Provider, "model": job.Model,
		"prompt": job.Prompt, "promptVersion": job.PromptVersion, "referencePack": job.ReferencePack,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = h.db.Exec(`UPDATE assets SET project_id=?, sku_id=?, created_by=?, generation_metadata=? WHERE id=?`,
		job.ProjectID, job.SKUID, job.CreatedBy, string(metadataJSON), asset.ID)
	if err != nil {
		return h.failRecipeJob(job, started, "asset_lineage_error", err)
	}
	if err := h.layoutRecipeAssetNode(job, asset, metadata); err != nil {
		return h.failRecipeJob(job, started, "canvas_layout_error", err)
	}
	_, _ = h.db.Exec(`UPDATE generation_jobs SET status='validating' WHERE id=?`, jobID)
	if asset.Width <= 0 || asset.Height <= 0 {
		return h.failRecipeJob(job, started, "image_validation_error", errors.New("生成图片尺寸无效"))
	}
	_, err = h.db.Exec(`UPDATE generation_jobs SET status='success', result_asset_id=?, actual_cost=estimated_cost,
		finished_at=datetime('now','localtime'), duration_ms=? WHERE id=?`, asset.ID, time.Since(started).Milliseconds(), jobID)
	if err != nil {
		return err
	}
	h.refreshRecipeRunStatus(job.RecipeRunID, job.ProjectID)
	h.log.Info("recipe job succeeded", "job_id", job.ID, "run_id", job.RecipeRunID, "project_id", job.ProjectID,
		"sku_id", job.SKUID, "provider", job.Provider, "asset_id", asset.ID, "duration_ms", time.Since(started).Milliseconds())
	return nil
}

func (h *Handler) layoutRecipeAssetNode(job GenerationJob, asset GeneratedAsset, metadata map[string]any) error {
	var canvasID, outputsJSON string
	var originX, originY float64
	err := h.db.QueryRow(`SELECT p.canvas_id, rr.layout_origin_x, rr.layout_origin_y, r.outputs
		FROM recipe_runs rr JOIN studio_projects p ON p.id=rr.project_id JOIN recipes r ON r.id=rr.recipe_id WHERE rr.id=?`, job.RecipeRunID).
		Scan(&canvasID, &originX, &originY, &outputsJSON)
	if err != nil {
		return err
	}
	var skuOrder int
	_ = h.db.QueryRow(`SELECT sort_order FROM product_skus WHERE id=?`, job.SKUID).Scan(&skuOrder)
	var outputs []RecipeOutput
	_ = json.Unmarshal([]byte(outputsJSON), &outputs)
	outputOrder := 0
	for index, output := range outputs {
		if output.OutputType == job.OutputType && output.AspectRatio == job.AspectRatio {
			outputOrder = index
			break
		}
	}
	width, height := assetNodeDimensions(asset.Width, asset.Height)
	x := originX + float64(outputOrder)*390
	y := originY + float64(skuOrder)*420
	content, _ := json.Marshal(map[string]any{
		"asset_id": asset.ID, "url": asset.URL, "name": asset.Filename, "size": asset.Size,
		"mime_type": asset.MimeType, "width": asset.Width, "height": asset.Height, "origin": "generated",
		"recipe_run_id": job.RecipeRunID, "generation_job_id": job.ID, "generation": metadata,
	})
	nodeID := "nd_" + strings.TrimPrefix(job.ID, "job_")
	config, _ := json.Marshal(map[string]any{"recipe_run_id": job.RecipeRunID, "sku_id": job.SKUID, "output_type": job.OutputType})
	_, err = h.db.Exec(`INSERT OR IGNORE INTO nodes (id, canvas_id, node_type, x, y, width, height, content, config)
		VALUES (?, ?, 'asset', ?, ?, ?, ?, ?, ?)`, nodeID, canvasID, x, y, width, height, string(content), string(config))
	return err
}

func assetNodeDimensions(imageWidth, imageHeight int) (float64, float64) {
	const shortSide = 300.0
	if imageWidth <= 0 || imageHeight <= 0 {
		return shortSide, shortSide
	}
	if imageWidth >= imageHeight {
		return shortSide * float64(imageWidth) / float64(imageHeight), shortSide
	}
	return shortSide, shortSide * float64(imageHeight) / float64(imageWidth)
}

func (h *Handler) generateRecipeJobImages(ctx context.Context, job GenerationJob) ([]GeneratedAsset, error) {
	references, err := h.referencePackURLs(job.ReferencePack)
	if err != nil {
		return nil, err
	}
	model := strings.TrimSpace(job.Model)
	if model == "" {
		model = h.imageModelFast
	}
	providerID := strings.TrimSpace(job.Provider)
	if providerID == "" {
		providerID = "legacy"
	}
	provider := h.generationProviders[providerID]
	if provider == nil && providerID == "legacy" {
		provider = NewLegacyProvider(h)
	}
	if provider == nil {
		return nil, fmt.Errorf("生成 Provider %q 未配置", providerID)
	}
	images, err := provider.Generate(ctx, GenerationProviderRequest{JobID: job.ID, Prompt: job.Prompt,
		Model: model, AspectRatio: job.AspectRatio, Resolution: "2K", ReferenceImages: references})
	if err != nil {
		return nil, err
	}
	return h.persistGeneratedImages(images, "fast")
}

func (h *Handler) generateLegacyRecipeImages(ctx context.Context, request GenerationProviderRequest) ([]geminiInlineData, error) {
	if h.imageProvider != nil {
		return h.imageProvider.Generate(ctx, ImageProviderRequest{TaskID: request.JobID, Prompt: request.Prompt,
			Model: request.Model, Profile: "fast", AspectRatio: request.AspectRatio, Resolution: request.Resolution,
			N: 1, ReferenceImages: request.ReferenceImages})
	}
	if h.vectorEngine == nil || !h.vectorEngine.Configured() {
		return nil, ErrVectorEngineNotConfigured
	}
	payload, err := buildGeminiImagePayload(VectorImageRequest{TaskID: request.JobID, Prompt: request.Prompt,
		AspectRatio: request.AspectRatio, ImageSize: request.Resolution, N: 1, ReferenceImages: request.ReferenceImages}, h.assetUploadDir())
	if err != nil {
		return nil, err
	}
	response, err := h.vectorEngine.PostGeminiJSON(ctx, "/v1beta/models/"+url.PathEscape(request.Model)+":generateContent", payload)
	if err != nil {
		return nil, err
	}
	images, err := normalizeGeminiImages(response)
	if err != nil {
		return nil, err
	}
	return images[:1], nil
}

func (h *Handler) referencePackURLs(pack ReferencePack) ([]string, error) {
	urls := make([]string, 0, len(pack.References))
	for _, reference := range pack.References {
		var assetURL string
		if err := h.db.QueryRow(`SELECT url FROM assets WHERE id=? AND deleted_at=''`, reference.AssetID).Scan(&assetURL); err != nil {
			return nil, fmt.Errorf("参考资产 %s 不可用: %w", reference.AssetID, err)
		}
		urls = append(urls, assetURL)
	}
	return urls, nil
}

func (h *Handler) generationJobByID(id string) (GenerationJob, error) {
	var job GenerationJob
	var packJSON string
	err := h.db.QueryRow(`SELECT id, project_id, recipe_run_id, sku_id, output_type, status, provider, model,
		prompt, prompt_version, reference_pack, aspect_ratio, created_by, estimated_cost, retry_count
		FROM generation_jobs WHERE id=?`, id).Scan(&job.ID, &job.ProjectID, &job.RecipeRunID, &job.SKUID,
		&job.OutputType, &job.Status, &job.Provider, &job.Model, &job.Prompt, &job.PromptVersion,
		&packJSON, &job.AspectRatio, &job.CreatedBy, &job.EstimatedCost, &job.RetryCount)
	_ = json.Unmarshal([]byte(packJSON), &job.ReferencePack)
	return job, err
}

func (h *Handler) failRecipeJob(job GenerationJob, started time.Time, code string, cause error) error {
	_, _ = h.db.Exec(`UPDATE generation_jobs SET status='failed', finished_at=datetime('now','localtime'),
		duration_ms=?, error_code=?, error_message=? WHERE id=?`, time.Since(started).Milliseconds(), code, friendlyGenerationError(cause), job.ID)
	h.refreshRecipeRunStatus(job.RecipeRunID, job.ProjectID)
	return cause
}

func (h *Handler) refreshRecipeRunStatusByJob(jobID string) {
	var runID, projectID string
	if h.db.QueryRow(`SELECT recipe_run_id, project_id FROM generation_jobs WHERE id=?`, jobID).Scan(&runID, &projectID) == nil {
		h.refreshRecipeRunStatus(runID, projectID)
	}
}

func (h *Handler) refreshRecipeRunStatus(runID, projectID string) {
	var active, failed, success int
	_ = h.db.QueryRow(`SELECT
		SUM(CASE WHEN status IN ('queued','retrying','running','generated','validating') THEN 1 ELSE 0 END),
		SUM(CASE WHEN status IN ('failed','rejected') THEN 1 ELSE 0 END),
		SUM(CASE WHEN status='success' THEN 1 ELSE 0 END)
		FROM generation_jobs WHERE recipe_run_id=?`, runID).Scan(&active, &failed, &success)
	if active > 0 {
		return
	}
	status, projectStatus := "completed", "reviewing"
	if failed > 0 {
		status, projectStatus = "failed", "reviewing"
	} else if success == 0 {
		status, projectStatus = "cancelled", "draft"
	}
	_, _ = h.db.Exec(`UPDATE recipe_runs SET status=?, total_cost=(SELECT COALESCE(SUM(actual_cost),0) FROM generation_jobs WHERE recipe_run_id=?), finished_at=datetime('now','localtime') WHERE id=?`, status, runID, runID)
	_, _ = h.db.Exec(`UPDATE studio_projects SET status=?, updated_at=datetime('now','localtime') WHERE id=?`, projectStatus, projectID)
}

func friendlyGenerationError(err error) string {
	if err == nil {
		return "生成失败"
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "生成所需的项目资料不存在"
	}
	var apiErr *VectorEngineAPIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 429:
			return "模型请求过于频繁，请稍后重试"
		case 408, 504:
			return "模型生成超时，请重试"
		default:
			return "模型服务暂时不可用"
		}
	}
	return err.Error()
}
