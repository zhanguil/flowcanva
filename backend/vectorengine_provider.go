package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrVectorEngineNotConfigured = errors.New("VectorEngine 未配置，请设置 VECTORENGINE_BASE_URL 和 VECTORENGINE_API_KEY")

type VectorEngineProvider struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

type VectorEngineAPIError struct {
	StatusCode int
	Body       string
}

func (e *VectorEngineAPIError) Error() string {
	return fmt.Sprintf("VectorEngine 请求失败（HTTP %d）: %s", e.StatusCode, e.Body)
}

func NewVectorEngineProvider(baseURL, apiKey string, client *http.Client) *VectorEngineProvider {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	return &VectorEngineProvider{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  strings.TrimSpace(apiKey),
		client:  client,
	}
}

func (p *VectorEngineProvider) Configured() bool {
	return p != nil && p.baseURL != "" && p.apiKey != ""
}

// PostJSON is the single server-side transport for VectorEngine APIs.
// Callers provide only the API path and payload; credentials never come from the browser.
func (p *VectorEngineProvider) PostJSON(ctx context.Context, apiPath string, payload any) ([]byte, error) {
	if !p.Configured() {
		return nil, ErrVectorEngineNotConfigured
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("编码 VectorEngine 请求: %w", err)
	}

	return p.do(ctx, p.endpoint(apiPath), "application/json", bytes.NewReader(body), true)
}

// PostGeminiJSON follows VectorEngine's Gemini-native contract, which uses
// the server-side API key as the required `key` query parameter.
func (p *VectorEngineProvider) PostGeminiJSON(ctx context.Context, apiPath string, payload any) ([]byte, error) {
	if !p.Configured() {
		return nil, ErrVectorEngineNotConfigured
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("编码 VectorEngine 请求: %w", err)
	}
	endpoint, err := url.Parse(p.endpoint(apiPath))
	if err != nil {
		return nil, fmt.Errorf("创建 VectorEngine 请求地址: %w", err)
	}
	query := endpoint.Query()
	query.Set("key", p.apiKey)
	endpoint.RawQuery = query.Encode()
	return p.do(ctx, endpoint.String(), "application/json", bytes.NewReader(body), false)
}

func (p *VectorEngineProvider) PostMultipart(ctx context.Context, apiPath, contentType string, body io.Reader) ([]byte, error) {
	if !p.Configured() {
		return nil, ErrVectorEngineNotConfigured
	}
	if !strings.HasPrefix(contentType, "multipart/form-data;") {
		return nil, errors.New("VectorEngine multipart 请求缺少 boundary")
	}
	return p.do(ctx, p.endpoint(apiPath), contentType, body, true)
}

func (p *VectorEngineProvider) do(ctx context.Context, endpoint, contentType string, body io.Reader, bearerAuth bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("创建 VectorEngine 请求: %w", err)
	}
	if bearerAuth {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 VectorEngine: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 VectorEngine 响应: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		safeBody := strings.ReplaceAll(strings.TrimSpace(string(responseBody)), p.apiKey, "***")
		return nil, &VectorEngineAPIError{StatusCode: resp.StatusCode, Body: safeBody}
	}
	return responseBody, nil
}

func (p *VectorEngineProvider) endpoint(apiPath string) string {
	cleanPath := "/" + strings.TrimLeft(strings.TrimSpace(apiPath), "/")
	if strings.HasSuffix(p.baseURL, "/v1") && strings.HasPrefix(cleanPath, "/v1/") {
		cleanPath = strings.TrimPrefix(cleanPath, "/v1")
	}
	return p.baseURL + cleanPath
}
