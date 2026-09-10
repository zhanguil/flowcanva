export interface Canvas {
  id: string
  name: string
  project_type: string
  created_at: string
  updated_at: string
  nodes?: Node[]
}

export interface Node {
  id: string
  canvas_id: string
  node_type: string
  x: number
  y: number
  width: number
  height: number
  content: string
  config: string
  created_at: string
  updated_at: string
}

export interface NodeConfig {
  id: string
  node_type: string
  model_name: string
  api_channel: string
  base_url: string
  api_key: string
  parameters: string
  prompt_template: string
  extra_config: string
  enabled: number
  created_at: string
  updated_at: string
}

export interface LogEntry {
  id: string
  level: string
  module: string
  message: string
  detail: string
  created_at: string
}

export interface Asset {
  id: string
  filename: string
  url: string
  size: number
  width: number
  height: number
  category: string
  tags: string
  created_at: string
	project_id?: string
	sku_id?: string
	source_type?: string
	role?: string
	parent_asset_id?: string
	created_by?: string
	generation_metadata?: string
}

export interface AssetListResponse {
  items: Asset[]
  total: number
  page: number
  page_size: number
}

export type ProjectStatus = 'draft' | 'generating' | 'reviewing' | 'completed' | 'archived'

export interface StudioProject {
  id: string
  workspace_id: string
  canvas_id: string
  name: string
  product_name: string
  created_by: string
  status: ProjectStatus
  cover_asset_id: string
  tags: string[]
  created_at: string
  updated_at: string
}

export interface ProductDNA {
  productType: string
  structuralFeatures: Record<string, string | number | boolean>
  materials: Record<string, string>
  forbiddenChanges: string[]
  allowedChanges: string[]
}

export interface ProductSKU {
  id?: string
  name: string
  label: string
  width?: number | null
  height?: number | null
  depth?: number | null
  reference_asset_ids?: string[]
  sort_order?: number
}

export interface ReferenceAsset {
  id?: string
  asset_id: string
  role: string
  weight: number
  locked: boolean
  description?: string
  sort_order?: number
}

export interface ProductPack {
  id?: string
  project_id?: string
  product_name: string
  created_by?: string
  product_dna: ProductDNA
  skus: ProductSKU[]
  reference_pack: { references: ReferenceAsset[] }
}

export interface StudioProjectDetail extends StudioProject {
  product_pack?: ProductPack
}

export interface RecipeOutput {
  outputType: string
  aspectRatio: string
}

export interface RecipeRunRequest {
  recipe_id: string
  request_id: string
  created_by: string
  sku_ids?: string[]
  output_types?: string[]
  selected_outputs?: RecipeOutput[]
  copies_per_item?: number
  provider?: string
  model?: string
}

export interface Recipe {
  id: string
  name: string
  version: number
  outputs: RecipeOutput[]
  prompt_template_id: string
  estimated_cost_per_job: number
  currency: string
  enabled: boolean
}

export type GenerationJobStatus = 'pending' | 'queued' | 'running' | 'generated' | 'validating' | 'success' | 'failed' | 'rejected' | 'cancelled' | 'retrying'

export interface GenerationJob {
  id: string
  project_id: string
  recipe_run_id: string
  sku_id: string
  output_type: string
  status: GenerationJobStatus
  provider: string
  model: string
  aspect_ratio: string
  retry_count: number
  result_asset_id: string
  error_message: string
  estimated_cost: number
  actual_cost: number
}

export interface RecipeRun {
  id: string
  project_id: string
  recipe_id: string
  recipe_version: number
  request_id: string
  status: string
  job_count: number
  total_cost: number
  currency: string
  created_by: string
  created_at: string
  jobs: GenerationJob[]
}

export interface GenerationProviderInfo {
	id: string
	name: string
	capabilities: {
		text_to_image: boolean
		image_edit: boolean
		multi_reference: boolean
		max_reference_images: number
		supported_aspect_ratios: string[]
		supports_seed: boolean
		supports_mask: boolean
		supports_async: boolean
	}
}

export interface AssistantAnalysisResult {
  content: string
  model_profile: string
}
