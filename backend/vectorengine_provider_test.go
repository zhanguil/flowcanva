package main

import (
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVectorEngineProviderPostJSON(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		if got := r.Header.Get("Authorization"); got != "Bearer server-secret" {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if _, leaked := payload["api_key"]; leaked {
			t.Fatal("api key leaked into request JSON")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"/result.png"}]}`))
	}))
	defer server.Close()

	provider := NewVectorEngineProvider(server.URL+"/v1", "server-secret", server.Client())
	body, err := provider.PostJSON(context.Background(), "/v1/images/generations", map[string]any{"prompt": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if receivedPath != "/v1/images/generations" {
		t.Fatalf("unexpected path: %s", receivedPath)
	}
	if string(body) != `{"data":[{"url":"/result.png"}]}` {
		t.Fatalf("unexpected body: %s", body)
	}
}

type failingRoundTripper struct{}

func (failingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, errors.New("connect failed: " + r.URL.String())
}

func TestVectorEngineProviderRedactsCredentialFromNetworkErrors(t *testing.T) {
	client := &http.Client{Transport: failingRoundTripper{}}
	provider := NewVectorEngineProvider("https://api.example.test", "server-secret", client)
	_, err := provider.PostGeminiJSON(context.Background(), "/v1beta/models/test:generateContent", map[string]any{})
	if err == nil {
		t.Fatal("expected network error")
	}
	if strings.Contains(err.Error(), "server-secret") {
		t.Fatalf("credential leaked in error: %s", err)
	}
	if !strings.Contains(err.Error(), "key=%2A%2A%2A") && !strings.Contains(err.Error(), "key=***") {
		t.Fatalf("expected redacted URL, got: %s", err)
	}
}

func TestVectorEngineProviderRequiresServerConfig(t *testing.T) {
	provider := NewVectorEngineProvider("", "", nil)
	_, err := provider.PostJSON(context.Background(), "/v1/images/generations", map[string]any{})
	if !errors.Is(err, ErrVectorEngineNotConfigured) {
		t.Fatalf("expected configuration error, got %v", err)
	}
}

func TestVectorEngineProviderGeminiQueryAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("key"); got != "server-secret" {
			t.Fatalf("unexpected key query: %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Gemini native request should not use bearer auth: %q", got)
		}
		_, _ = w.Write([]byte(`{"candidates":[]}`))
	}))
	defer server.Close()

	provider := NewVectorEngineProvider(server.URL, "server-secret", server.Client())
	_, err := provider.PostGeminiJSON(context.Background(), "/v1beta/models/test:generateContent", map[string]any{"contents": []any{}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestVectorEngineProviderMultipartBearerAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer server-secret" {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Fatal(err)
		}
		if got := r.FormValue("model"); got != "gpt-image-2" {
			t.Fatalf("unexpected model: %q", got)
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	var body strings.Builder
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "gpt-image-2")
	_ = writer.Close()

	provider := NewVectorEngineProvider(server.URL, "server-secret", server.Client())
	_, err := provider.PostMultipart(context.Background(), "/v1/images/edits", writer.FormDataContentType(), strings.NewReader(body.String()))
	if err != nil {
		t.Fatal(err)
	}
}

func TestVectorEngineProviderPreservesUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream unavailable"}`))
	}))
	defer server.Close()

	provider := NewVectorEngineProvider(server.URL, "server-secret", server.Client())
	_, err := provider.PostJSON(context.Background(), "/v1/images/generations", map[string]any{})
	var apiErr *VectorEngineAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected VectorEngineAPIError, got %v", err)
	}
	if apiErr.StatusCode != http.StatusBadGateway || apiErr.Body != `{"error":"upstream unavailable"}` {
		t.Fatalf("unexpected upstream error: %#v", apiErr)
	}
}
