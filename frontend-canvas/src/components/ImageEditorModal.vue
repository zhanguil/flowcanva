<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, shallowRef, watch } from 'vue'
import {
  IconArrowBackUp,
  IconArrowForwardUp,
  IconArrowLeft,
  IconArrowUpRight,
  IconArrowsMaximize,
  IconBrush,
  IconCheck,
  IconCircle,
  IconCrop,
  IconDeviceFloppy,
  IconDimensions,
  IconGridDots,
  IconLine,
  IconLock,
  IconLockOpen,
  IconMaximize,
  IconPhoto,
  IconRectangle,
  IconShield,
  IconTypography,
  IconX,
  IconZoomIn,
  IconZoomOut,
} from '@tabler/icons-vue'

type EditorTool = 'view' | 'mask' | 'brush' | 'text' | 'crop' | 'expand' | 'resize' | 'grid'
type DrawShape = 'freehand' | 'rectangle' | 'circle' | 'arrow'

interface HistoryFrame {
  width: number
  height: number
  pixels: ImageData
  mask?: ImageData
}

const props = withDefaults(defineProps<{
  open: boolean
  src: string
  filename?: string
  saving?: boolean
  saveError?: string
}>(), {
  filename: 'image.png',
  saving: false,
})

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'save', file: File, mode: 'replace' | 'copy'): void
  (event: 'grid-split', payload: { cols: number; rows: number; urls: string[] }): void
}>()

const canvasRef = ref<HTMLCanvasElement | null>(null)
const maskRef = ref<HTMLCanvasElement | null>(null)
const hasMask = ref(false)
const saveMode = ref<'replace' | 'copy'>('replace')
let sourceImage: HTMLImageElement | null = null
let loadVersion = 0
let panStart: { x: number; y: number; left: number; top: number } | null = null
const stageRef = ref<HTMLDivElement | null>(null)
const loading = ref(false)
const errorMessage = ref('')
const activeTool = ref<EditorTool>('view')
const drawShape = ref<DrawShape>('freehand')
const zoom = ref(1)
const brushSize = ref(25)
const feather = ref(8)
const opacity = ref(1)
const color = ref('#ff4545')
const textValue = ref('双击输入文字')
const fontSize = ref(48)
const keepRatio = ref(true)
const resizeWidth = ref(0)
const resizeHeight = ref(0)
const expandWidth = ref(0)
const expandHeight = ref(0)
const gridCols = ref(2)
const gridRows = ref(2)
const cropRect = ref<{ x: number; y: number; width: number; height: number } | null>(null)
const pointerStart = reactive({ x: 0, y: 0 })
const drawing = ref(false)
const history = shallowRef<HistoryFrame[]>([])
const future = shallowRef<HistoryFrame[]>([])
let draftFrame: HistoryFrame | null = null
function limitedFrames(frames: HistoryFrame[]) {
  let bytes = 0
  return frames.slice(-20).reverse().filter((frame, index) => {
    bytes += frame.pixels.data.byteLength + (frame.mask?.data.byteLength || 0)
    return index === 0 || bytes <= 128 * 1024 * 1024
  }).reverse()
}
function drawingContext() {
  if (activeTool.value !== 'mask') return canvasContext()
  hasMask.value = true
  return maskRef.value?.getContext('2d', { willReadFrequently: true }) || null
}
function transformMask(width: number, height: number, draw: (ctx: CanvasRenderingContext2D, mask: HTMLCanvasElement) => void) {
  const mask = maskRef.value
  if (!mask) return
  const temporary = document.createElement('canvas'); temporary.width = width; temporary.height = height
  if (hasMask.value) draw(temporary.getContext('2d')!, mask)
  mask.width = width; mask.height = height; mask.getContext('2d')?.drawImage(temporary, 0, 0)
}
function clearMask() {
  checkpoint(); maskRef.value?.getContext('2d')?.clearRect(0, 0, canvasWidth.value, canvasHeight.value); hasMask.value = false
}
function resetImage() {
  if (!sourceImage || loading.value) return
  checkpoint()
  const canvas = canvasRef.value!, mask = maskRef.value!
  canvas.width = sourceImage.naturalWidth; canvas.height = sourceImage.naturalHeight
  canvasContext()?.drawImage(sourceImage, 0, 0)
  mask.width = canvas.width; mask.height = canvas.height; hasMask.value = false
  cropRect.value = null; syncDimensionInputs(); fitToWindow()
}
function exportMask() {
  if (!maskRef.value || !hasMask.value) return
  const output = document.createElement('canvas'); output.width = canvasWidth.value; output.height = canvasHeight.value
  const ctx = output.getContext('2d')!; ctx.fillStyle = '#000'; ctx.fillRect(0, 0, output.width, output.height); ctx.drawImage(maskRef.value, 0, 0)
  output.toBlob(blob => {
    if (!blob) return
    const url = URL.createObjectURL(blob), link = document.createElement('a')
    link.href = url; link.download = 'selection-mask.png'; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000)
  }, 'image/png')
}

const tools: { id: EditorTool; label: string; icon: any }[] = [
  { id: 'mask', label: '遮罩', icon: IconShield },
  { id: 'brush', label: '画笔', icon: IconBrush },
  { id: 'text', label: '文字', icon: IconTypography },
  { id: 'crop', label: '裁剪', icon: IconCrop },
  { id: 'expand', label: '扩图', icon: IconArrowsMaximize },
  { id: 'resize', label: '尺寸', icon: IconDimensions },
  { id: 'grid', label: '宫格拆分', icon: IconGridDots },
]

const shapes: { id: DrawShape; label: string; icon: any }[] = [
  { id: 'freehand', label: '自由画笔', icon: IconBrush },
  { id: 'rectangle', label: '矩形', icon: IconRectangle },
  { id: 'circle', label: '圆形', icon: IconCircle },
  { id: 'arrow', label: '箭头', icon: IconArrowUpRight },
]

const colors = ['#000000', '#ffffff', '#ff4545', '#34c759', '#1687ff', '#ffd21e']
const canvasWidth = ref(0)
const canvasHeight = ref(0)
const zoomLabel = computed(() => `${Math.round(zoom.value * 100)}%`)
const canUndo = computed(() => history.value.length > 0)
const canRedo = computed(() => future.value.length > 0)
const stageCursor = computed(() => {
  if (activeTool.value === 'view') return 'grab'
  if (activeTool.value === 'text') return 'text'
  return 'crosshair'
})

function canvasContext() {
  return canvasRef.value?.getContext('2d', { willReadFrequently: true }) || null
}

function captureFrame(): HistoryFrame | null {
  const canvas = canvasRef.value
  const ctx = canvasContext()
  if (!canvas || !ctx || canvas.width === 0 || canvas.height === 0) return null
  return { width: canvas.width, height: canvas.height, pixels: ctx.getImageData(0, 0, canvas.width, canvas.height), mask: hasMask.value ? maskRef.value?.getContext('2d')?.getImageData(0, 0, canvas.width, canvas.height) : undefined }
}

function restoreFrame(frame: HistoryFrame) {
  const canvas = canvasRef.value
  if (!canvas) return
  if (canvas.width !== frame.width || canvas.height !== frame.height) { canvas.width = frame.width; canvas.height = frame.height }
  const mask = maskRef.value
  if (mask) {
    mask.width = frame.width; mask.height = frame.height
    if (frame.mask) mask.getContext('2d')?.putImageData(frame.mask, 0, 0)
  }
  hasMask.value = Boolean(frame.mask)
  canvasContext()?.putImageData(frame.pixels, 0, 0)
  syncDimensionInputs()
}

function checkpoint() {
  const frame = captureFrame()
  if (!frame) return
  history.value = limitedFrames([...history.value, frame])
  future.value = []
  return frame
}

function undo() {
  const previous = history.value[history.value.length - 1]
  const current = captureFrame()
  if (!previous || !current) return
  history.value = history.value.slice(0, -1)
  future.value = limitedFrames([...future.value, current])
  restoreFrame(previous)
}

function redo() {
  const next = future.value[future.value.length - 1]
  const current = captureFrame()
  if (!next || !current) return
  future.value = future.value.slice(0, -1)
  history.value = limitedFrames([...history.value, current])
  restoreFrame(next)
}

function syncDimensionInputs() {
  const canvas = canvasRef.value
  if (!canvas) return
  canvasWidth.value = canvas.width
  canvasHeight.value = canvas.height
  resizeWidth.value = canvas.width
  resizeHeight.value = canvas.height
  expandWidth.value = canvas.width
  expandHeight.value = canvas.height
}

function selectTool(tool: EditorTool) {
  activeTool.value = activeTool.value === tool ? 'view' : tool
  cropRect.value = null
}

function imagePoint(event: PointerEvent) {
  const canvas = canvasRef.value
  if (!canvas) return { x: 0, y: 0 }
  const rect = canvas.getBoundingClientRect()
  return {
    x: Math.max(0, Math.min(canvas.width, (event.clientX - rect.left) * canvas.width / rect.width)),
    y: Math.max(0, Math.min(canvas.height, (event.clientY - rect.top) * canvas.height / rect.height)),
  }
}

function configureStroke(ctx: CanvasRenderingContext2D) {
  ctx.lineCap = 'round'
  ctx.lineJoin = 'round'
  ctx.lineWidth = brushSize.value
  ctx.strokeStyle = activeTool.value === 'mask' ? '#ffffff' : color.value
  ctx.fillStyle = activeTool.value === 'mask' ? '#ffffff' : color.value
  ctx.globalAlpha = activeTool.value === 'mask' ? 1 : opacity.value
  ctx.shadowColor = activeTool.value === 'mask' ? '#ffffff' : color.value
  ctx.shadowBlur = feather.value
}

function drawArrow(ctx: CanvasRenderingContext2D, sx: number, sy: number, ex: number, ey: number) {
  const angle = Math.atan2(ey - sy, ex - sx)
  const head = Math.max(14, brushSize.value * 2.2)
  ctx.beginPath()
  ctx.moveTo(sx, sy)
  ctx.lineTo(ex, ey)
  ctx.stroke()
  ctx.beginPath()
  ctx.moveTo(ex, ey)
  ctx.lineTo(ex - head * Math.cos(angle - Math.PI / 6), ey - head * Math.sin(angle - Math.PI / 6))
  ctx.lineTo(ex - head * Math.cos(angle + Math.PI / 6), ey - head * Math.sin(angle + Math.PI / 6))
  ctx.closePath()
  ctx.fill()
}

function drawShapePreview(endX: number, endY: number) {
  const ctx = drawingContext()
  if (!ctx || !draftFrame) return
  restoreFrame(draftFrame)
  if (activeTool.value === 'mask') hasMask.value = true
  ctx.save()
  configureStroke(ctx)
  const x = Math.min(pointerStart.x, endX)
  const y = Math.min(pointerStart.y, endY)
  const width = Math.abs(endX - pointerStart.x)
  const height = Math.abs(endY - pointerStart.y)
  if (drawShape.value === 'rectangle') ctx.strokeRect(x, y, width, height)
  if (drawShape.value === 'circle') {
    ctx.beginPath()
    ctx.ellipse(x + width / 2, y + height / 2, width / 2, height / 2, 0, 0, Math.PI * 2)
    ctx.stroke()
  }
  if (drawShape.value === 'arrow') drawArrow(ctx, pointerStart.x, pointerStart.y, endX, endY)
  ctx.restore()
}

function onPointerDown(event: PointerEvent) {
  if (loading.value || props.saving) return
  if (activeTool.value === 'view' && stageRef.value) {
    panStart = { x: event.clientX, y: event.clientY, left: stageRef.value.scrollLeft, top: stageRef.value.scrollTop }
    canvasRef.value?.setPointerCapture(event.pointerId); return
  }
  if (activeTool.value === 'expand' || activeTool.value === 'resize' || activeTool.value === 'grid') return
  const canvas = canvasRef.value
  const point = imagePoint(event)
  if (!canvas) return
  canvas.setPointerCapture(event.pointerId)
  pointerStart.x = point.x
  pointerStart.y = point.y

  if (activeTool.value === 'text') {
    if (!textValue.value.trim()) return
    checkpoint()
    const ctx = canvasContext()
    if (!ctx) return
    ctx.save()
    ctx.globalAlpha = opacity.value
    ctx.fillStyle = color.value
    ctx.font = `600 ${fontSize.value}px Inter, "Microsoft YaHei", sans-serif`
    ctx.textBaseline = 'top'
    ctx.shadowColor = 'rgba(0,0,0,.45)'
    ctx.shadowBlur = Math.min(8, feather.value)
    ctx.fillText(textValue.value.trim(), point.x, point.y)
    ctx.restore()
    return
  }

  if (activeTool.value === 'crop') {
    drawing.value = true
    cropRect.value = { x: point.x, y: point.y, width: 0, height: 0 }
    return
  }

  draftFrame = checkpoint() || null
  drawing.value = true
  if (drawShape.value === 'freehand') {
    const ctx = drawingContext()
    if (!ctx) return
    ctx.save()
    configureStroke(ctx)
    ctx.beginPath()
    ctx.moveTo(point.x, point.y)
    ctx.lineTo(point.x + 0.01, point.y + 0.01)
    ctx.stroke()
    ctx.restore()
  }
}

function onPointerMove(event: PointerEvent) {
  if (panStart && stageRef.value) {
    stageRef.value.scrollLeft = panStart.left - (event.clientX - panStart.x)
    stageRef.value.scrollTop = panStart.top - (event.clientY - panStart.y); return
  }
  if (!drawing.value) return
  const point = imagePoint(event)
  if (activeTool.value === 'crop') {
    cropRect.value = {
      x: Math.min(pointerStart.x, point.x),
      y: Math.min(pointerStart.y, point.y),
      width: Math.abs(point.x - pointerStart.x),
      height: Math.abs(point.y - pointerStart.y),
    }
    return
  }
  if (drawShape.value !== 'freehand') {
    drawShapePreview(point.x, point.y)
    return
  }
  const ctx = drawingContext()
  if (!ctx) return
  ctx.save()
  configureStroke(ctx)
  ctx.beginPath()
  ctx.moveTo(pointerStart.x, pointerStart.y)
  ctx.lineTo(point.x, point.y)
  ctx.stroke()
  ctx.restore()
  pointerStart.x = point.x
  pointerStart.y = point.y
}

function onPointerUp(event: PointerEvent) {
  if (panStart) { panStart = null; if (canvasRef.value?.hasPointerCapture(event.pointerId)) canvasRef.value.releasePointerCapture(event.pointerId) }
  if (!drawing.value) return
  drawing.value = false
  draftFrame = null
  canvasRef.value?.releasePointerCapture(event.pointerId)
}

function cropOverlayStyle() {
  const rect = cropRect.value
  const canvas = canvasRef.value
  if (!rect || !canvas) return {}
  return {
    left: `${rect.x / canvas.width * 100}%`,
    top: `${rect.y / canvas.height * 100}%`,
    width: `${rect.width / canvas.width * 100}%`,
    height: `${rect.height / canvas.height * 100}%`,
  }
}

function replaceCanvasFrom(source: HTMLCanvasElement) {
  const canvas = canvasRef.value
  if (!canvas) return
  canvas.width = source.width
  canvas.height = source.height
  canvasContext()?.drawImage(source, 0, 0)
  syncDimensionInputs()
}

function applyCrop() {
  const canvas = canvasRef.value
  const rect = cropRect.value
  if (!canvas || !rect || rect.width < 2 || rect.height < 2) return
  checkpoint()
  const output = document.createElement('canvas')
  output.width = Math.max(1, Math.round(rect.width))
  output.height = Math.max(1, Math.round(rect.height))
  output.getContext('2d')?.drawImage(canvas, rect.x, rect.y, rect.width, rect.height, 0, 0, output.width, output.height)
  transformMask(output.width, output.height, (ctx, mask) => ctx.drawImage(mask, rect.x, rect.y, rect.width, rect.height, 0, 0, output.width, output.height))
  replaceCanvasFrom(output)
  cropRect.value = null
  fitToWindow()
}

function onResizeWidth(value: number) {
  resizeWidth.value = Math.max(1, Math.round(value || 1))
  if (keepRatio.value && canvasWidth.value > 0) resizeHeight.value = Math.max(1, Math.round(resizeWidth.value * canvasHeight.value / canvasWidth.value))
}

function onResizeHeight(value: number) {
  resizeHeight.value = Math.max(1, Math.round(value || 1))
  if (keepRatio.value && canvasHeight.value > 0) resizeWidth.value = Math.max(1, Math.round(resizeHeight.value * canvasWidth.value / canvasHeight.value))
}

function applyResize() {
  const canvas = canvasRef.value
  if (!canvas || resizeWidth.value < 1 || resizeHeight.value < 1) return
  checkpoint()
  const output = document.createElement('canvas')
  output.width = resizeWidth.value
  output.height = resizeHeight.value
  output.getContext('2d')?.drawImage(canvas, 0, 0, output.width, output.height)
  transformMask(output.width, output.height, (ctx, mask) => ctx.drawImage(mask, 0, 0, output.width, output.height))
  replaceCanvasFrom(output)
  fitToWindow()
}

function applyExpand() {
  const canvas = canvasRef.value
  if (!canvas) return
  const width = Math.max(canvas.width, Math.round(expandWidth.value || canvas.width))
  const height = Math.max(canvas.height, Math.round(expandHeight.value || canvas.height))
  if (width === canvas.width && height === canvas.height) return
  checkpoint()
  const output = document.createElement('canvas')
  output.width = width
  output.height = height
  const ctx = output.getContext('2d')
  if (!ctx) return
  ctx.fillStyle = color.value
  ctx.globalAlpha = opacity.value
  ctx.fillRect(0, 0, width, height)
  ctx.globalAlpha = 1
  ctx.drawImage(canvas, Math.round((width - canvas.width) / 2), Math.round((height - canvas.height) / 2))
  transformMask(width, height, (ctx, mask) => ctx.drawImage(mask, Math.round((width - canvas.width) / 2), Math.round((height - canvas.height) / 2)))
  replaceCanvasFrom(output)
  fitToWindow()
}

function splitGrid() {
  const canvas = canvasRef.value
  if (!canvas || gridCols.value < 2 || gridRows.value < 2) return
  const cellWidth = canvas.width / gridCols.value
  const cellHeight = canvas.height / gridRows.value
  const output = document.createElement('canvas')
  output.width = Math.round(cellWidth)
  output.height = Math.round(cellHeight)
  const ctx = output.getContext('2d')
  if (!ctx) return
  const urls: string[] = []
  for (let row = 0; row < gridRows.value; row++) {
    for (let col = 0; col < gridCols.value; col++) {
      ctx.clearRect(0, 0, output.width, output.height)
      ctx.drawImage(canvas, col * cellWidth, row * cellHeight, cellWidth, cellHeight, 0, 0, output.width, output.height)
      urls.push(output.toDataURL('image/png'))
    }
  }
  emit('grid-split', { cols: gridCols.value, rows: gridRows.value, urls })
}

function changeZoom(delta: number) {
  zoom.value = Math.max(0.1, Math.min(3, Number((zoom.value + delta).toFixed(2))))
}

function fitToWindow() {
  nextTick(() => {
    const stage = stageRef.value
    const canvas = canvasRef.value
    if (!stage || !canvas || canvas.width === 0 || canvas.height === 0) return
    const availableWidth = Math.max(100, stage.clientWidth - 64)
    const availableHeight = Math.max(100, stage.clientHeight - 64)
    zoom.value = Math.max(0.1, Math.min(1, Math.min(availableWidth / canvas.width, availableHeight / canvas.height)))
  })
}

function resetZoom() {
  zoom.value = 1
}

function exportFile() {
  const canvas = canvasRef.value
  if (!canvas || props.saving) return
  canvas.toBlob(blob => {
    if (!blob) return
    const stem = (props.filename || 'image').replace(/\.[a-z0-9]{2,5}$/i, '')
    emit('save', new File([blob], `${stem}-edited.png`, { type: 'image/png' }), saveMode.value)
  }, 'image/png')
}

async function loadSource() {
  const canvas = canvasRef.value
  if (!props.open || !canvas || !props.src) return
  const version = ++loadVersion
  loading.value = true
  errorMessage.value = ''
  history.value = []
  future.value = []
  cropRect.value = null
  activeTool.value = 'view'
  const image = new Image()
  image.crossOrigin = 'anonymous'
  image.onload = () => {
    if (version !== loadVersion || !props.open) return
    sourceImage = image
    if (maskRef.value) { maskRef.value.width = image.naturalWidth; maskRef.value.height = image.naturalHeight }
    hasMask.value = false
    canvas.width = image.naturalWidth
    canvas.height = image.naturalHeight
    const ctx = canvasContext()
    ctx?.clearRect(0, 0, canvas.width, canvas.height)
    ctx?.drawImage(image, 0, 0)
    syncDimensionInputs()
    loading.value = false
    fitToWindow()
  }
  image.onerror = () => {
    if (version !== loadVersion || !props.open) return
    loading.value = false
    errorMessage.value = '图片加载失败，请检查图片地址或重新上传。'
  }
  image.src = props.src
}

function closeEditor() {
  if (!props.saving) emit('close')
}

function onKeydown(event: KeyboardEvent) {
  if (!props.open) return
  if ((event.target as HTMLElement).closest('input, textarea, [contenteditable]')) return
  if (event.key === 'Escape') closeEditor()
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'z') {
    event.preventDefault()
    event.shiftKey ? redo() : undo()
  }
}

watch(() => [props.open, props.src], () => {
  if (props.open) nextTick(loadSource)
  else { ++loadVersion; history.value = []; future.value = []; draftFrame = null; sourceImage = null; panStart = null }
})

onMounted(() => window.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  ++loadVersion
  sourceImage = null
})
</script>

<template>
  <Teleport to="body">
    <div v-if="open" data-testid="image-editor" class="fixed inset-0 z-[12000] flex flex-col overflow-hidden bg-[#070b11] text-white" @pointerdown.stop>
      <header class="flex min-h-14 shrink-0 flex-wrap items-center gap-2 border-b border-white/10 bg-[#111b28] px-4 shadow-xl">
        <button class="editor-button mr-auto" aria-label="返回查看" @click="closeEditor">
          <IconArrowLeft :size="18" />
          <span>返回查看</span>
        </button>

        <nav class="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto" aria-label="图片编辑工具">
          <button
            v-for="tool in tools"
            :key="tool.id"
            class="editor-tab"
            :class="{ 'editor-tab-active': activeTool === tool.id }"
            :aria-pressed="activeTool === tool.id"
            @click="selectTool(tool.id)"
          >
            <component :is="tool.icon" :size="18" />
            <span>{{ tool.label }}</span>
          </button>
        </nav>

        <div class="ml-auto flex items-center gap-1">
          <button class="icon-button" aria-label="撤销" :disabled="!canUndo" @click="undo"><IconArrowBackUp :size="19" /></button>
          <button class="icon-button" aria-label="恢复" :disabled="!canRedo" @click="redo"><IconArrowForwardUp :size="19" /></button>
          <button class="editor-button" aria-label="清空编辑" :disabled="saving || loading" @click="resetImage">清空</button>
          <select v-model="saveMode" aria-label="保存方式" class="editor-number"><option value="replace">保存并替换</option><option value="copy">保存为新图片</option></select>
          <button data-testid="save-edited-image" class="ml-2 inline-flex h-9 items-center gap-2 rounded-xl bg-cyan-500 px-4 text-sm font-medium text-slate-950 transition hover:bg-cyan-400 disabled:opacity-50" :disabled="saving || loading" @click="exportFile">
            <span v-if="saving" class="loading loading-spinner loading-xs" />
            <IconDeviceFloppy v-else :size="18" />
            {{ saving ? '保存中…' : '保存' }}
          </button>
          <button class="icon-button ml-1" aria-label="关闭编辑器" @click="closeEditor"><IconX :size="20" /></button>
        </div>
      </header>
      <p v-if="saveError" role="alert" class="bg-red-950 px-4 py-2 text-sm text-red-200">{{ saveError }}</p>

      <section v-if="activeTool !== 'view'" class="flex min-h-14 shrink-0 items-center gap-4 overflow-x-auto border-b border-white/10 bg-[#121e2b] px-5 text-xs text-white/60">
        <template v-if="activeTool === 'brush' || activeTool === 'mask'">
          <template v-if="activeTool === 'mask'"><span class="whitespace-nowrap">独立选区，不写入原图</span><button class="editor-action shrink-0" :disabled="!hasMask" @click="exportMask">导出遮罩</button><button class="editor-action shrink-0" :disabled="!hasMask" @click="clearMask">清空遮罩</button></template>
          <div class="flex items-center gap-1 rounded-xl border border-white/10 bg-white/5 p-1">
            <button v-for="shape in shapes" :key="shape.id" class="icon-button" :class="{ '!bg-cyan-400/15 !text-cyan-300': drawShape === shape.id }" :aria-label="shape.label" :title="shape.label" @click="drawShape = shape.id">
              <component :is="shape.icon" :size="18" />
            </button>
          </div>
          <label class="flex items-center gap-2 whitespace-nowrap">大小 <input v-model.number="brushSize" type="range" min="2" max="160" class="accent-cyan-400" /> <span class="w-12 text-white/80">{{ brushSize }}px</span></label>
          <label class="flex items-center gap-2 whitespace-nowrap">边缘 <input v-model.number="feather" type="range" min="0" max="40" class="accent-cyan-400" /> <span class="w-10 text-white/80">{{ feather }}</span></label>
          <label class="flex items-center gap-2 whitespace-nowrap">透明度 <input v-model.number="opacity" type="range" min="0.1" max="1" step="0.05" class="accent-cyan-400" /> <span class="w-10 text-white/80">{{ Math.round(opacity * 100) }}%</span></label>
          <div class="ml-auto flex items-center gap-2 whitespace-nowrap">颜色
            <button v-for="swatch in colors" :key="swatch" class="h-7 w-7 rounded-full border-2 transition" :class="color === swatch ? 'scale-110 border-cyan-300 ring-2 ring-cyan-300/30' : 'border-white/20'" :style="{ backgroundColor: swatch }" :aria-label="`选择颜色 ${swatch}`" @click="color = swatch" />
          </div>
        </template>

        <template v-else-if="activeTool === 'text'">
          <IconTypography :size="18" class="text-cyan-300" />
          <input v-model="textValue" data-testid="editor-text-input" class="h-9 w-72 rounded-lg border border-white/10 bg-white/5 px-3 text-sm text-white outline-none focus:border-cyan-400/60" placeholder="输入文字，然后点击图片放置" />
          <label class="flex items-center gap-2">字号 <input v-model.number="fontSize" type="number" min="10" max="240" class="editor-number w-20" /> px</label>
          <span class="text-white/35">输入完成后点击图片放置文字</span>
          <div class="ml-auto flex items-center gap-2">颜色
            <button v-for="swatch in colors" :key="swatch" class="h-7 w-7 rounded-full border-2" :class="color === swatch ? 'border-cyan-300 ring-2 ring-cyan-300/30' : 'border-white/20'" :style="{ backgroundColor: swatch }" @click="color = swatch" />
          </div>
        </template>

        <template v-else-if="activeTool === 'crop'">
          <IconCrop :size="18" class="text-cyan-300" />
          <span>在图片上拖出裁剪区域</span>
          <span v-if="cropRect" class="rounded-lg bg-white/5 px-3 py-1.5 text-white/80">{{ Math.round(cropRect.width) }} × {{ Math.round(cropRect.height) }}</span>
          <button class="editor-action" :disabled="!cropRect || cropRect.width < 2 || cropRect.height < 2" @click="applyCrop"><IconCheck :size="16" />应用裁剪</button>
        </template>

        <template v-else-if="activeTool === 'expand'">
          <IconArrowsMaximize :size="18" class="text-cyan-300" />
          <span>画布扩展（纯色填充）</span>
          <input v-model.number="expandWidth" type="number" min="1" class="editor-number w-24" aria-label="扩图宽度" />
          <span>×</span>
          <input v-model.number="expandHeight" type="number" min="1" class="editor-number w-24" aria-label="扩图高度" />
          <span class="text-white/35">居中扩展，空白区域使用当前颜色</span>
          <button class="editor-action" @click="applyExpand"><IconCheck :size="16" />应用扩图</button>
        </template>

        <template v-else-if="activeTool === 'resize'">
          <IconDimensions :size="18" class="text-cyan-300" />
          <span>图片尺寸</span>
          <input :value="resizeWidth" type="number" min="1" class="editor-number w-24" aria-label="图片宽度" @input="onResizeWidth(Number(($event.target as HTMLInputElement).value))" />
          <button class="icon-button" :aria-label="keepRatio ? '取消锁定比例' : '锁定比例'" @click="keepRatio = !keepRatio"><IconLock v-if="keepRatio" :size="16" /><IconLockOpen v-else :size="16" /></button>
          <input :value="resizeHeight" type="number" min="1" class="editor-number w-24" aria-label="图片高度" @input="onResizeHeight(Number(($event.target as HTMLInputElement).value))" />
          <span>px</span>
          <button class="editor-action" @click="applyResize"><IconCheck :size="16" />应用尺寸</button>
        </template>

        <template v-else-if="activeTool === 'grid'">
          <IconGridDots :size="18" class="text-cyan-300" />
          <span>宫格拆分</span>
          <select v-model.number="gridCols" class="editor-number" aria-label="宫格列数"><option :value="2">2 列</option><option :value="3">3 列</option><option :value="4">4 列</option></select>
          <span>×</span>
          <select v-model.number="gridRows" class="editor-number" aria-label="宫格行数"><option :value="2">2 行</option><option :value="3">3 行</option><option :value="4">4 行</option></select>
          <span class="text-white/35">拆分后将在画布中创建 {{ gridCols * gridRows }} 个新图片节点</span>
          <button data-testid="split-image-grid" class="editor-action" @click="splitGrid"><IconGridDots :size="16" />开始拆分</button>
        </template>
      </section>

      <main ref="stageRef" class="relative flex min-h-0 flex-1 overflow-auto bg-[#080d14] p-8">
        <div v-if="loading" class="absolute inset-0 z-20 flex items-center justify-center bg-[#080d14]"><span class="loading loading-spinner loading-lg text-cyan-300" /></div>
        <div v-if="errorMessage" class="absolute inset-0 z-20 flex flex-col items-center justify-center gap-3 text-white/55"><IconPhoto :size="44" /><p>{{ errorMessage }}</p></div>
        <div class="relative m-auto shrink-0 shadow-2xl shadow-black/70" :style="{ width: `${canvasWidth * zoom}px`, height: `${canvasHeight * zoom}px` }">
          <canvas
            ref="canvasRef"
            data-testid="image-editor-canvas"
            class="block h-full w-full bg-[linear-gradient(45deg,#18212d_25%,transparent_25%),linear-gradient(-45deg,#18212d_25%,transparent_25%),linear-gradient(45deg,transparent_75%,#18212d_75%),linear-gradient(-45deg,transparent_75%,#18212d_75%)] bg-[length:24px_24px] bg-[position:0_0,0_12px,12px_-12px,-12px_0]"
            :style="{ cursor: stageCursor, touchAction: 'none' }"
            @pointerdown="onPointerDown"
            @pointermove="onPointerMove"
            @pointerup="onPointerUp"
            @pointercancel="onPointerUp"
          />
          <canvas ref="maskRef" data-testid="editor-mask" class="pointer-events-none absolute inset-0 h-full w-full opacity-45" />
          <div v-if="activeTool === 'crop' && cropRect" class="pointer-events-none absolute border-2 border-cyan-300 bg-cyan-300/10 shadow-[0_0_0_9999px_rgba(0,0,0,.45)]" :style="cropOverlayStyle()">
            <span class="absolute -top-7 left-0 rounded bg-black/75 px-2 py-1 text-[10px] text-white/80">{{ Math.round(cropRect.width) }} × {{ Math.round(cropRect.height) }}</span>
          </div>
          <div v-if="activeTool === 'grid'" class="pointer-events-none absolute inset-0 grid" :style="{ gridTemplateColumns: `repeat(${gridCols}, 1fr)`, gridTemplateRows: `repeat(${gridRows}, 1fr)` }">
            <div v-for="cell in gridCols * gridRows" :key="cell" class="border border-cyan-300/70 bg-cyan-300/[0.03]" />
          </div>
        </div>
      </main>

      <footer class="pointer-events-none absolute bottom-5 left-1/2 z-30 -translate-x-1/2">
        <div class="pointer-events-auto flex items-center gap-1 rounded-2xl border border-white/10 bg-[#111b28]/95 p-1.5 shadow-2xl backdrop-blur-xl">
          <button class="editor-button h-9" @click="fitToWindow"><IconMaximize :size="17" />适应窗口</button>
          <button class="icon-button" aria-label="缩小" @click="changeZoom(-0.1)"><IconZoomOut :size="18" /></button>
          <span class="w-14 text-center text-xs text-white/70">{{ zoomLabel }}</span>
          <button class="icon-button" aria-label="放大" @click="changeZoom(0.1)"><IconZoomIn :size="18" /></button>
          <button class="editor-button h-9" @click="resetZoom">100%</button>
          <span class="mx-1 h-5 w-px bg-white/10" />
          <span class="px-2 text-[11px] text-white/35">{{ canvasWidth }} × {{ canvasHeight }}</span>
        </div>
      </footer>
    </div>
  </Teleport>
</template>

<style scoped>
.editor-button {
  display: inline-flex;
  height: 2.25rem;
  align-items: center;
  gap: .45rem;
  border-radius: .65rem;
  padding: 0 .65rem;
  color: rgb(255 255 255 / .68);
  font-size: .8rem;
  transition: color .16s ease, background-color .16s ease;
}
.editor-button:hover { color: white; background: rgb(255 255 255 / .07); }
.editor-tab {
  display: inline-flex;
  height: 2.45rem;
  align-items: center;
  gap: .4rem;
  border: 1px solid transparent;
  border-radius: .7rem;
  padding: 0 .7rem;
  color: rgb(255 255 255 / .65);
  font-size: .8rem;
  white-space: nowrap;
  transition: all .16s ease;
}
.editor-tab:hover { color: white; background: rgb(255 255 255 / .06); }
.editor-tab-active { color: rgb(103 232 249); border-color: rgb(34 211 238 / .55); background: rgb(34 211 238 / .1); }
.icon-button {
  display: inline-flex;
  width: 2.25rem;
  height: 2.25rem;
  align-items: center;
  justify-content: center;
  border-radius: .65rem;
  color: rgb(255 255 255 / .6);
  transition: all .16s ease;
}
.icon-button:hover:not(:disabled) { color: white; background: rgb(255 255 255 / .08); }
.icon-button:disabled { opacity: .25; cursor: not-allowed; }
.editor-number {
  height: 2rem;
  border: 1px solid rgb(255 255 255 / .1);
  border-radius: .5rem;
  background: rgb(255 255 255 / .05);
  padding: 0 .55rem;
  color: rgb(255 255 255 / .82);
  outline: none;
}
.editor-number:focus { border-color: rgb(34 211 238 / .55); }
.editor-action {
  display: inline-flex;
  height: 2rem;
  align-items: center;
  gap: .35rem;
  border-radius: .55rem;
  background: rgb(34 211 238 / .14);
  padding: 0 .7rem;
  color: rgb(103 232 249);
  transition: background-color .16s ease;
}
.editor-action:hover:not(:disabled) { background: rgb(34 211 238 / .22); }
.editor-action:disabled { opacity: .3; cursor: not-allowed; }
</style>
