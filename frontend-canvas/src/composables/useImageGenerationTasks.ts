import { reactive } from 'vue'
import type { GenerationContext, GenerationRecord } from '../types/product'
import { createGenerationRecord } from '../utils/generationContext'
import { useProductAssets } from './useProductAssets'

export interface GeneratedImageAsset {
  id: string
  filename: string
  url: string
  size: number
  mime_type: string
  width: number
  height: number
  generation?: GenerationRecord
}

export interface ImageGenerationRequest {
  taskId: string
  canvasId: string
  nodeId: string
  profile: string
  prompt: string
  count: number
  aspectRatio: string
  imageSize: string
  referenceImages: string[]
  context?: GenerationContext
  parentGenerationIds?: string[]
}

export interface ImageGenerationTaskState {
  taskId: string
  status: 'running' | 'succeeded' | 'failed'
  error: string
  assets: GeneratedImageAsset[]
  startedAt: number
  finishedAt: number | null
}

export type ImageGenerationEvent =
  | { type: 'completed'; request: ImageGenerationRequest; assets: GeneratedImageAsset[] }
  | { type: 'failed'; request: ImageGenerationRequest; error: string }

type ImageGenerationListener = (event: ImageGenerationEvent) => void | Promise<void>

const tasks = reactive<Record<string, ImageGenerationTaskState>>({})
const listeners = new Set<ImageGenerationListener>()
const controllers = new Map<string, AbortController>()

async function notify(event: ImageGenerationEvent) {
  await Promise.all([...listeners].map(listener => listener(event)))
}

async function startImageGeneration(request: ImageGenerationRequest) {
  if (tasks[request.nodeId]?.status === 'running') return false
  request = JSON.parse(JSON.stringify(request))
  const productStore = useProductAssets()
  let generation = request.context ? createGenerationRecord(request.taskId, request.context, request.parentGenerationIds || []) : null

  const controller = new AbortController()
  controllers.set(request.nodeId, controller)
  tasks[request.nodeId] = {
    taskId: request.taskId,
    status: 'running',
    error: '',
    assets: [],
    startedAt: Date.now(),
    finishedAt: null,
  }

  try {
    if (generation) productStore.saveGeneration(generation)
    const response = await fetch('/api/images/generate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      signal: controller.signal,
      body: JSON.stringify({
        task_id: request.taskId,
        canvas_id: request.canvasId,
        node_id: request.nodeId,
        profile: request.profile,
        prompt: request.prompt,
        n: request.count,
        aspect_ratio: request.aspectRatio,
        image_size: request.imageSize,
        reference_images: [...new Set(request.referenceImages)],
        reference_mode: 'explicit',
        generation_context: request.context,
        lineage: generation ? { id: generation.id, parentGenerationId: generation.parentGenerationId, parentGenerationIds: generation.parentGenerationIds, createdAt: generation.createdAt } : undefined,
      }),
    })
    if (!response.ok) {
      const body = await response.json().catch(() => ({ error: response.statusText }))
      throw new Error(typeof body.error === 'string' ? body.error : (body.error?.message || response.statusText))
    }

    const body = await response.json()
    let assets = Array.isArray(body.data) ? body.data as GeneratedImageAsset[] : []
    if (assets.length === 0) throw new Error('图片生成完成，但未返回可展示的结果')

    if (tasks[request.nodeId]?.taskId !== request.taskId) return false
    if (generation) {
      generation = { ...generation, ...(assets[0].generation || {}), status: 'succeeded', outputAssetIds: assets.map(asset => asset.id) }
      assets = assets.map(asset => ({ ...asset, generation: generation! }))
      // The backend also persists the context on each output. If local storage
      // fills during generation, still render the completed images and show it.
      try { productStore.saveGeneration(generation) }
      catch (error) { tasks[request.nodeId].error = (error as Error).message }
    }
    tasks[request.nodeId].assets = assets
    await notify({ type: 'completed', request, assets })
    if (tasks[request.nodeId]?.taskId === request.taskId) {
      tasks[request.nodeId].status = 'succeeded'
    }
  } catch (error: any) {
    if (error?.name === 'AbortError' || tasks[request.nodeId]?.taskId !== request.taskId) return false
    const message = error?.message || '图片生成失败'
    tasks[request.nodeId].status = 'failed'
    tasks[request.nodeId].error = message
    if (generation) {
      try { productStore.saveGeneration({ ...generation, status: 'failed', error: message }) } catch { /* storageError is shown in the product binding */ }
    }
    await notify({ type: 'failed', request, error: message })
  } finally {
    if (controllers.get(request.nodeId) === controller) controllers.delete(request.nodeId)
    if (tasks[request.nodeId]?.taskId === request.taskId) {
      tasks[request.nodeId].finishedAt = Date.now()
    }
  }
  return true
}

function cancelImageGeneration(nodeId: string) {
  const store = useProductAssets()
  const record = store.getGeneration(tasks[nodeId]?.taskId || '')
  if (record?.status === 'running') {
    try { store.saveGeneration({ ...record, status: 'cancelled' }) } catch { /* storageError is visible in the product UI */ }
  }
  controllers.get(nodeId)?.abort()
  controllers.delete(nodeId)
  delete tasks[nodeId]
}

function onImageGenerationEvent(listener: ImageGenerationListener) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

export function useImageGenerationTasks() {
  return { tasks, startImageGeneration, cancelImageGeneration, onImageGenerationEvent }
}
