package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type FalProvider struct {
	baseURL, apiKey, model, uploadDir string
	client                            *http.Client
}

func NewFalProvider(baseURL, apiKey, model, uploadDir string, client *http.Client) *FalProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	return &FalProvider{baseURL: strings.TrimRight(baseURL, "/"), apiKey: strings.TrimSpace(apiKey), model: strings.Trim(model, "/"), uploadDir: uploadDir, client: client}
}

func (p *FalProvider) ID() string   { return "fal" }
func (p *FalProvider) Name() string { return "fal · Qwen Image Edit 2511" }
func (p *FalProvider) Capabilities() GenerationProviderCapabilities {
	return GenerationProviderCapabilities{ImageEdit: true, MultiReference: true, MaxReferenceImages: 8,
		SupportedAspectRatio: []string{"1:1", "3:4", "4:3", "16:9"}, SupportsSeed: true, SupportsAsync: true}
}
func (p *FalProvider) CancelJob(string) error {
	return errors.New("fal 同步请求通过上下文取消")
}

func (p *FalProvider) Generate(ctx context.Context, request GenerationProviderRequest) ([]geminiInlineData, error) {
	if p.apiKey == "" {
		return nil, errors.New("Fal Provider 未配置 FAL_KEY")
	}
	if len(request.ReferenceImages) == 0 {
		return nil, errors.New("Qwen Image Edit 至少需要一张参考图")
	}
	imageURLs := make([]string, 0, len(request.ReferenceImages))
	for _, reference := range request.ReferenceImages {
		inline, err := loadReferenceImage(reference, p.uploadDir)
		if err != nil {
			return nil, err
		}
		imageURLs = append(imageURLs, "data:"+inline.MIMEType+";base64,"+inline.Data)
	}
	payload := map[string]any{"prompt": request.Prompt, "image_urls": imageURLs, "image_size": falImageSize(request.AspectRatio),
		"num_images": 1, "output_format": "png", "enable_safety_checker": true, "acceleration": "regular"}
	body, _ := json.Marshal(payload)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/"+p.model, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Authorization", "Key "+p.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("调用 Fal: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Fal 请求失败（HTTP %d）", response.StatusCode)
	}
	var output struct {
		Images []struct {
			URL         string `json:"url"`
			ContentType string `json:"content_type"`
		} `json:"images"`
	}
	if err := json.Unmarshal(responseBody, &output); err != nil || len(output.Images) == 0 {
		return nil, errors.New("Fal 未返回有效图片")
	}
	images := make([]geminiInlineData, 0, len(output.Images))
	for _, remote := range output.Images {
		download, err := http.NewRequestWithContext(ctx, http.MethodGet, remote.URL, nil)
		if err != nil {
			return nil, err
		}
		fileResponse, err := p.client.Do(download)
		if err != nil {
			return nil, err
		}
		raw, readErr := io.ReadAll(io.LimitReader(fileResponse.Body, maxReferenceImageSize+1))
		fileResponse.Body.Close()
		if readErr != nil || fileResponse.StatusCode < 200 || fileResponse.StatusCode >= 300 || len(raw) > maxReferenceImageSize {
			return nil, errors.New("Fal 结果图片下载失败")
		}
		mimeType := http.DetectContentType(raw)
		images = append(images, geminiInlineData{MIMEType: mimeType, Data: base64.StdEncoding.EncodeToString(raw)})
	}
	return images, nil
}

func falImageSize(aspectRatio string) map[string]int {
	switch aspectRatio {
	case "3:4":
		return map[string]int{"width": 1536, "height": 2048}
	case "4:3":
		return map[string]int{"width": 2048, "height": 1536}
	case "16:9":
		return map[string]int{"width": 2048, "height": 1152}
	default:
		return map[string]int{"width": 2048, "height": 2048}
	}
}
