package main

type RecipeOutput struct {
	OutputType  string `json:"outputType"`
	AspectRatio string `json:"aspectRatio"`
}

type Recipe struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Version             int            `json:"version"`
	Outputs             []RecipeOutput `json:"outputs"`
	PromptTemplateID    string         `json:"prompt_template_id"`
	EstimatedCostPerJob float64        `json:"estimated_cost_per_job"`
	Currency            string         `json:"currency"`
	CreatedBy           string         `json:"created_by"`
	Enabled             bool           `json:"enabled"`
}

type GenerationJob struct {
	ID            string        `json:"id"`
	ProjectID     string        `json:"project_id"`
	RecipeRunID   string        `json:"recipe_run_id"`
	SKUID         string        `json:"sku_id"`
	OutputType    string        `json:"output_type"`
	Status        string        `json:"status"`
	Provider      string        `json:"provider"`
	Model         string        `json:"model"`
	Prompt        string        `json:"prompt"`
	PromptVersion int           `json:"prompt_version"`
	ReferencePack ReferencePack `json:"reference_pack"`
	AspectRatio   string        `json:"aspect_ratio"`
	CreatedBy     string        `json:"created_by"`
	CreatedAt     string        `json:"created_at"`
	StartedAt     string        `json:"started_at"`
	FinishedAt    string        `json:"finished_at"`
	EstimatedCost float64       `json:"estimated_cost"`
	ActualCost    float64       `json:"actual_cost"`
	DurationMS    int64         `json:"duration_ms"`
	RetryCount    int           `json:"retry_count"`
	ResultAssetID string        `json:"result_asset_id"`
	ErrorCode     string        `json:"error_code"`
	ErrorMessage  string        `json:"error_message"`
}

type RecipeRun struct {
	ID            string          `json:"id"`
	ProjectID     string          `json:"project_id"`
	RecipeID      string          `json:"recipe_id"`
	RecipeVersion int             `json:"recipe_version"`
	RequestID     string          `json:"request_id"`
	Status        string          `json:"status"`
	JobCount      int             `json:"job_count"`
	TotalCost     float64         `json:"total_cost"`
	Currency      string          `json:"currency"`
	CreatedBy     string          `json:"created_by"`
	CreatedAt     string          `json:"created_at"`
	Jobs          []GenerationJob `json:"jobs"`
}

type RecipePlanRequest struct {
	RecipeID    string   `json:"recipe_id"`
	RequestID   string   `json:"request_id"`
	CreatedBy   string   `json:"created_by"`
	SKUIDs      []string `json:"sku_ids"`
	OutputTypes []string `json:"output_types"`
	Provider    string   `json:"provider"`
	Model       string   `json:"model"`
}
