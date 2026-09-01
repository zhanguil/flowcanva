package main

import (
	"net/http"
	"path"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type GenerationDebugRecord struct {
	TaskID               string   `json:"task_id"`
	NodeID               string   `json:"node_id"`
	IncomingEdges        []string `json:"incoming_edges"`
	ResolvedInputNodes   []string `json:"resolved_input_nodes"`
	ResolvedImageAssets  []string `json:"resolved_image_assets"`
	ReferenceImageCount  int      `json:"reference_image_count"`
	Model                string   `json:"model"`
	AspectRatio          string   `json:"aspect_ratio"`
	Resolution           string   `json:"resolution"`
	ProviderRequestSent  bool     `json:"provider_request_sent"`
	ProviderImageCount   int      `json:"provider_image_count"`
	ProviderImageSources []string `json:"provider_image_sources"`
	ProviderImageBytes   []int    `json:"provider_image_bytes"`
}

func (s *GenerationDebugStore) Update(taskID string, update func(*GenerationDebugRecord)) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.has || s.latest.TaskID != taskID {
		return
	}
	update(&s.latest)
}

type GenerationDebugStore struct {
	mu     sync.RWMutex
	latest GenerationDebugRecord
	has    bool
}

func (s *GenerationDebugStore) Set(record GenerationDebugRecord) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.latest = record
	s.has = true
	s.mu.Unlock()
}

func (s *GenerationDebugStore) Latest() (GenerationDebugRecord, bool) {
	if s == nil {
		return GenerationDebugRecord{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latest, s.has
}

func ensureGenerationTaskID(value string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		if len(value) > 128 {
			return value[:128]
		}
		return value
	}
	return "task_" + uuid.New().String()[:8]
}

func (h *Handler) recordGenerationDebug(req VectorImageRequest, resolved ResolvedNodeInputs, model string) {
	if !h.devMode {
		return
	}
	record := GenerationDebugRecord{
		TaskID: req.TaskID, NodeID: req.NodeID, ReferenceImageCount: len(req.ReferenceImages),
		Model: model, AspectRatio: req.AspectRatio, Resolution: req.ImageSize,
		IncomingEdges: []string{}, ResolvedInputNodes: []string{}, ResolvedImageAssets: []string{},
		ProviderImageSources: []string{}, ProviderImageBytes: []int{},
	}
	for _, edge := range resolved.IncomingEdges {
		record.IncomingEdges = append(record.IncomingEdges, edge.ID)
	}
	for _, node := range resolved.UpstreamNodes {
		record.ResolvedInputNodes = append(record.ResolvedInputNodes, node.ID)
	}
	seenLabels := map[string]bool{}
	for _, image := range resolved.Images {
		label := safeGenerationImageLabel(image.URL, image.AssetID)
		if !seenLabels[label] {
			seenLabels[label] = true
			record.ResolvedImageAssets = append(record.ResolvedImageAssets, label)
		}
	}
	for _, reference := range req.ReferenceImages {
		label := safeGenerationImageLabel(reference, "")
		if !seenLabels[label] {
			seenLabels[label] = true
			record.ResolvedImageAssets = append(record.ResolvedImageAssets, label)
		}
	}
	h.generationDebug.Set(record)
	h.log.Info("generation debug",
		"task_id", record.TaskID,
		"node_id", record.NodeID,
		"incoming_edges", record.IncomingEdges,
		"resolved_input_nodes", record.ResolvedInputNodes,
		"resolved_image_assets", record.ResolvedImageAssets,
		"reference_image_count", record.ReferenceImageCount,
		"model", record.Model,
		"aspect_ratio", record.AspectRatio,
		"resolution", record.Resolution,
	)
}

func (h *Handler) recordProviderPayload(req VectorImageRequest, payload geminiGenerateRequest) {
	if !h.devMode {
		return
	}
	sources := make([]string, 0, len(req.ReferenceImages))
	for _, reference := range req.ReferenceImages {
		sources = append(sources, safeGenerationImageLabel(reference, ""))
	}
	imageBytes := []int{}
	for _, content := range payload.Contents {
		for _, part := range content.Parts {
			inline := part.InlineData
			if inline == nil {
				inline = part.InlineDataCamel
			}
			if inline != nil {
				imageBytes = append(imageBytes, len(inline.Data)*3/4)
			}
		}
	}
	h.generationDebug.Update(req.TaskID, func(record *GenerationDebugRecord) {
		record.ProviderImageCount = len(imageBytes)
		record.ProviderImageSources = sources
		record.ProviderImageBytes = imageBytes
	})
}

func (h *Handler) recordProviderRequestSent(taskID string) {
	if !h.devMode {
		return
	}
	h.generationDebug.Update(taskID, func(record *GenerationDebugRecord) {
		record.ProviderRequestSent = true
	})
}

func safeGenerationImageLabel(urlValue, assetID string) string {
	if assetID != "" {
		return assetID
	}
	if strings.HasPrefix(urlValue, "data:") {
		return "[inline-image]"
	}
	label := path.Base(urlValue)
	if label == "." || label == "/" || label == "" {
		return "[image]"
	}
	return label
}

func (h *Handler) GetGenerationDebug(c *gin.Context) {
	record, ok := h.generationDebug.Latest()
	if !ok {
		c.JSON(http.StatusOK, gin.H{"data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": record})
}
