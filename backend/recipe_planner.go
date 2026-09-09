package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func planRecipeJobs(project StudioProject, pack ProductPack, recipe Recipe, request RecipePlanRequest) ([]GenerationJob, error) {
	if len(recipe.Outputs) == 0 {
		return nil, errors.New("Recipe 没有输出配置")
	}
	selected := map[string]bool{}
	for _, id := range request.SKUIDs {
		selected[id] = true
	}
	selectedOutputs := map[string]bool{}
	for _, outputType := range request.OutputTypes {
		selectedOutputs[outputType] = true
	}
	provider := strings.TrimSpace(request.Provider)
	if provider == "" {
		provider = "legacy"
	}
	createdBy := strings.TrimSpace(request.CreatedBy)
	if createdBy == "" {
		createdBy = project.CreatedBy
	}
	jobs := []GenerationJob{}
	for _, sku := range pack.SKUs {
		if len(selected) > 0 && !selected[sku.ID] {
			continue
		}
		for _, output := range recipe.Outputs {
			if len(selectedOutputs) > 0 && !selectedOutputs[output.OutputType] {
				continue
			}
			if output.OutputType == "" || output.AspectRatio == "" {
				return nil, errors.New("Recipe 输出配置不完整")
			}
			jobs = append(jobs, GenerationJob{
				ID: "job_" + uuid.New().String()[:8], ProjectID: project.ID, SKUID: sku.ID,
				OutputType: output.OutputType, Status: "queued", Provider: provider, Model: request.Model,
				Prompt: compileRecipePrompt(project, pack, sku, output), PromptVersion: 1,
				ReferencePack: pack.References, AspectRatio: output.AspectRatio,
				CreatedBy: createdBy, EstimatedCost: recipe.EstimatedCostPerJob,
			})
		}
	}
	if len(jobs) == 0 {
		return nil, errors.New("没有可生成的 SKU")
	}
	return jobs, nil
}

func compileRecipePrompt(project StudioProject, pack ProductPack, sku ProductSKU, output RecipeOutput) string {
	dna, _ := json.Marshal(pack.ProductDNA)
	return fmt.Sprintf("为家具电商项目「%s」生成 %s 图片。产品：%s；SKU：%s（%s）。输出比例：%s。严格保持 Product DNA，不得臆造未提供的产品事实。Product DNA：%s",
		project.Name, output.OutputType, pack.ProductName, sku.Name, sku.Label, output.AspectRatio, string(dna))
}
