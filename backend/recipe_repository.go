package main

import "encoding/json"

func (h *Handler) recipeRunByID(id string) (RecipeRun, error) {
	var run RecipeRun
	err := h.db.QueryRow(`SELECT rr.id, rr.project_id, rr.recipe_id, rr.recipe_version, rr.request_id,
		rr.status, rr.job_count, rr.total_cost, r.currency, rr.created_by, rr.created_at
		FROM recipe_runs rr JOIN recipes r ON r.id=rr.recipe_id WHERE rr.id=?`, id).
		Scan(&run.ID, &run.ProjectID, &run.RecipeID, &run.RecipeVersion, &run.RequestID,
			&run.Status, &run.JobCount, &run.TotalCost, &run.Currency, &run.CreatedBy, &run.CreatedAt)
	if err != nil {
		return RecipeRun{}, err
	}
	run.Jobs = []GenerationJob{}
	rows, err := h.db.Query(`SELECT id, project_id, recipe_run_id, sku_id, output_type, status, provider,
		model, prompt, prompt_version, reference_pack, aspect_ratio, created_by, created_at, started_at,
		finished_at, estimated_cost, actual_cost, duration_ms, retry_count, result_asset_id, error_code,
		error_message FROM generation_jobs WHERE recipe_run_id=? ORDER BY created_at, id`, run.ID)
	if err != nil {
		return RecipeRun{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var job GenerationJob
		var referencePack string
		if err := rows.Scan(&job.ID, &job.ProjectID, &job.RecipeRunID, &job.SKUID, &job.OutputType,
			&job.Status, &job.Provider, &job.Model, &job.Prompt, &job.PromptVersion, &referencePack,
			&job.AspectRatio, &job.CreatedBy, &job.CreatedAt, &job.StartedAt, &job.FinishedAt,
			&job.EstimatedCost, &job.ActualCost, &job.DurationMS, &job.RetryCount, &job.ResultAssetID,
			&job.ErrorCode, &job.ErrorMessage); err != nil {
			return RecipeRun{}, err
		}
		job.ReferencePack.References = []ReferenceAsset{}
		_ = json.Unmarshal([]byte(referencePack), &job.ReferencePack)
		run.Jobs = append(run.Jobs, job)
	}
	return run, rows.Err()
}
