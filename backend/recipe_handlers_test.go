package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestRecipeRunCreatesAllJobsOnceAndRetriesOnlyFailures(t *testing.T) {
	h, router := studioTestHandler(t)
	created := studioRequest(t, router, http.MethodPost, "/api/projects", map[string]any{
		"name": "电视柜批量主图", "product_name": "电视柜", "created_by": "designer-a",
		"skus": []string{"1300", "1600", "1800"},
	})
	var project StudioProjectDetail
	_ = json.Unmarshal(created.Body.Bytes(), &project)
	if _, err := h.db.Exec(`UPDATE recipes SET estimated_cost_per_job=0.05 WHERE id='furniture_sku_main_images'`); err != nil {
		t.Fatalf("set recipe cost: %v", err)
	}

	request := map[string]any{
		"recipe_id": "furniture_sku_main_images", "request_id": "request-six-jobs",
		"created_by": "designer-a", "provider": "legacy", "model": "fast",
	}
	first := studioRequest(t, router, http.MethodPost, "/api/projects/"+project.ID+"/recipe-runs", request)
	if first.Code != http.StatusCreated {
		t.Fatalf("create run status=%d body=%s", first.Code, first.Body.String())
	}
	var run RecipeRun
	_ = json.Unmarshal(first.Body.Bytes(), &run)
	if run.JobCount != 6 || len(run.Jobs) != 6 {
		t.Fatalf("jobs=%d/%d want 6", run.JobCount, len(run.Jobs))
	}
	if run.TotalCost < 0.299 || run.TotalCost > 0.301 || run.Status != "queued" {
		t.Fatalf("run cost/status = %.2f/%s", run.TotalCost, run.Status)
	}
	ratioCounts := map[string]int{}
	for _, job := range run.Jobs {
		ratioCounts[job.AspectRatio]++
		if job.Status != "queued" || job.RecipeRunID != run.ID {
			t.Fatalf("invalid planned job: %+v", job)
		}
	}
	if ratioCounts["1:1"] != 3 || ratioCounts["3:4"] != 3 {
		t.Fatalf("unexpected ratios: %+v", ratioCounts)
	}

	replayed := studioRequest(t, router, http.MethodPost, "/api/projects/"+project.ID+"/recipe-runs", request)
	if replayed.Code != http.StatusOK || replayed.Header().Get("X-Idempotent-Replay") != "true" {
		t.Fatalf("idempotent replay status=%d headers=%v", replayed.Code, replayed.Header())
	}
	var runCount int
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM recipe_runs WHERE request_id='request-six-jobs'`).Scan(&runCount)
	if runCount != 1 {
		t.Fatalf("duplicate request created %d runs", runCount)
	}

	failedID, successID := run.Jobs[0].ID, run.Jobs[1].ID
	_, _ = h.db.Exec(`UPDATE generation_jobs SET status='failed', error_code='provider_timeout', error_message='模型超时' WHERE id=?`, failedID)
	_, _ = h.db.Exec(`UPDATE generation_jobs SET status='success', result_asset_id='ast_ok' WHERE id=?`, successID)
	retry := studioRequest(t, router, http.MethodPost, "/api/recipe-runs/"+run.ID+"/retry-failed", nil)
	if retry.Code != http.StatusOK {
		t.Fatalf("retry status=%d body=%s", retry.Code, retry.Body.String())
	}
	var retried struct {
		Retried int       `json:"retried"`
		Run     RecipeRun `json:"run"`
	}
	_ = json.Unmarshal(retry.Body.Bytes(), &retried)
	if retried.Retried != 1 {
		t.Fatalf("retried=%d want 1", retried.Retried)
	}
	states := map[string]GenerationJob{}
	for _, job := range retried.Run.Jobs {
		states[job.ID] = job
	}
	if states[failedID].Status != "queued" || states[failedID].RetryCount != 1 || states[failedID].ErrorMessage != "" {
		t.Fatalf("failed job was not reset: %+v", states[failedID])
	}
	if states[successID].Status != "success" || states[successID].RetryCount != 0 || states[successID].ResultAssetID != "ast_ok" {
		t.Fatalf("successful job was changed: %+v", states[successID])
	}

	single := studioRequest(t, router, http.MethodPost, "/api/generation-jobs/"+successID+"/retry", nil)
	if single.Code != http.StatusOK {
		t.Fatalf("single retry status=%d body=%s", single.Code, single.Body.String())
	}
	var singleResult struct {
		JobID string    `json:"job_id"`
		Run   RecipeRun `json:"run"`
	}
	_ = json.Unmarshal(single.Body.Bytes(), &singleResult)
	if singleResult.JobID == successID || singleResult.Run.JobCount != 7 || len(singleResult.Run.Jobs) != 7 {
		t.Fatalf("single regenerate did not preserve the original result: %+v", singleResult)
	}
}

func TestRecipeRunUsesExactOutputSelectionAndCopies(t *testing.T) {
	_, router := studioTestHandler(t)
	created := studioRequest(t, router, http.MethodPost, "/api/projects", map[string]any{
		"name": "电视柜批量主图", "product_name": "电视柜", "created_by": "designer-a",
		"skus": []string{"1300", "1600", "1800"},
	})
	var project StudioProjectDetail
	_ = json.Unmarshal(created.Body.Bytes(), &project)

	response := studioRequest(t, router, http.MethodPost, "/api/projects/"+project.ID+"/recipe-runs", map[string]any{
		"recipe_id": "furniture_sku_main_images", "request_id": "exact-output-copies", "created_by": "designer-a",
		"sku_ids":          []string{project.ProductPack.SKUs[0].ID, project.ProductPack.SKUs[1].ID},
		"selected_outputs": []map[string]any{{"outputType": "scene_front", "aspectRatio": "3:4"}},
		"copies_per_item":  2,
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("create run status=%d body=%s", response.Code, response.Body.String())
	}
	var run RecipeRun
	_ = json.Unmarshal(response.Body.Bytes(), &run)
	if run.JobCount != 4 || len(run.Jobs) != 4 {
		t.Fatalf("jobs=%d/%d want 4", run.JobCount, len(run.Jobs))
	}
	for _, job := range run.Jobs {
		if job.AspectRatio != "3:4" || job.OutputType != "scene_front" {
			t.Fatalf("unexpected output: %+v", job)
		}
	}
}
