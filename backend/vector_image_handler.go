package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type VectorImageRequest struct {
	Prompt      string `json:"prompt"`
	AspectRatio string `json:"aspect_ratio"`
	ImageSize   string `json:"image_size"`
	N           int    `json:"n"`
}

type geminiGenerateRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text            string            `json:"text,omitempty"`
	InlineData      *geminiInlineData `json:"inline_data,omitempty"`
	InlineDataCamel *geminiInlineData `json:"inlineData,omitempty"`
}

type geminiInlineData struct {
	MIMEType      string `json:"mime_type"`
	MIMETypeCamel string `json:"mimeType"`
	Data          string `json:"data"`
}

type geminiGenerationConfig struct {
	ResponseModalities []string          `json:"responseModalities"`
	ImageConfig        geminiImageConfig `json:"imageConfig"`
}

type geminiImageConfig struct {
	AspectRatio string `json:"aspectRatio"`
	ImageSize   string `json:"imageSize"`
}

type geminiGenerateResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
}

type GeneratedAsset struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	URL      string `json:"url"`
	Size     int64  `json:"size"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Category string `json:"category"`
	Tags     string `json:"tags"`
}

func (h *Handler) GenerateImage(c *gin.Context) {
	var req VectorImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不正确"})
		return
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "提示词不能为空"})
		return
	}
	if h.vectorEngine == nil || !h.vectorEngine.Configured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": ErrVectorEngineNotConfigured.Error()})
		return
	}

	count := allowImageCount(req.N)
	payload := buildGeminiImagePayload(req)
	model := strings.TrimSpace(h.imageModelFast)
	if model == "" {
		model = "gemini-3.1-flash-image-preview"
	}
	apiPath := "/v1beta/models/" + url.PathEscape(model) + ":generateContent"

	generated := make([]geminiInlineData, 0, count)
	for len(generated) < count {
		responseBody, err := h.vectorEngine.PostGeminiJSON(c.Request.Context(), apiPath, payload)
		if err != nil {
			h.writeImageGenerationError(c, err)
			return
		}
		batch, err := normalizeGeminiImages(responseBody)
		if err != nil {
			h.log.Error("invalid VectorEngine image response", "model_profile", "fast", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "VectorEngine 未返回有效图片"})
			return
		}
		generated = append(generated, batch...)
	}
	generated = generated[:count]

	assets, err := h.persistGeneratedImages(generated)
	if err != nil {
		h.log.Error("persist generated images failed", "model_profile", "fast", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成图片保存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": assets, "model_profile": "fast"})
}

func buildGeminiImagePayload(req VectorImageRequest) geminiGenerateRequest {
	ratio := allowValue(req.AspectRatio, "1:1", "1:1", "4:3", "3:4", "3:2", "2:3", "16:9", "9:16", "5:4", "4:5", "21:9")
	imageSize := allowValue(strings.ToUpper(req.ImageSize), "1K", "1K", "2K", "4K")
	return geminiGenerateRequest{
		Contents: []geminiContent{{Role: "user", Parts: []geminiPart{{Text: req.Prompt}}}},
		GenerationConfig: geminiGenerationConfig{
			ResponseModalities: []string{"IMAGE"},
			ImageConfig:        geminiImageConfig{AspectRatio: ratio, ImageSize: imageSize},
		},
	}
}

func (h *Handler) writeImageGenerationError(c *gin.Context, err error) {
	h.log.Error("VectorEngine image generation failed", "model_profile", "fast", "error", err)
	var apiErr *VectorEngineAPIError
	if errors.As(err, &apiErr) {
		c.JSON(http.StatusBadGateway, gin.H{"error": "VectorEngine 图片生成失败", "upstream_status": apiErr.StatusCode})
		return
	}
	c.JSON(http.StatusBadGateway, gin.H{"error": "VectorEngine 图片生成失败"})
}

func normalizeGeminiImages(body []byte) ([]geminiInlineData, error) {
	var response geminiGenerateResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析响应: %w", err)
	}
	images := make([]geminiInlineData, 0)
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			inline := part.InlineData
			if inline == nil {
				inline = part.InlineDataCamel
			}
			if inline == nil || strings.TrimSpace(inline.Data) == "" {
				continue
			}
			if inline.MIMEType == "" {
				inline.MIMEType = inline.MIMETypeCamel
			}
			if inline.MIMEType == "" {
				inline.MIMEType = "image/png"
			}
			images = append(images, *inline)
		}
	}
	if len(images) == 0 {
		return nil, errors.New("响应中没有 inline image data")
	}
	return images, nil
}

func (h *Handler) persistGeneratedImages(images []geminiInlineData) ([]GeneratedAsset, error) {
	if h.db == nil {
		return nil, errors.New("数据库未初始化")
	}
	uploadDir := h.assetUploadDir()
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, fmt.Errorf("创建上传目录: %w", err)
	}

	assets := make([]GeneratedAsset, 0, len(images))
	for _, generated := range images {
		asset, err := h.persistGeneratedImage(uploadDir, generated)
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	return assets, nil
}

func (h *Handler) persistGeneratedImage(uploadDir string, generated geminiInlineData) (GeneratedAsset, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(generated.Data))
	if err != nil {
		return GeneratedAsset{}, fmt.Errorf("解码生成图片: %w", err)
	}
	ext, err := imageExtension(generated.MIMEType)
	if err != nil {
		return GeneratedAsset{}, err
	}
	id := "ast_" + uuid.New().String()[:8]
	filename := "generated-" + id + ext
	filePath := filepath.Join(uploadDir, filename)
	if err := os.WriteFile(filePath, raw, 0644); err != nil {
		return GeneratedAsset{}, fmt.Errorf("写入生成图片: %w", err)
	}

	width, height := 0, 0
	if config, _, decodeErr := image.DecodeConfig(bytes.NewReader(raw)); decodeErr == nil {
		width, height = config.Width, config.Height
	}
	asset := GeneratedAsset{
		ID: id, Filename: filename, URL: "/uploads/" + filename, Size: int64(len(raw)),
		Width: width, Height: height, Category: "AI生成", Tags: `["ai-generated","nano-banana-2"]`,
	}
	_, err = h.db.Exec(`INSERT INTO assets (id, filename, url, size, width, height, category, tags) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		asset.ID, asset.Filename, asset.URL, asset.Size, asset.Width, asset.Height, asset.Category, asset.Tags)
	if err != nil {
		_ = os.Remove(filePath)
		return GeneratedAsset{}, fmt.Errorf("登记生成资产: %w", err)
	}
	return asset, nil
}

func imageExtension(mimeType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return ".png", nil
	case "image/jpeg", "image/jpg":
		return ".jpg", nil
	case "image/webp":
		return ".webp", nil
	case "image/gif":
		return ".gif", nil
	default:
		return "", fmt.Errorf("不支持的生成图片格式: %s", mimeType)
	}
}

func allowImageCount(count int) int {
	if count == 2 || count == 4 {
		return count
	}
	return 1
}

func allowValue(value, fallback string, allowed ...string) string {
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return fallback
}
