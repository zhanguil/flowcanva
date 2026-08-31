package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGenerateFastImageUsesServerModelAndCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	h := &Handler{
		log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		vectorEngine:   NewVectorEngineProvider(upstream.URL, "server-secret", upstream.Client()),
		imageModelFast: "server-fast-model",
	}
	router := gin.New()
	router.POST("/api/images/generate", h.GenerateImage)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/images/generate", strings.NewReader(`{"prompt":"a chair","model":"browser-model","base_url":"https://evil.invalid","api_key":"browser-secret"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "data:image/png;base64,aW1hZ2U=") {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
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
	if got := images[0]["url"]; got != "data:image/jpeg;base64,YWJj" {
		t.Fatalf("unexpected data URL: %v", got)
	}
}
