import { reactive } from 'vue'

export interface GeneratedImageAsset {
  id: string
  filename: string
  url: string
  size: number
  mime_type: string
  width: number
  height: number
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

async function notify(event: ImageGenerationEvent) {
  await Promise.all([...listeners].map(listener => listener(event)))
}

async function startImageGeneration(request: ImageGenerationRequest) {
  if (tasks[request.nodeId]?.status === 'running') return false

  tasks[request.nodeId] = {
    taskId: request.taskId,
    status: 'running',
    error: '',
    assets: [],
    startedAt: Date.now(),
    finishedAt: null,
  }

  try {
    const response = await fetch('/api/images/generate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
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
      }),
    })
    if (!response.ok) {
      const body = await response.json().catch(() => ({ error: response.statusText }))
      throw new Error(typeof body.error === 'string' ? body.error : (body.error?.message || response.statusText))
    }

    const body = await response.json()
    const assets = Array.isArray(body.data) ? body.data as GeneratedImageAsset[] : []
    if (assets.length === 0) throw new Error('图片生成完成，但未返回可展示的结果')

    tasks[request.nodeId].assets = assets
    await notify({ type: 'completed', request, assets })
    tasks[request.nodeId].status = 'succeeded'
  } catch (error: any) {
    const message = error?.message || '图片生成失败'
    tasks[request.nodeId].status = 'failed'
    tasks[request.nodeId].error = message
    await notify({ type: 'failed', request, error: message })
  } finally {
    tasks[request.nodeId].finishedAt = Date.now()
  }
  return true
}

function onImageGenerationEvent(listener: ImageGenerationListener) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

export function useImageGenerationTasks() {
  return { tasks, startImageGeneration, onImageGenerationEvent }
}
