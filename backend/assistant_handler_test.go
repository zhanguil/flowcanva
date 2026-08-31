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

func TestAssistantUsesServerModelAndCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer server-secret" {
			t.Fatalf("unexpected authorization: %q", got)
		}
		var payload assistantChatPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Model != "server-assistant-model" {
			t.Fatalf("unexpected model: %s", payload.Model)
		}
		if len(payload.Messages) != 2 || payload.Messages[0].Role != "system" || payload.Messages[1].Content != "分析这把椅子" {
			t.Fatalf("unexpected messages: %#v", payload.Messages)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"这是产品分析结果"}}]}`))
	}))
	defer upstream.Close()

	h := &Handler{
		log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		vectorEngine:   NewVectorEngineProvider(upstream.URL, "server-secret", upstream.Client()),
		assistantModel: "server-assistant-model",
	}
	router := gin.New()
	router.POST("/api/assistant/chat", h.ChatWithAssistant)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assistant/chat", strings.NewReader(`{"model":"browser-model","api_key":"browser-secret","messages":[{"role":"user","content":"分析这把椅子"}]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "这是产品分析结果") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAssistantRequiresServerConfigurationAndUserMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	for name, testCase := range map[string]struct {
		handler *Handler
		body    string
		status  int
	}{
		"missing model": {
			handler: &Handler{log: log, vectorEngine: NewVectorEngineProvider("https://example.test", "secret", nil)},
			body:    `{"messages":[{"role":"user","content":"分析产品"}]}`,
			status:  http.StatusServiceUnavailable,
		},
		"invalid last role": {
			handler: &Handler{log: log, vectorEngine: NewVectorEngineProvider("https://example.test", "secret", nil), assistantModel: "configured"},
			body:    `{"messages":[{"role":"assistant","content":"旧回复"}]}`,
			status:  http.StatusBadRequest,
		},
	} {
		t.Run(name, func(t *testing.T) {
			router := gin.New()
			router.POST("/api/assistant/chat", testCase.handler.ChatWithAssistant)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/assistant/chat", strings.NewReader(testCase.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			if recorder.Code != testCase.status {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
