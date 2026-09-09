package main

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

type GenerationProviderCapabilities struct {
	TextToImage          bool     `json:"text_to_image"`
	ImageEdit            bool     `json:"image_edit"`
	MultiReference       bool     `json:"multi_reference"`
	MaxReferenceImages   int      `json:"max_reference_images"`
	SupportedAspectRatio []string `json:"supported_aspect_ratios"`
	SupportsSeed         bool     `json:"supports_seed"`
	SupportsMask         bool     `json:"supports_mask"`
	SupportsAsync        bool     `json:"supports_async"`
}

type GenerationProviderRequest struct {
	JobID           string
	Prompt          string
	Model           string
	AspectRatio     string
	Resolution      string
	ReferenceImages []string
}

type GenerationProvider interface {
	ID() string
	Name() string
	Capabilities() GenerationProviderCapabilities
	Generate(context.Context, GenerationProviderRequest) ([]geminiInlineData, error)
	CancelJob(string) error
}

type LegacyProvider struct{ handler *Handler }

func NewLegacyProvider(handler *Handler) *LegacyProvider { return &LegacyProvider{handler: handler} }
func (p *LegacyProvider) ID() string                     { return "legacy" }
func (p *LegacyProvider) Name() string                   { return "Legacy Provider" }
func (p *LegacyProvider) Capabilities() GenerationProviderCapabilities {
	return GenerationProviderCapabilities{TextToImage: true, ImageEdit: true, MultiReference: true,
		MaxReferenceImages: maxReferenceImages, SupportedAspectRatio: []string{"1:1", "3:4", "4:3", "16:9"}}
}
func (p *LegacyProvider) Generate(ctx context.Context, request GenerationProviderRequest) ([]geminiInlineData, error) {
	return p.handler.generateLegacyRecipeImages(ctx, request)
}
func (p *LegacyProvider) CancelJob(string) error { return nil }

func (h *Handler) ListGenerationProviders(c *gin.Context) {
	providers := make([]gin.H, 0, len(h.generationProviders))
	for _, provider := range h.generationProviders {
		providers = append(providers, gin.H{"id": provider.ID(), "name": provider.Name(), "capabilities": provider.Capabilities()})
	}
	c.JSON(http.StatusOK, providers)
}
