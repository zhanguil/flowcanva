package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
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

func TestGenerateProImageUsesServerProModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/server-pro-model:generateContent" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("key"); got != "server-secret" {
			t.Fatalf("unexpected server credential: %q", got)
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
	h := &Handler{
		db: db, log: log,
		vectorEngine:   NewVectorEngineProvider(upstream.URL, "server-secret", upstream.Client()),
		imageModelFast: "server-fast-model", imageModelPro: "server-pro-model",
		uploadDir: filepath.Join(tempDir, "uploads"),
	}
	router := gin.New()
	router.POST("/api/images/generate", h.GenerateImage)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/images/generate", strings.NewReader(`{"profile":"pro","prompt":"a premium chair"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"model_profile":"pro"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGenerateGPTImageUsesGenerationEndpointAndServerModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	encoded := base64.StdEncoding.EncodeToString(testPNG(t))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer server-secret" {
			t.Fatalf("unexpected authorization: %q", got)
		}
		var payload gptImageGenerationRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Model != "server-edit-model" || payload.N != 2 || payload.Size != "2048x1152" || payload.Format != "png" {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output_format": "png",
			"data":          []map[string]string{{"b64_json": encoded}, {"b64_json": encoded}},
		})
	}))
	defer upstream.Close()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	tempDir := t.TempDir()
	db, err := initDB(filepath.Join(tempDir, "test.db"), log)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{
		db: db, log: log,
		vectorEngine:   NewVectorEngineProvider(upstream.URL, "server-secret", upstream.Client()),
		imageModelEdit: "server-edit-model", uploadDir: filepath.Join(tempDir, "uploads"),
	}
	router := gin.New()
	router.POST("/api/images/generate", h.GenerateImage)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/images/generate", strings.NewReader(`{"profile":"edit","prompt":"a walnut console","n":2,"aspect_ratio":"16:9","image_size":"2K","model":"browser-model","api_key":"browser-secret"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data         []GeneratedAsset `json:"data"`
		ModelProfile string           `json:"model_profile"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data) != 2 || response.ModelProfile != "edit" || !strings.Contains(response.Data[0].Tags, "gpt-image-2") {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestGenerateGPTImageUsesMultipartEditForMultipleReferences(t *testing.T) {
	gin.SetMode(gin.TestMode)
	raw := testPNG(t)
	encoded := base64.StdEncoding.EncodeToString(raw)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer server-secret" {
			t.Fatalf("unexpected authorization: %q", got)
		}
		if err := r.ParseMultipartForm(50 << 20); err != nil {
			t.Fatal(err)
		}
		if got := len(r.MultipartForm.File["image"]); got != 2 {
			t.Fatalf("image file fields=%d", got)
		}
		if r.FormValue("model") != "server-edit-model" || r.FormValue("size") != "832x1248" || r.FormValue("prompt") != "merge both references" {
			t.Fatalf("unexpected form values: %#v", r.MultipartForm.Value)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output_format": "png", "data": map[string]string{"b64_json": encoded},
		})
	}))
	defer upstream.Close()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	tempDir := t.TempDir()
	uploadDir := filepath.Join(tempDir, "uploads")
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, "reference.png"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	db, err := initDB(filepath.Join(tempDir, "test.db"), log)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{
		db: db, log: log,
		vectorEngine:   NewVectorEngineProvider(upstream.URL, "server-secret", upstream.Client()),
		imageModelEdit: "server-edit-model", uploadDir: uploadDir,
	}
	router := gin.New()
	router.POST("/api/images/generate", h.GenerateImage)

	body, err := json.Marshal(VectorImageRequest{
		Profile: "edit", Prompt: "merge both references", AspectRatio: "2:3", ImageSize: "1K", N: 1,
		ReferenceImages: []string{"data:image/png;base64," + encoded, "/uploads/reference.png"},
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/images/generate", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
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

func TestBuildGeminiImagePayloadIncludesDataAndUploadedReferences(t *testing.T) {
	raw := testPNG(t)
	uploadDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(uploadDir, "reference.png"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	request := VectorImageRequest{
		Prompt: "keep product from image 1 and lighting from image 2",
		ReferenceImages: []string{
			"data:image/png;base64," + base64.StdEncoding.EncodeToString(raw),
			"/uploads/reference.png",
		},
	}

	payload, err := buildGeminiImagePayload(request, uploadDir)
	if err != nil {
		t.Fatal(err)
	}
	parts := payload.Contents[0].Parts
	if len(parts) != 5 || parts[0].Text != "参考图1：" || parts[2].Text != "参考图2：" || parts[4].Text != request.Prompt {
		t.Fatalf("unexpected reference part order: %#v", parts)
	}
	for _, index := range []int{1, 3} {
		if parts[index].InlineData == nil || parts[index].InlineData.MIMEType != "image/png" {
			t.Fatalf("part %d is not an inline PNG: %#v", index, parts[index])
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("/uploads/reference.png")) || !bytes.Contains(encoded, []byte(`"inline_data"`)) {
		t.Fatalf("payload should contain inline bytes instead of a local URL: %s", encoded)
	}
}

func TestBuildGeminiImagePayloadRejectsUnsafeReferences(t *testing.T) {
	for _, reference := range []string{
		"https://example.com/reference.png",
		"/uploads/../secret.png",
		"data:image/png,not-base64",
	} {
		_, err := buildGeminiImagePayload(VectorImageRequest{Prompt: "chair", ReferenceImages: []string{reference}}, t.TempDir())
		if err == nil {
			t.Fatalf("reference %q should be rejected", reference)
		}
	}
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 180, G: 120, B: 60, A: 255})
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
