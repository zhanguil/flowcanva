package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"strings"
	"sync"
)

type ImageProviderRequest struct {
	TaskID            string             `json:"task_id"`
	CanvasID          string             `json:"canvas_id"`
	NodeID            string             `json:"node_id"`
	Prompt            string             `json:"prompt"`
	Model             string             `json:"model"`
	Profile           string             `json:"profile"`
	AspectRatio       string             `json:"aspect_ratio"`
	Resolution        string             `json:"resolution"`
	N                 int                `json:"n"`
	ReferenceImages   []string           `json:"reference_images"`
	GenerationContext *GenerationContext `json:"generation_context,omitempty"`
}

type ImageProvider interface {
	Generate(context.Context, ImageProviderRequest) ([]geminiInlineData, error)
}

type MockImageProvider struct {
	mu       sync.Mutex
	requests []ImageProviderRequest
	image    geminiInlineData
}

func NewMockImageProvider() *MockImageProvider {
	return &MockImageProvider{image: mockPlaceholderImage()}
}

func (p *MockImageProvider) Generate(_ context.Context, request ImageProviderRequest) ([]geminiInlineData, error) {
	p.mu.Lock()
	p.requests = append(p.requests, request)
	p.mu.Unlock()

	count := allowImageCount(request.N)
	images := make([]geminiInlineData, count)
	output := p.image
	if parts := strings.Split(request.AspectRatio, ":"); len(parts) == 2 {
		w, _ := strconv.Atoi(parts[0])
		h, _ := strconv.Atoi(parts[1])
		if w > 0 && h > 0 && w <= 21 && h <= 21 {
			output = mockImageDimensions(w*32, h*32)
		}
	}
	for i := range images {
		images[i] = output
	}
	return images, nil
}

func (p *MockImageProvider) Requests() []ImageProviderRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make([]ImageProviderRequest, len(p.requests))
	copy(result, p.requests)
	return result
}

func mockPlaceholderImage() geminiInlineData {
	return mockImageDimensions(96, 96)
}

func mockImageDimensions(width, height int) geminiInlineData {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			shade := uint8(45 + (x+y)%40)
			img.SetRGBA(x, y, color.RGBA{R: shade, G: 105, B: 180, A: 255})
		}
	}
	var buffer bytes.Buffer
	_ = png.Encode(&buffer, img)
	return geminiInlineData{MIMEType: "image/png", Data: base64.StdEncoding.EncodeToString(buffer.Bytes())}
}
