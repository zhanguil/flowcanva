package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGenerateFastImageUsesServerModelAndCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	callCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.URL.Path != "/v1beta/models/server-fast-model:generateContent" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("key"); got != "server-secret" {
			t.Fatalf("unexpected server credential: %q", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if _, leaked := payload["api_key"]; leaked {
			t.Fatal("browser credential leaked to upstream payload")
		}
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]}}]}`))
	}))
	defer upstream.Close()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	tempDir := t.TempDir()
	db, err := initDB(filepath.Join(tempDir, "test.db"), log)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	uploadDir := filepath.Join(tempDir, "uploads")
	h := &Handler{
		db:             db,
		log:            log,
		vectorEngine:   NewVectorEngineProvider(upstream.URL, "server-secret", upstream.Client()),
		imageModelFast: "server-fast-model",
		uploadDir:      uploadDir,
	}
	router := gin.New()
	router.POST("/api/images/generate", h.GenerateImage)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/images/generate", strings.NewReader(`{"prompt":"a chair","n":2,"model":"browser-model","base_url":"https://evil.invalid","api_key":"browser-secret"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data []GeneratedAsset `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data) != 2 || callCount != 2 {
		t.Fatalf("assets=%d upstream_calls=%d body=%s", len(response.Data), callCount, recorder.Body.String())
	}
	for _, asset := range response.Data {
		if !strings.HasPrefix(asset.URL, "/uploads/generated-ast_") {
			t.Fatalf("unexpected asset URL: %s", asset.URL)
		}
		if _, err := os.Stat(filepath.Join(uploadDir, asset.Filename)); err != nil {
			t.Fatalf("saved file missing: %v", err)
		}
	}
	var assetCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM assets`).Scan(&assetCount); err != nil || assetCount != 2 {
		t.Fatalf("asset rows=%d err=%v", assetCount, err)
	}
}

func TestGenerateFastImageRequiresPromptAndServerConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	router := gin.New()
	router.POST("/api/images/generate", h.GenerateImage)

	for name, testCase := range map[string]struct {
		body       string
		wantStatus int
	}{
		"empty prompt":   {body: `{"prompt":" "}`, wantStatus: http.StatusBadRequest},
		"missing config": {body: `{"prompt":"chair"}`, wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/images/generate", strings.NewReader(testCase.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestNormalizeGeminiImagesSupportsSnakeCase(t *testing.T) {
	images, err := normalizeGeminiImages([]byte(`{"candidates":[{"content":{"parts":[{"inline_data":{"mime_type":"image/jpeg","data":"YWJj"}}]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := images[0].Data; got != "YWJj" {
		t.Fatalf("unexpected image data: %v", got)
	}
}

func TestImageGenerationDefaultsToOneResult(t *testing.T) {
	for _, count := range []int{-1, 0, 1, 3, 10} {
		if got := allowImageCount(count); got != 1 {
			t.Fatalf("count=%d got=%d", count, got)
		}
	}
}
