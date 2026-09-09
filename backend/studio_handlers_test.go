package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func studioTestHandler(t *testing.T) (*Handler, http.Handler) {
	t.Helper()
	db, err := initDB(filepath.Join(t.TempDir(), "studio.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("initDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	h := &Handler{db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/projects", h.ListStudioProjects)
	router.POST("/api/projects", h.CreateStudioProject)
	router.GET("/api/projects/:id", h.GetStudioProject)
	router.PUT("/api/projects/:id", h.UpdateStudioProject)
	router.DELETE("/api/projects/:id", h.ArchiveStudioProject)
	router.PUT("/api/projects/:id/product-pack", h.PutProductPack)
	router.GET("/api/recipes", h.ListRecipes)
	router.POST("/api/projects/:id/recipe-runs", h.CreateRecipeRun)
	router.GET("/api/recipe-runs/:id", h.GetRecipeRun)
	router.POST("/api/recipe-runs/:id/retry-failed", h.RetryFailedJobs)
	return h, router
}

func studioRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func TestStudioProjectCreatesSharedProductPackAndSKUs(t *testing.T) {
	h, router := studioTestHandler(t)
	created := studioRequest(t, router, http.MethodPost, "/api/projects", map[string]any{
		"name": "中古晚渡电视柜", "product_name": "移动电视柜", "created_by": "designer-a",
		"tags": []string{"电视柜", "V0.2"}, "skus": []string{"1300", "1600", "1800"},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var detail StudioProjectDetail
	if err := json.Unmarshal(created.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if detail.CanvasID == "" || detail.ProductPack == nil || len(detail.ProductPack.SKUs) != 3 {
		t.Fatalf("incomplete project: %+v", detail)
	}
	if detail.Status != "draft" || detail.CreatedBy != "designer-a" {
		t.Fatalf("unexpected ownership/status: %+v", detail.StudioProject)
	}
	var canvasName string
	if err := h.db.QueryRow(`SELECT name FROM canvases WHERE id = ?`, detail.CanvasID).Scan(&canvasName); err != nil || canvasName != detail.Name {
		t.Fatalf("project canvas missing: name=%q err=%v", canvasName, err)
	}

	got := studioRequest(t, router, http.MethodGet, "/api/projects/"+detail.ID, nil)
	if got.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", got.Code, got.Body.String())
	}
	var loaded StudioProjectDetail
	_ = json.Unmarshal(got.Body.Bytes(), &loaded)
	if loaded.ProductPack == nil || loaded.ProductPack.SKUs[1].Name != "1600" {
		t.Fatalf("SKUs not persisted: %+v", loaded.ProductPack)
	}
}

func TestProductPackPersistsDNAAndReferencePack(t *testing.T) {
	h, router := studioTestHandler(t)
	created := studioRequest(t, router, http.MethodPost, "/api/projects", map[string]any{
		"name": "电视柜项目", "product_name": "电视柜", "skus": []string{"1300"},
	})
	var detail StudioProjectDetail
	_ = json.Unmarshal(created.Body.Bytes(), &detail)
	_, err := h.db.Exec(`INSERT INTO assets (id, filename, url, mime_type, width, height) VALUES ('ast_product', 'product.png', '/uploads/product.png', 'image/png', 1200, 900)`)
	if err != nil {
		t.Fatalf("insert asset: %v", err)
	}

	updated := studioRequest(t, router, http.MethodPut, "/api/projects/"+detail.ID+"/product-pack", map[string]any{
		"product_name": "移动电视柜",
		"product_dna": map[string]any{
			"productType":        "移动电视柜",
			"structuralFeatures": map[string]any{"casterCount": 4, "glassDoorCount": 2},
			"materials":          map[string]string{"main": "深胡桃木"},
			"forbiddenChanges":   []string{"禁止修改脚轮数量"},
			"allowedChanges":     []string{"整体透视匹配"},
		},
		"skus": []map[string]any{{"name": "1300", "label": "1300mm", "width": 1300}},
		"reference_pack": map[string]any{"references": []map[string]any{{
			"asset_id": "ast_product", "role": "product_main", "weight": 1.5, "locked": true,
		}}},
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	var pack ProductPack
	_ = json.Unmarshal(updated.Body.Bytes(), &pack)
	if pack.ProductDNA.ProductType != "移动电视柜" || pack.ProductDNA.StructuralFeatures["casterCount"] != float64(4) {
		t.Fatalf("DNA not persisted: %+v", pack.ProductDNA)
	}
	if len(pack.References.References) != 1 || !pack.References.References[0].Locked || pack.References.References[0].Role != "product_main" {
		t.Fatalf("reference pack not persisted: %+v", pack.References)
	}
	if len(pack.SKUs) != 1 || pack.SKUs[0].Width == nil || *pack.SKUs[0].Width != 1300 {
		t.Fatalf("SKU dimensions not persisted: %+v", pack.SKUs)
	}
}

func TestStudioProjectArchiveIsSoftDelete(t *testing.T) {
	h, router := studioTestHandler(t)
	created := studioRequest(t, router, http.MethodPost, "/api/projects", map[string]any{"name": "归档项目", "product_name": "柜子"})
	var detail StudioProjectDetail
	_ = json.Unmarshal(created.Body.Bytes(), &detail)
	archived := studioRequest(t, router, http.MethodDelete, "/api/projects/"+detail.ID, nil)
	if archived.Code != http.StatusNoContent {
		t.Fatalf("archive status=%d", archived.Code)
	}
	if got := studioRequest(t, router, http.MethodGet, "/api/projects/"+detail.ID, nil); got.Code != http.StatusNotFound {
		t.Fatalf("archived project remained visible: %d", got.Code)
	}
	var count int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM canvases WHERE id = ?`, detail.CanvasID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("soft delete removed canvas: count=%d err=%v", count, err)
	}
}
