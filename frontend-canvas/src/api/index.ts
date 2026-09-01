import type { Canvas, Node, Edge, Asset } from '../types'

const BASE = '/api'

async function req<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    const msg = typeof body.error === 'string' ? body.error : (body.error?.message || body.error?.msg || res.statusText)
    throw new Error(msg)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

// Canvas
export function fetchCanvases() {
  return req<Canvas[]>(`${BASE}/canvases`)
}

export function createCanvas(name: string) {
  return req<Canvas>(`${BASE}/canvases`, {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
}

export function fetchCanvas(id: string) {
  return req<Canvas>(`${BASE}/canvases/${id}`)
}

export function updateCanvas(id: string, name: string) {
  return req<Canvas>(`${BASE}/canvases/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ name }),
  })
}

// Nodes
export function createNode(canvasId: string, data: Partial<Node>) {
  return req<Node>(`${BASE}/canvases/${canvasId}/nodes`, {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export function updateNode(canvasId: string, id: string, data: Partial<Node>) {
  return req<Node>(`${BASE}/canvases/${canvasId}/nodes/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

export function deleteNode(canvasId: string, id: string) {
  return req<void>(`${BASE}/canvases/${canvasId}/nodes/${id}`, {
    method: 'DELETE',
  })
}

// Edges
export function fetchEdges(canvasId: string) {
  return req<Edge[]>(`${BASE}/canvases/${canvasId}/edges`)
}

export function createEdge(canvasId: string, sourceNodeId: string, targetNodeId: string) {
  return req<Edge>(`${BASE}/canvases/${canvasId}/edges`, {
    method: 'POST',
    body: JSON.stringify({ source_node_id: sourceNodeId, target_node_id: targetNodeId }),
  })
}

export function deleteEdge(canvasId: string, edgeId: string) {
  return req<void>(`${BASE}/canvases/${canvasId}/edges/${edgeId}`, {
    method: 'DELETE',
  })
}

// Assets (personal asset library)
export function fetchAssets() {
  return req<any>(`/api/admin/assets?page_size=10000`).then(r => r.items || r)
}

// Node configs
export function fetchNodeConfigs() {
  return req<any[]>(`/api/admin/node-configs`)
}

async function readImageSize(file: File): Promise<{ width: number; height: number }> {
  if (!file.type.startsWith('image/')) return { width: 0, height: 0 }
  if ('createImageBitmap' in window) {
    try {
      const bitmap = await createImageBitmap(file)
      const result = { width: bitmap.width, height: bitmap.height }
      bitmap.close()
      return result
    } catch { /* use HTMLImageElement fallback */ }
  }
  return new Promise(resolve => {
    const url = URL.createObjectURL(file)
    const image = new Image()
    image.onload = () => {
      resolve({ width: image.naturalWidth, height: image.naturalHeight })
      URL.revokeObjectURL(url)
    }
    image.onerror = () => {
      resolve({ width: 0, height: 0 })
      URL.revokeObjectURL(url)
    }
    image.src = url
  })
}

export async function uploadAsset(file: File): Promise<Asset> {
  const dimensions = await readImageSize(file)
  const fd = new FormData()
  fd.append('file', file)
  fd.append('mime_type', file.type || 'application/octet-stream')
  fd.append('width', String(dimensions.width))
  fd.append('height', String(dimensions.height))
  const response = await fetch('/api/admin/assets/upload', { method: 'POST', body: fd })
  if (!response.ok) {
    const error = await response.json().catch(() => ({}))
    throw new Error(error.error || response.statusText)
  }
  return response.json()
}

export function updateAsset(id: string, data: { category?: string; tags?: string }) {
  return req<void>(`/api/admin/assets/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

export function deleteAsset(id: string) {
  return req<void>(`/api/admin/assets/${id}`, { method: 'DELETE' })
}

// LLM
export function chatWithLLM(model: string, messages: { role: string; content: any }[], modelConfig?: { channel?: string; base_url?: string; parameters?: Record<string, any> } | null) {
  const body: Record<string, any> = { model, messages }
  if (modelConfig) {
    if (modelConfig.channel) body.channel = modelConfig.channel
    if (modelConfig.base_url) body.base_url = modelConfig.base_url
    if (modelConfig.parameters) body.parameters = modelConfig.parameters
  }
  return req<any>(`${BASE}/llm/chat`, {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export async function* chatWithLLMStream(model: string, messages: { role: string; content: any }[], modelConfig?: { channel?: string; base_url?: string; parameters?: Record<string, any> } | null) {
  const body: Record<string, any> = { model, messages, stream: true }
  if (modelConfig) {
    if (modelConfig.channel) body.channel = modelConfig.channel
    if (modelConfig.base_url) body.base_url = modelConfig.base_url
    if (modelConfig.parameters) body.parameters = modelConfig.parameters
  }
  const res = await fetch(`${BASE}/llm/chat`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    const msg = typeof body.error === 'string' ? body.error : (body.error?.message || body.error?.msg || res.statusText)
    throw new Error(msg)
  }
  const reader = res.body!.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split('\n')
    buffer = lines.pop() || ''
    for (const line of lines) {
      if (line.startsWith('data: ')) {
        const data = line.slice(6).trim()
        if (data === '[DONE]') return
        try {
          const json = JSON.parse(data)
          const content = json.choices?.[0]?.delta?.content
          if (content) yield content
        } catch { /* skip unparseable chunks */ }
      }
    }
  }
}

export function chatWithAssistant(
  messages: { role: 'user' | 'assistant'; content: string }[],
  selectedImages: string[] = [],
  canvasId = '',
  selectedNodeIds: string[] = [],
) {
  return req<{ content: string; model_profile: 'assistant' }>(`${BASE}/assistant/chat`, {
    method: 'POST',
    body: JSON.stringify({ messages, selected_images: selectedImages, canvas_id: canvasId, selected_node_ids: selectedNodeIds }),
  })
}
