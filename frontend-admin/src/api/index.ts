import type { Canvas, NodeConfig, LogEntry, Asset, AssetListResponse, StudioProject, StudioProjectDetail, ProductPack, ProjectStatus, Recipe, RecipeRun, GenerationProviderInfo } from '../types'

const BASE = '/api/admin'
const API = '/api'

async function req<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

// Node configs
export function fetchNodeConfigs() {
  return req<NodeConfig[]>(`${BASE}/node-configs`)
}

export function fetchNodeConfig(nodeType: string) {
  return req<NodeConfig>(`${BASE}/node-configs/${nodeType}`)
}

export function updateNodeConfig(nodeType: string, data: Partial<NodeConfig>) {
  return req<NodeConfig>(`${BASE}/node-configs/${nodeType}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

// Logs
export function fetchLogs(params?: { level?: string; module?: string; limit?: number; offset?: number }) {
  const search = new URLSearchParams()
  if (params?.level) search.set('level', params.level)
  if (params?.module) search.set('module', params.module)
  if (params?.limit) search.set('limit', String(params.limit))
  if (params?.offset) search.set('offset', String(params.offset))
  const qs = search.toString()
  return req<LogEntry[]>(`${BASE}/logs${qs ? '?' + qs : ''}`)
}

// Assets
export function fetchAssets(params?: { page?: number; page_size?: number; category?: string; project_id?: string; sku_id?: string; source_type?: string; role?: string; created_by?: string }) {
  const search = new URLSearchParams()
  if (params?.page) search.set('page', String(params.page))
  if (params?.page_size) search.set('page_size', String(params.page_size))
  if (params?.category) search.set('category', params.category)
	if (params?.project_id) search.set('project_id', params.project_id)
	if (params?.sku_id) search.set('sku_id', params.sku_id)
	if (params?.source_type) search.set('source_type', params.source_type)
	if (params?.role) search.set('role', params.role)
	if (params?.created_by) search.set('created_by', params.created_by)
  const qs = search.toString()
  return req<AssetListResponse>(`${BASE}/assets${qs ? '?' + qs : ''}`)
}

export async function uploadAsset(file: File, metadata?: { project_id?: string; sku_id?: string; source_type?: string; role?: string; created_by?: string }) {
  const form = new FormData()
  form.append('file', file)
	for (const [key, value] of Object.entries(metadata || {})) if (value) form.append(key, value)
	const response = await fetch(`${BASE}/assets/upload`, { method: 'POST', body: form })
	const body = await response.json().catch(() => ({ error: response.statusText }))
	if (!response.ok) throw new Error(body.error || response.statusText)
	return body as Asset
}

export function deleteAsset(id: string) {
  return req<void>(`${BASE}/assets/${id}`, { method: 'DELETE' })
}

export function updateAsset(id: string, data: { category?: string; tags?: string }) {
  return req<void>(`${BASE}/assets/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

// Canvases
export function fetchCanvases(params?: { page?: number; page_size?: number; project_type?: string }) {
  const search = new URLSearchParams()
  if (params?.page) search.set('page', String(params.page))
  if (params?.page_size) search.set('page_size', String(params.page_size))
  if (params?.project_type) search.set('project_type', params.project_type)
  const qs = search.toString()
  return req<{ items: Canvas[]; total: number; page: number; page_size: number }>(`${API}/canvases${qs ? '?' + qs : ''}`)
}

export function createCanvas(name: string, projectType: string = 'canvas') {
  return req<Canvas>(`${API}/canvases`, {
    method: 'POST',
    body: JSON.stringify({ name, project_type: projectType }),
  })
}

export function deleteCanvas(id: string) {
  return req<void>(`${API}/canvases/${id}`, { method: 'DELETE' })
}

export function renameCanvas(id: string, name: string) {
  return req<Canvas>(`${API}/canvases/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ name }),
  })
}

export function fetchProjects(params?: { page?: number; page_size?: number; status?: ProjectStatus }) {
  const search = new URLSearchParams()
  if (params?.page) search.set('page', String(params.page))
  if (params?.page_size) search.set('page_size', String(params.page_size))
  if (params?.status) search.set('status', params.status)
  const query = search.toString()
  return req<{ items: StudioProject[]; total: number; page: number; page_size: number }>(`${API}/projects${query ? '?' + query : ''}`)
}

export function createProject(data: { name: string; product_name: string; created_by: string; tags?: string[]; skus?: string[] }) {
  return req<StudioProjectDetail>(`${API}/projects`, { method: 'POST', body: JSON.stringify(data) })
}

export function updateProject(id: string, data: Partial<Pick<StudioProject, 'name' | 'product_name' | 'status' | 'cover_asset_id' | 'tags'>>) {
  return req<StudioProject>(`${API}/projects/${id}`, { method: 'PUT', body: JSON.stringify(data) })
}

export function archiveProject(id: string) {
  return req<void>(`${API}/projects/${id}`, { method: 'DELETE' })
}

export function saveProductPack(projectId: string, pack: ProductPack) {
  return req<ProductPack>(`${API}/projects/${projectId}/product-pack`, { method: 'PUT', body: JSON.stringify(pack) })
}

export function fetchProject(id: string) {
  return req<StudioProjectDetail>(`${API}/projects/${id}`)
}

export function fetchRecipes() {
  return req<Recipe[]>(`${API}/recipes`)
}

export function fetchGenerationProviders() {
	return req<GenerationProviderInfo[]>(`${API}/generation-providers`)
}

export function createRecipeRun(projectId: string, data: { recipe_id: string; request_id: string; created_by: string; sku_ids?: string[]; output_types?: string[]; provider?: string; model?: string }) {
  return req<RecipeRun>(`${API}/projects/${projectId}/recipe-runs`, { method: 'POST', body: JSON.stringify(data) })
}

export function fetchRecipeRun(id: string) {
  return req<RecipeRun>(`${API}/recipe-runs/${id}`)
}

export function fetchProjectRecipeRuns(projectId: string) {
	return req<RecipeRun[]>(`${API}/projects/${projectId}/recipe-runs`)
}

export function retryFailedJobs(id: string) {
  return req<{ retried: number; run: RecipeRun }>(`${API}/recipe-runs/${id}/retry-failed`, { method: 'POST' })
}

export function cancelGenerationJob(id: string) {
  return req<{ id: string; status: string }>(`${API}/generation-jobs/${id}/cancel`, { method: 'POST' })
}
