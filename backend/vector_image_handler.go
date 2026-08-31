package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
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

	ratio := allowValue(req.AspectRatio, "1:1", "1:1", "4:3", "3:4", "3:2", "2:3", "16:9", "9:16", "5:4", "4:5", "21:9")
	imageSize := allowValue(strings.ToUpper(req.ImageSize), "1K", "1K", "2K", "4K")
	payload := geminiGenerateRequest{
		Contents: []geminiContent{{Role: "user", Parts: []geminiPart{{Text: req.Prompt}}}},
		GenerationConfig: geminiGenerationConfig{
			ResponseModalities: []string{"IMAGE"},
			ImageConfig:        geminiImageConfig{AspectRatio: ratio, ImageSize: imageSize},
		},
	}

	model := strings.TrimSpace(h.imageModelFast)
	if model == "" {
		model = "gemini-3.1-flash-image-preview"
	}
	apiPath := "/v1beta/models/" + url.PathEscape(model) + ":generateContent"
	responseBody, err := h.vectorEngine.PostGeminiJSON(c.Request.Context(), apiPath, payload)
	if err != nil {
		h.log.Error("VectorEngine image generation failed", "model_profile", "fast", "error", err)
		var apiErr *VectorEngineAPIError
		if errors.As(err, &apiErr) {
			c.JSON(http.StatusBadGateway, gin.H{"error": "VectorEngine 图片生成失败", "upstream_status": apiErr.StatusCode})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "VectorEngine 图片生成失败"})
		return
	}

	images, err := normalizeGeminiImages(responseBody)
	if err != nil {
		h.log.Error("invalid VectorEngine image response", "model_profile", "fast", "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "VectorEngine 未返回有效图片"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": images, "model_profile": "fast"})
}

func normalizeGeminiImages(body []byte) ([]gin.H, error) {
	var response geminiGenerateResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析响应: %w", err)
	}
	images := make([]gin.H, 0)
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			inline := part.InlineData
			if inline == nil {
				inline = part.InlineDataCamel
			}
			if inline == nil || strings.TrimSpace(inline.Data) == "" {
				continue
			}
			mimeType := inline.MIMEType
			if mimeType == "" {
				mimeType = inline.MIMETypeCamel
			}
			if mimeType == "" {
				mimeType = "image/png"
			}
			images = append(images, gin.H{"url": "data:" + mimeType + ";base64," + inline.Data})
		}
	}
	if len(images) == 0 {
		return nil, errors.New("响应中没有 inline image data")
	}
	return images, nil
}

func allowValue(value, fallback string, allowed ...string) string {
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return fallback
}
