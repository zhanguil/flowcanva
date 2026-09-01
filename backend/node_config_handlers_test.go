package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNodeConfigAPIKeepsCredentialsOutOfBrowser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := initDB(filepath.Join(t.TempDir(), "test.db"), log)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`UPDATE node_configs SET api_key=?, extra_config=? WHERE node_type='image'`,
		"server-only-secret", `{"models":[{"name":"legacy","api_key":"nested-secret","apiKey":"camel-secret"}]}`)
	if err != nil {
		t.Fatal(err)
	}

	h := &Handler{db: db, log: log}
	router := gin.New()
	router.GET("/api/admin/node-configs", h.ListNodeConfigs)
	router.GET("/api/admin/node-configs/:type", h.GetNodeConfig)
	router.PUT("/api/admin/node-configs/:type", h.UpdateNodeConfig)

	for _, target := range []string{"/api/admin/node-configs", "/api/admin/node-configs/image"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		assertNoBrowserCredential(t, recorder)
	}

	body := `{"api_key":"browser-secret","extra_config":"{\"models\":[{\"name\":\"safe\",\"VECTORENGINE_API_KEY\":\"nested-vector-secret\"}]}"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/node-configs/image", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	assertNoBrowserCredential(t, recorder)

	var storedKey, storedExtra string
	if err := db.QueryRow(`SELECT api_key, extra_config FROM node_configs WHERE node_type='image'`).Scan(&storedKey, &storedExtra); err != nil {
		t.Fatal(err)
	}
	if storedKey != "server-only-secret" {
		t.Fatalf("browser request changed backend credential: %q", storedKey)
	}
	if strings.Contains(strings.ToLower(storedExtra), "api_key") || strings.Contains(strings.ToLower(storedExtra), "apikey") || strings.Contains(storedExtra, "nested-vector-secret") {
		t.Fatalf("browser extra_config retained credential material: %s", storedExtra)
	}
}

func assertNoBrowserCredential(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(recorder.Body.String())
	for _, forbidden := range []string{"api_key", "apikey", "vectorengine_api_key", "server-only-secret", "nested-secret", "camel-secret", "browser-secret"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("browser response leaked %q: %s", forbidden, recorder.Body.String())
		}
	}
}
