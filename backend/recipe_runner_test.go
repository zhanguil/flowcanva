package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestRecipeRunnerExecutesSixIndependentJobsAndPersistsLineage(t *testing.T) {
	h, router := studioTestHandler(t)
	h.uploadDir = t.TempDir()
	h.imageProvider = NewMockImageProvider()
	h.imageModelFast = "mock-furniture-model"
	h.jobQueue = NewJobQueue(2)

	created := studioRequest(t, router, http.MethodPost, "/api/projects", map[string]any{
		"name": "电视柜六图验收", "product_name": "移动电视柜", "created_by": "designer-a",
		"skus": []string{"1300", "1600", "1800"},
	})
	var project StudioProjectDetail
	_ = json.Unmarshal(created.Body.Bytes(), &project)
	runResponse := studioRequest(t, router, http.MethodPost, "/api/projects/"+project.ID+"/recipe-runs", map[string]any{
		"recipe_id": "furniture_sku_main_images", "request_id": "runner-six-jobs", "created_by": "designer-a",
	})
	if runResponse.Code != http.StatusCreated {
		t.Fatalf("create run status=%d body=%s", runResponse.Code, runResponse.Body.String())
	}
	var createdRun RecipeRun
	_ = json.Unmarshal(runResponse.Body.Bytes(), &createdRun)

	deadline := time.Now().Add(5 * time.Second)
	var run RecipeRun
	for time.Now().Before(deadline) {
		run, _ = h.recipeRunByID(createdRun.ID)
		if run.Status == "completed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if run.Status != "completed" || len(run.Jobs) != 6 {
		t.Fatalf("run did not complete: status=%s jobs=%d", run.Status, len(run.Jobs))
	}
	for _, job := range run.Jobs {
		if job.Status != "success" || job.ResultAssetID == "" {
			t.Fatalf("job did not succeed independently: %+v", job)
		}
		var projectID, skuID, metadata string
		if err := h.db.QueryRow(`SELECT project_id, sku_id, generation_metadata FROM assets WHERE id=?`, job.ResultAssetID).
			Scan(&projectID, &skuID, &metadata); err != nil {
			t.Fatalf("load result asset: %v", err)
		}
		if projectID != project.ID || skuID != job.SKUID || metadata == "{}" {
			t.Fatalf("asset lineage missing: project=%s sku=%s metadata=%s", projectID, skuID, metadata)
		}
	}
	requests := h.imageProvider.(*MockImageProvider).Requests()
	if len(requests) != 6 {
		t.Fatalf("provider requests=%d want 6", len(requests))
	}
	var nodeCount, portraitCount, squareCount, uniquePositionCount int
	if err := h.db.QueryRow(`SELECT COUNT(*),
		SUM(CASE WHEN height > width THEN 1 ELSE 0 END),
		SUM(CASE WHEN height = width THEN 1 ELSE 0 END),
		COUNT(DISTINCT printf('%.2f,%.2f', x, y))
		FROM nodes WHERE canvas_id=? AND node_type='asset'`, project.CanvasID).Scan(&nodeCount, &portraitCount, &squareCount, &uniquePositionCount); err != nil {
		t.Fatal(err)
	}
	if nodeCount != 6 || portraitCount != 3 || squareCount != 3 || uniquePositionCount != 6 {
		t.Fatalf("unexpected canvas grid nodes: total=%d portrait=%d square=%d positions=%d", nodeCount, portraitCount, squareCount, uniquePositionCount)
	}
}
