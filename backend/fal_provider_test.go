package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFalProviderKeepsCredentialsServerSideAndUsesMultipleReferences(t *testing.T) {
	imageData, _ := base64.StdEncoding.DecodeString(mockPlaceholderImage().Data)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imageData)
			return
		}
		if r.Header.Get("Authorization") != "Key server-secret" {
			t.Errorf("missing server-side fal credentials")
		}
		var payload struct {
			Prompt    string         `json:"prompt"`
			ImageURLs []string       `json:"image_urls"`
			ImageSize map[string]int `json:"image_size"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.ImageURLs) != 2 || payload.ImageSize["width"] != 1536 || payload.ImageSize["height"] != 2048 {
			t.Errorf("unexpected fal payload: %+v", payload)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"images": []map[string]any{{"url": server.URL + "/result.png", "content_type": "image/png"}}})
	}))
	defer server.Close()

	uploadDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(uploadDir, "a.png"), imageData, 0600); err != nil {
		t.Fatal(err)
	}
	provider := NewFalProvider(server.URL, "server-secret", "fal-ai/qwen-image-edit-2511", uploadDir, server.Client())
	images, err := provider.Generate(context.Background(), GenerationProviderRequest{Prompt: "保持产品结构", AspectRatio: "3:4",
		ReferenceImages: []string{"/uploads/a.png", "/uploads/a.png"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || images[0].MIMEType != "image/png" || images[0].Data == "" {
		t.Fatalf("unexpected images: %+v", images)
	}
}
