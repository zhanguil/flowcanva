package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMissingEmbeddedAssetReturnsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := setupRouter(&Handler{log: logger}, Config{Embedded: true})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/assets/does-not-exist.js", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected missing embedded asset to return 404, got %d", response.Code)
	}
}
