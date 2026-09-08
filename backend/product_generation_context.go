package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

type ProductConstraint struct {
	StructureLock  bool           `json:"structureLock"`
	MaterialLock   bool           `json:"materialLock"`
	TextureLock    bool           `json:"textureLock"`
	ProportionLock bool           `json:"proportionLock"`
	HardwareLock   bool           `json:"hardwareLock"`
	LockedFields   map[string]any `json:"lockedFields"`
	CustomRules    []string       `json:"customRules"`
}
type ProductAssetContext struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	CoverImageID string            `json:"coverImageId"`
	ReferenceIDs []string          `json:"referenceIds"`
	Constraints  ProductConstraint `json:"constraints"`
	Metadata     map[string]any    `json:"metadata"`
	CreatedAt    string            `json:"createdAt"`
	UpdatedAt    string            `json:"updatedAt"`
}
type ProductReference struct {
	ID           string `json:"id"`
	AssetID      string `json:"assetId,omitempty"`
	URL          string `json:"url"`
	Name         string `json:"name,omitempty"`
	Role         string `json:"role"`
	Origin       string `json:"origin,omitempty"`
	GenerationID string `json:"generationId,omitempty"`
}
type ProductOutputOptions struct {
	Model          string `json:"model"`
	AspectRatio    string `json:"aspectRatio"`
	ImageSize      string `json:"imageSize"`
	Count          int    `json:"count"`
	GenerationType string `json:"generationType"`
}
type GenerationContext struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Product       *ProductAssetContext `json:"product"`
	References    []ProductReference   `json:"references"`
	Constraints   ProductConstraint    `json:"constraints"`
	Prompt        string               `json:"prompt"`
	OutputOptions ProductOutputOptions `json:"outputOptions"`
}
type GenerationLineage struct {
	ID                  string   `json:"id"`
	ParentGenerationID  *string  `json:"parentGenerationId"`
	ParentGenerationIDs []string `json:"parentGenerationIds"`
	CreatedAt           string   `json:"createdAt"`
}
type ProductGenerationRecord struct {
	GenerationLineage
	RootProductAssetID *string            `json:"rootProductAssetId"`
	ReferenceIDs       []string           `json:"referenceIds"`
	Prompt             string             `json:"prompt"`
	Model              string             `json:"model"`
	AspectRatio        string             `json:"aspectRatio"`
	GenerationType     string             `json:"generationType"`
	Context            *GenerationContext `json:"context"`
	Status             string             `json:"status"`
	OutputAssetIDs     []string           `json:"outputAssetIds"`
}

func applyGenerationContext(req *VectorImageRequest) error {
	ctx := req.GenerationContext
	if ctx == nil {
		return nil
	}
	if ctx.SchemaVersion != 1 {
		return fmt.Errorf("不支持的 GenerationContext 版本")
	}
	if req.Lineage == nil || req.Lineage.ID != req.TaskID {
		return fmt.Errorf("生成血缘 ID 与任务不一致")
	}
	parents := map[string]bool{}
	for _, id := range req.Lineage.ParentGenerationIDs {
		if id == "" || id == req.TaskID || parents[id] {
			return fmt.Errorf("父代生成 ID 无效或重复")
		}
		parents[id] = true
	}
	if (req.Lineage.ParentGenerationID == nil && len(parents) > 0) || (req.Lineage.ParentGenerationID != nil && !parents[*req.Lineage.ParentGenerationID]) {
		return fmt.Errorf("主父代与父代列表不一致")
	}
	if ctx.Product != nil {
		if strings.TrimSpace(ctx.Product.ID) == "" || strings.TrimSpace(ctx.Product.Name) == "" {
			return fmt.Errorf("产品 ID 和名称不能为空")
		}
		if !reflect.DeepEqual(ctx.Product.Constraints, ctx.Constraints) {
			return fmt.Errorf("产品锁与本次生成约束不一致")
		}
	}
	if ctx.OutputOptions.Count != 1 && ctx.OutputOptions.Count != 2 && ctx.OutputOptions.Count != 4 {
		return fmt.Errorf("生成数量无效")
	}
	if !containsContextValue(ctx.OutputOptions.GenerationType, "hero", "scene", "angle", "detail", "material", "structure", "sellingPoint", "custom") {
		return fmt.Errorf("生成类型无效")
	}
	if !containsContextValue(ctx.OutputOptions.AspectRatio, "自适应", "1:1", "3:4", "4:3", "3:2", "2:3", "16:9", "9:16", "5:4", "4:5", "21:9") {
		return fmt.Errorf("图片比例无效")
	}
	if len(ctx.References) > maxReferenceImages {
		return fmt.Errorf("参考图片最多支持 %d 张", maxReferenceImages)
	}
	seen := map[string]bool{}
	seenURLs := map[string]bool{}
	references := make([]string, 0, len(ctx.References))
	for _, reference := range ctx.References {
		if reference.ID == "" || strings.TrimSpace(reference.URL) == "" || seen[reference.ID] || seenURLs[reference.URL] {
			return fmt.Errorf("参考图片 ID/URL 无效或重复")
		}
		if !containsContextValue(reference.Role, "product", "structure", "material", "scene", "composition", "lighting", "style", "hardware", "detail") {
			return fmt.Errorf("参考图片角色无效")
		}
		seen[reference.ID] = true
		seenURLs[reference.URL] = true
		references = append(references, reference.URL)
	}
	if strings.TrimSpace(ctx.Prompt) == "" {
		return fmt.Errorf("提示词不能为空")
	}
	req.ReferenceImages = references
	req.ReferenceMode = "explicit"
	req.Prompt = ctx.Prompt
	req.Profile = ctx.OutputOptions.Model
	req.AspectRatio = ctx.OutputOptions.AspectRatio
	req.ImageSize = ctx.OutputOptions.ImageSize
	req.N = ctx.OutputOptions.Count
	return nil
}

func containsContextValue(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if candidate == value {
			return true
		}
	}
	return false
}

// Provider-specific APIs accept image parts/files and text. Keep typed context
// intact for persistence; compile an instruction only at that final boundary.
func productProviderPrompt(req VectorImageRequest) string {
	ctx := req.GenerationContext
	if ctx == nil {
		return req.Prompt
	}
	var out strings.Builder
	out.WriteString(ctx.Prompt)
	out.WriteString("\n[产品一致性] 产品锁优先于场景、风格、光照参考。未知字段不得臆造。")
	if ctx.Product != nil {
		metadata, _ := json.Marshal(ctx.Product.Metadata)
		out.WriteString("\n产品：" + ctx.Product.Name + "\n已提供资料：" + string(metadata))
	}
	locks, _ := json.Marshal(ctx.Constraints)
	out.WriteString("\n必须保持的结构化产品锁（尺寸单位 mm）：" + string(locks))
	for index, reference := range ctx.References {
		out.WriteString(fmt.Sprintf("\n参考图 %d 角色=%s；仅用于该角色，不得改变产品锁。", index+1, reference.Role))
	}
	return out.String()
}

func attachProductGeneration(req VectorImageRequest, model string, assets []GeneratedAsset) {
	if req.GenerationContext == nil || req.Lineage == nil {
		return
	}
	lineage := *req.Lineage
	if lineage.CreatedAt == "" {
		lineage.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	record := &ProductGenerationRecord{
		GenerationLineage: lineage, Context: req.GenerationContext, Prompt: req.GenerationContext.Prompt,
		Model: model, AspectRatio: req.AspectRatio, GenerationType: req.GenerationContext.OutputOptions.GenerationType,
		Status: "succeeded", ReferenceIDs: []string{}, OutputAssetIDs: []string{},
	}
	if req.GenerationContext.Product != nil {
		record.RootProductAssetID = &req.GenerationContext.Product.ID
	}
	for _, ref := range req.GenerationContext.References {
		record.ReferenceIDs = append(record.ReferenceIDs, ref.ID)
	}
	for _, asset := range assets {
		record.OutputAssetIDs = append(record.OutputAssetIDs, asset.ID)
	}
	for index := range assets {
		assets[index].Generation = record
	}
}
