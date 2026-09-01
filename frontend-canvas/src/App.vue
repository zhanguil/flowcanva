<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import InfiniteCanvas from './components/InfiniteCanvas.vue'
import LeftToolbar from './components/LeftToolbar.vue'
import BottomToolbar from './components/BottomToolbar.vue'
import AssetManager from './components/AssetManager.vue'
import PresetManager from './components/PresetManager.vue'
import RightDock, { type RightDockTab } from './components/RightDock.vue'
import { useCanvas } from './composables/useCanvas'
import { useNodes } from './composables/useNodes'
import { useEdges } from './composables/useEdges'
import { useHistory } from './composables/useHistory'
import { useAssets } from './composables/useAssets'
import { useNodeConfigs } from './composables/useNodeConfigs'
import type { Node as CanvasNode } from './types'
import type { Asset } from './types'
import { isSupportedCanvasImage, staggerCanvasPoint } from './utils/canvasCoordinates'

const consoleURL = import.meta.env.DEV ? '/' : '/'
const isDev = import.meta.env.DEV

const NODE_LABELS: Record<string, string> = {
  text: '文本',
  image: '生图',
  video: '视频',
  table: '剧本',
  full_image: '全图',
  agent: '智能体',
  workflow: '工作流',
  asset: '图片',
  director: '导演台',
}

const {
  viewport,
  worldStyle,
  gridStyle,
  screenToCanvasPoint,
  onWheel,
  onPointerDown,
  onPointerMove,
  onPointerUp,
  resetView,
  zoomIn,
  zoomOut,
  navigateTo,
} = useCanvas()

const {
  nodes,
  selectedNodeId,
  selectedNodeIds,
  selectedNode,
  canvasId,
  canvasName,
  nodeZIndices,
  loadOrCreateCanvas,
  renameCanvas,
  addNode,
  moveNode,
  resizeNode,
  updateNodeContent,
  removeNode,
  selectNode,
} = useNodes()

const {
  edges,
  canvasId: edgesCanvasId,
  addEdge,
  removeEdge,
  removeNodeEdges,
} = useEdges()

const { canUndo, canRedo, push: pushHistory, undo, redo } = useHistory(nodes, edges)

const { assets, loadAssets, addAsset, setCategory: setAssetCategory, removeAsset } = useAssets()
// 进入画布即预加载资产,避免资产节点 picker 首次打开显示"暂无资产"(之前只有打开资产库面板才触发加载)
loadAssets()

const showAssetManager = ref(false)
const showPresetManager = ref(false)
const dockTab = ref<RightDockTab | null>('assistant')

interface AssistantSelectedImage {
  nodeId: string
  assetId: string
  url: string
  name: string
  mimeType: string
  width: number
  height: number
}

const selectedAssistantImages = computed<AssistantSelectedImage[]>(() => {
  const result: AssistantSelectedImage[] = []
  const seen = new Set<string>()
  for (const nodeId of selectedNodeIds.value) {
    const node = nodes.value.find(item => item.id === nodeId)
    if (!node || node.node_type !== 'asset') continue
    try {
      const data = JSON.parse(node.content || '{}')
      const candidates = [{ url: data.url, name: data.name }]
      for (const candidate of candidates) {
        const url = candidate?.url
        if (!url || seen.has(url)) continue
        seen.add(url)
        result.push({
          nodeId,
          assetId: data.asset_id || '',
          url,
          name: candidate.name || `图${result.length + 1}`,
          mimeType: data.mime_type || '',
          width: data.width || 0,
          height: data.height || 0,
        })
        if (result.length >= 8) return result
      }
    } catch { /* ignore nodes without valid image content */ }
  }
  return result
})

const { loadNodeConfigs } = useNodeConfigs()

const availableNodeTypes = ref<{ type: string; label: string }[]>(
  Object.entries(NODE_LABELS).map(([type, label]) => ({ type, label }))
)

async function loadConfigs() {
  const configs = await loadNodeConfigs()
  if (configs) {
    const enabled = new Set(configs.filter(c => c.enabled).map(c => c.node_type))
    const alwaysOn = new Set(['asset', 'director'])
    availableNodeTypes.value = Object.entries(NODE_LABELS)
      .filter(([type]) => enabled.has(type) || alwaysOn.has(type))
      .map(([type, label]) => ({ type, label }))
  }
}

const isEditingName = ref(false)
const editNameValue = ref('')

function startEditName() {
  editNameValue.value = canvasName.value
  isEditingName.value = true
}

function confirmEditName() {
  const name = editNameValue.value.trim()
  if (name && name !== canvasName.value) {
    renameCanvas(name)
  }
  isEditingName.value = false
}

function cancelEditName() {
  isEditingName.value = false
}

function openAssetManager() {
  dockTab.value = null
  showAssetManager.value = true
}

function openPresetManager() {
  dockTab.value = null
  showPresetManager.value = true
}

// Keyboard shortcuts
function onKeydown(e: KeyboardEvent) {
  // Alt+Shift+F 整理画布
  if (e.altKey && e.shiftKey && e.key === 'F') {
    e.preventDefault()
    autoArrange()
    return
  }
  // Ctrl+Z 撤销
  if ((e.ctrlKey || e.metaKey) && e.key === 'z' && !e.shiftKey) {
    e.preventDefault()
    undo()
    return
  }
  // Ctrl+Y 或 Ctrl+Shift+Z 重做
  if ((e.ctrlKey || e.metaKey) && (e.key === 'y' || (e.key === 'z' && e.shiftKey))) {
    e.preventDefault()
    redo()
    return
  }
  if (e.key === 'Delete' || e.key === 'Backspace') {
    if (selectedNodeId.value && document.activeElement === document.body) {
      handleDeleteNode(selectedNodeId.value)
    }
  }
  if (e.key === 'Escape') {
    selectNode(null)
  }
}

// Ctrl+V 粘贴截图
async function onPaste(e: ClipboardEvent) {
  const items = e.clipboardData?.items
  if (!items) return
  for (const item of items) {
    if (item.type.startsWith('image/')) {
      const file = item.getAsFile()
      if (!file) continue
      const a = await addAsset(file)
      if (!a) continue
      const cx = window.innerWidth / 2
      const cy = window.innerHeight / 2
      const { wx, wy } = screenToCanvasPoint(cx, cy)
      pushHistory()
      await createAssetImageNode(a, wx, wy)
      break
    }
  }
}

function assetNodeContent(asset: Asset, parentGenerationNodeId = '') {
  return JSON.stringify({
    asset_id: asset.id,
    url: asset.url,
    name: asset.filename,
    size: asset.size,
    mime_type: asset.mime_type,
    width: asset.width,
    height: asset.height,
    ...(parentGenerationNodeId ? { parent_generation_node_id: parentGenerationNodeId } : {}),
  })
}

async function createAssetImageNode(asset: Asset, centerX: number, centerY: number, parentGenerationNodeId = '') {
  return addNode('asset', centerX, centerY, assetNodeContent(asset, parentGenerationNodeId))
}

function hasSupportedDraggedImage(event: DragEvent) {
  return Array.from(event.dataTransfer?.items || []).some(item => item.kind === 'file')
}

function handleCanvasDragOver(event: DragEvent) {
  if ((event.target as HTMLElement).closest('[data-testid="right-dock"]')) return
  if (!hasSupportedDraggedImage(event)) return
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
}

async function handleCanvasFileDrop(event: DragEvent) {
  if ((event.target as HTMLElement).closest('[data-testid="right-dock"]')) return
  const files = Array.from(event.dataTransfer?.files || []).filter(isSupportedCanvasImage)
  if (files.length === 0) return
  event.preventDefault()
  event.stopPropagation()
  const origin = screenToCanvasPoint(event.clientX, event.clientY)
  pushHistory()
  for (let index = 0; index < files.length; index++) {
    const asset = await addAsset(files[index])
    if (!asset) continue
    const point = staggerCanvasPoint(origin, index)
    await createAssetImageNode(asset, point.wx, point.wy)
  }
}

// 底部工具栏状态
const snapToGrid = ref(false)

function autoArrange() {
  if (nodes.value.length === 0) return
  pushHistory()
  const gapX = 100, gapY = 40, startX = 50, startY = 50

  const incoming = new Map<string, Set<string>>()
  const outgoing = new Map<string, Set<string>>()
  for (const e of edges.value) {
    if (!outgoing.has(e.source_node_id)) outgoing.set(e.source_node_id, new Set())
    outgoing.get(e.source_node_id)!.add(e.target_node_id)
    if (!incoming.has(e.target_node_id)) incoming.set(e.target_node_id, new Set())
    incoming.get(e.target_node_id)!.add(e.source_node_id)
  }

  // 反向BFS：从末节点开始
  const rdepth = new Map<string, number>()
  const rqueue: string[] = []
  for (const n of nodes.value) {
    if (!outgoing.has(n.id) || outgoing.get(n.id)!.size === 0) {
      rdepth.set(n.id, 0)
      rqueue.push(n.id)
    }
  }
  while (rqueue.length > 0) {
    const id = rqueue.shift()!
    const d = rdepth.get(id)!
    for (const src of incoming.get(id) ?? []) {
      if (!rdepth.has(src) || rdepth.get(src)! < d + 1) {
        rdepth.set(src, d + 1)
        if (!rqueue.includes(src)) rqueue.push(src)
      }
    }
  }

  // 深度映射：root 最小 → 列0(最左)
  const maxDepth = Math.max(...rdepth.values(), 0)
  const depth = new Map<string, number>()
  for (const n of nodes.value) depth.set(n.id, maxDepth - (rdepth.get(n.id) ?? 0))

  const disconnected = nodes.value.filter(n => !incoming.has(n.id) && !outgoing.has(n.id))

  const levels = new Map<number, CanvasNode[]>()
  for (const n of nodes.value) {
    if (disconnected.includes(n)) continue
    const d = depth.get(n.id) ?? 0
    if (!levels.has(d)) levels.set(d, [])
    levels.get(d)!.push(n)
  }

  // Sugiyama 交叉最小化：质心排序
  const sortedLevels = [...levels.entries()].sort((a, b) => a[0] - b[0]).map(([, ns]) => [...ns])
  
  for (let pass = 0; pass < 4; pass++) {
    // 正向：每层按上层关联节点的平均位置排序
    for (let li = 1; li < sortedLevels.length; li++) {
      const prev = sortedLevels[li - 1]
      const cur = sortedLevels[li]
      const prevIdx = new Map(prev.map((n, i) => [n.id, i]))
      cur.sort((a, b) => {
        const inA = [...(incoming.get(a.id) ?? [])].filter(x => prevIdx.has(x))
        const inB = [...(incoming.get(b.id) ?? [])].filter(x => prevIdx.has(x))
        const avgA = inA.length ? inA.reduce((s, x) => s + prevIdx.get(x)!, 0) / inA.length : 9999
        const avgB = inB.length ? inB.reduce((s, x) => s + prevIdx.get(x)!, 0) / inB.length : 9999
        return avgA - avgB
      })
    }
    // 反向：每层按下层关联节点的平均位置排序
    for (let li = sortedLevels.length - 2; li >= 0; li--) {
      const next = sortedLevels[li + 1]
      const cur = sortedLevels[li]
      const nextIdx = new Map(next.map((n, i) => [n.id, i]))
      cur.sort((a, b) => {
        const outA = [...(outgoing.get(a.id) ?? [])].filter(x => nextIdx.has(x))
        const outB = [...(outgoing.get(b.id) ?? [])].filter(x => nextIdx.has(x))
        const avgA = outA.length ? outA.reduce((s, x) => s + nextIdx.get(x)!, 0) / outA.length : 9999
        const avgB = outB.length ? outB.reduce((s, x) => s + nextIdx.get(x)!, 0) / outB.length : 9999
        return avgA - avgB
      })
    }
  }

  let cx = startX
  for (const levelNodes of sortedLevels) {
    const colW = Math.max(...levelNodes.map(n => n.width))
    let cy = startY
    levelNodes.forEach(n => {
      resizeNode(n.id, n.width, n.height, cx, cy)
      cy += n.height + gapY
    })
    cx += colW + gapX
  }

  for (const n of disconnected) {
    resizeNode(n.id, n.width, n.height, cx, startY)
    cx += n.width + gapX
  }

  // 孤立节点每人一列
  for (const n of disconnected) {
    resizeNode(n.id, n.width, n.height, cx, startY)
    cx += n.width + gapX
  }
}

function getCanvasIdFromURL(): string | undefined {
  const hash = window.location.hash
  const match = hash.match(/canvas=([^#&]+)/)
  return match ? decodeURIComponent(match[1]) : undefined
}

onMounted(async () => {
  const cid = getCanvasIdFromURL()
  const loadedEdges = await loadOrCreateCanvas(cid)
  if (canvasId.value) {
    edgesCanvasId.value = canvasId.value
    if (loadedEdges) {
      edges.value = loadedEdges
    }
  }
  await loadConfigs()
  document.addEventListener('keydown', onKeydown)
  document.addEventListener('paste', onPaste as any)
  window.addEventListener('focus', loadConfigs)
})

onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown)
  document.removeEventListener('paste', onPaste as any)
  window.removeEventListener('focus', loadConfigs)
})

function handleAddNode(type: string) {
  const cx = window.innerWidth / 2
  const cy = window.innerHeight / 2
  const { wx, wy } = screenToCanvasPoint(cx, cy)
  pushHistory()
  addNode(type as any, wx, wy)
}

function handleCanvasSelect(id: string, event?: PointerEvent) {
  selectNode(id, Boolean(event && (event.shiftKey || event.ctrlKey || event.metaKey)))
}

async function handleAddConnectedNode(type: string, wx: number, wy: number, sourceNodeId: string) {
  pushHistory()
  const node = await addNode(type as any, wx, wy)
  if (node) {
    addEdge(sourceNodeId, node.id)
  }
}

async function handleCreateAssetFromScreenshot(imageUrl: string, name: string) {
  const sid = selectedNodeId.value
  if (!sid) return
  const src = nodes.value.find(n => n.id === sid)
  if (!src) return
  const cx = src.x + src.width / 2 + src.width * 0.8
  const cy = src.y + src.height / 2
  pushHistory()
  const node = await addNode('asset' as any, cx, cy)
  if (node) {
    updateNodeContent(node.id, JSON.stringify({ url: imageUrl, name, size: 0 }))
    addEdge(sid, node.id)
  }
}

interface GeneratedAssetResult {
  id: string
  filename: string
  url: string
  size: number
  mime_type: string
  width: number
  height: number
}

async function handleGeneratedAssets(payload: { sourceNodeId: string; assets: GeneratedAssetResult[] }) {
  const src = nodes.value.find(n => n.id === payload.sourceNodeId)
  if (!src || payload.assets.length === 0) return

  pushHistory()
  const nodeWidth = 320
  const nodeHeight = 300
  const gap = 40
  const columns = payload.assets.length > 1 ? 2 : 1
  // 图片加载会把生图节点自适应放大；按其最大显示宽度预留空间，避免结果节点重叠。
  const startLeft = src.x + Math.max(src.width, 660) + 60
  const startTop = src.y

  for (let i = 0; i < payload.assets.length; i++) {
    const asset = payload.assets[i]
    const col = i % columns
    const row = Math.floor(i / columns)
    const centerX = startLeft + col * (nodeWidth + gap) + nodeWidth / 2
    const centerY = startTop + row * (nodeHeight + gap) + nodeHeight / 2
    const node = await createAssetImageNode(asset as Asset, centerX, centerY, payload.sourceNodeId)
    if (!node) continue
    await addEdge(payload.sourceNodeId, node.id)
    selectNode(payload.sourceNodeId)
  }
  await loadAssets()
}

async function handleReferenceAssetsUploaded(payload: { targetNodeId: string; assets: Asset[] }) {
  const target = nodes.value.find(node => node.id === payload.targetNodeId)
  if (!target || payload.assets.length === 0) return
  pushHistory()
  for (let index = 0; index < payload.assets.length; index++) {
    const centerX = target.x - 360 - (index % 2) * 340
    const centerY = target.y + 150 + Math.floor(index / 2) * 320
    const referenceNode = await createAssetImageNode(payload.assets[index], centerX, centerY)
    if (referenceNode) await addEdge(referenceNode.id, target.id)
  }
}

async function handleApplyAssistantPrompt(payload: { prompt: string; referenceNodeIds: string[] }) {
  const cleanPrompt = payload.prompt.trim()
  if (!cleanPrompt) return
  const source = selectedNode.value
  const referenceNodeIds = [...new Set(payload.referenceNodeIds)]
  let centerX: number
  let centerY: number
  if (source) {
    centerX = source.x + source.width + 80 + 200
    centerY = source.y + 150
  } else {
    const world = screenToCanvasPoint(Math.max(220, (window.innerWidth - 390) / 2), window.innerHeight / 2)
    centerX = world.wx
    centerY = world.wy
  }
  pushHistory()
  const node = await addNode('image', centerX, centerY)
  if (!node) return
  await updateNodeContent(node.id, JSON.stringify({
    prompt: cleanPrompt,
    promptHtml: '',
    images: [],
    generated_images: [],
  }))
  for (const sourceNodeId of referenceNodeIds) {
    await addEdge(sourceNodeId, node.id)
  }
}

async function handleLoadTestCanvas() {
  const response = await fetch('/api/dev/test-canvas', { method: 'POST' })
  if (!response.ok) return
  const data = await response.json()
  if (data.canvas_id) {
    window.location.hash = `canvas=${encodeURIComponent(data.canvas_id)}`
    window.location.reload()
  }
}

// 宫格分镜: 切图→生成资产节点网格排列
async function handleGridSplit(data: { cols: number; rows: number; urls: string[] }) {
  const sid = selectedNodeId.value
  if (!sid) return
  const src = nodes.value.find(n => n.id === sid)
  if (!src) return
  pushHistory()
  const gap = 20
  const cellW = 180
  const cellH = 120
  const startX = src.x + src.width + gap * 2
  const startY = src.y
  for (let r = 0; r < data.rows; r++) {
    for (let c = 0; c < data.cols; c++) {
      const idx = r * data.cols + c
      const node = await addNode('asset' as any, startX + c * (cellW + gap), startY + r * (cellH + gap))
      if (node) {
        updateNodeContent(node.id, JSON.stringify({ url: data.urls[idx], name: `分镜${r+1}-${c+1}`, size: 0 }))
        addEdge(sid, node.id)
      }
      // 调节点宽高
      if (node) {
        await new Promise(resolve => setTimeout(resolve, 50))
        moveNode(node.id, startX + c * (cellW + gap), startY + r * (cellH + gap))
        resizeNode(node.id, cellW, cellH, startX + c * (cellW + gap), startY + r * (cellH + gap))
      }
    }
  }
}

function handleLayerSelect(id: string) {
  selectNode(id)
  const n = nodes.value.find(x => x.id === id)
  if (n) navigateTo(n.x + n.width / 2, n.y + n.height / 2)
}

function handleDeleteNode(nodeId: string) {
  pushHistory()
  removeNode(nodeId)
  removeNodeEdges(nodeId)
}

function handleMoveNode(id: string, x: number, y: number) {
  pushHistory()
  moveNode(id, x, y)
}

function handleResizeNode(id: string, w: number, h: number, x: number, y: number) {
  pushHistory()
  resizeNode(id, w, h, x, y)
}

function handleImageLoaded(id: string, w: number, h: number) {
  const nodeRef = nodes.value.find(n => n.id === id)
  if (!nodeRef) return
  if (!isNaN(w) && !isNaN(h) && (nodeRef.width !== w || nodeRef.height !== h)) {
    pushHistory()
    resizeNode(id, w, h, nodeRef.x, nodeRef.y)
  }
}

function handleCreateEdge(sourceNodeId: string, targetNodeId: string) {
  pushHistory()
  addEdge(sourceNodeId, targetNodeId)
}

function handleDeleteEdge(edgeId: string) {
  pushHistory()
  removeEdge(edgeId)
}

function handleRemoveConnectedEdge(edgeId: string) {
  pushHistory()
  removeEdge(edgeId)
}

function handleSaveNodeContent(nodeId: string, content: string) {
  updateNodeContent(nodeId, content)
}

function handleUpdateAsset(id: string, data: { category?: string; tags?: string }) {
  if (data.category !== undefined) setAssetCategory(id, data.category)
}

function handleRemoveAsset(id: string) {
  removeAsset(id)
}

async function handleSavePanel(content: string) {
  if (!selectedNodeId.value) return

  const node = nodes.value.find(n => n.id === selectedNodeId.value)
  if (!node) return

  const isVideo = node.node_type === 'video'
  const needsSplit = isVideo

  let results: any[] | null = null
  let parsedData: any = null

  if (needsSplit) {
    try {
      parsedData = JSON.parse(content)
      if (!parsedData._loading) {
        if (isVideo && Array.isArray(parsedData.videos) && parsedData.videos.length > 1) {
          results = parsedData.videos
        }
      }
    } catch {}
  }

  pushHistory()

  if (results && results.length > 1) {
    const firstContent = { ...parsedData }
    firstContent.videos = [results[0]]
    updateNodeContent(node.id, JSON.stringify(firstContent))

    const dx = node.width * 0.6
    const dy = node.height * 0.5
    for (let i = 1; i < results.length; i++) {
      const cloneData = { ...parsedData }
      cloneData.videos = [results[i]]

      const savedId: string | null = selectedNodeId.value
      const cx = node.x + node.width / 2 + dx * (i - 1)
      const cy = node.y + node.height / 2 + dy * (i - 1)
      const clone = await addNode(node.node_type as any, cx, cy)
      if (!clone) { selectNode(savedId); continue }

      updateNodeContent(clone.id, JSON.stringify(cloneData))

      for (const edge of edges.value) {
        if (edge.source_node_id === node.id) {
          await addEdge(clone.id, edge.target_node_id)
        }
        if (edge.target_node_id === node.id) {
          await addEdge(edge.source_node_id, clone.id)
        }
      }

      selectNode(savedId)
    }
  } else {
    updateNodeContent(selectedNodeId.value, content)
  }

  if (node.node_type === 'asset' && !content) {
    edges.value = edges.value.filter(e => e.source_node_id !== selectedNodeId.value)
  }
}
</script>

<template>
  <div
    data-testid="canvas-drop-zone"
    class="w-screen h-screen overflow-hidden bg-base-100"
    @wheel="onWheel"
    @pointerdown="onPointerDown"
    @pointermove="onPointerMove"
    @pointerup="onPointerUp"
    @dragover="handleCanvasDragOver"
    @drop="handleCanvasFileDrop"
  >
    <!-- 顶部栏 -->
    <header class="fixed top-0 left-0 right-0 z-50 h-12 flex items-center justify-between px-4 bg-base-200/80 backdrop-blur border-b border-base-300">
      <div class="flex items-center gap-3">
        <!-- FlowCanva 图标 -->
        <div class="w-7 h-7 rounded-lg bg-primary/15 flex items-center justify-center text-primary shrink-0 ring-1 ring-primary/20">
          <svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.2"><path stroke-linecap="round" stroke-linejoin="round" d="M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zm0 8a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zm12 0a1 1 0 011-1h2a1 1 0 011 1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-6z"/></svg>
        </div>
        <template v-if="isEditingName">
          <input
            ref="nameInput"
            v-model="editNameValue"
            class="input input-sm input-bordered w-44 h-8 text-sm"
            @keydown.enter="confirmEditName"
            @keydown.escape="cancelEditName"
            @blur="confirmEditName"
          />
          <button class="btn btn-ghost btn-xs" @click="confirmEditName">确定</button>
          <button class="btn btn-ghost btn-xs" @click="cancelEditName">取消</button>
        </template>
        <template v-else>
          <span class="text-sm font-bold cursor-pointer hover:text-primary transition-colors" @click="startEditName">{{ canvasName || '未命名画布' }}</span>
          <button class="btn btn-ghost btn-xs btn-square" title="重命名" @click="startEditName">
            <svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 3a2.828 2.828 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z"/></svg>
          </button>
        </template>
      </div>
      <div class="flex items-center gap-2">
        <button
          data-testid="open-assistant"
          class="btn btn-sm gap-1.5 text-xs"
          :class="dockTab === 'assistant' ? 'btn-primary' : 'btn-ghost'"
          @click="dockTab = dockTab === 'assistant' ? null : 'assistant'"
        >
          <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 3l1.8 4.7L18.5 9.5l-4.7 1.8L12 16l-1.8-4.7-4.7-1.8 4.7-1.8L12 3z"/><path d="M19 15l.8 2.2L22 18l-2.2.8L19 21l-.8-2.2L16 18l2.2-.8L19 15z"/></svg>
          AI Assistant
        </button>
        <button
          data-testid="open-project"
          class="btn btn-sm gap-1.5 text-xs"
          :class="dockTab === 'project' ? 'btn-primary' : 'btn-ghost'"
          @click="dockTab = dockTab === 'project' ? null : 'project'"
        >项目</button>
        <a
          :href="consoleURL"
          class="btn btn-ghost btn-sm gap-1.5 text-xs font-semibold"
        >
          <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><polyline points="9 22 9 12 15 12 15 22"/></svg>
          返回首页
        </a>
      </div>
    </header>

    <InfiniteCanvas
      :viewport="viewport"
      :world-style="worldStyle"
      :grid-style="gridStyle"
      :nodes="nodes"
      :edges="edges"
      :selected-node-id="selectedNodeId"
      :selected-node-ids="selectedNodeIds"
      :selected-node="selectedNode"
      :node-z-indices="nodeZIndices"
      :assets="assets"
      :snap-to-grid="snapToGrid"
      @select="handleCanvasSelect"
      @move-node="handleMoveNode"
      @resize-node="handleResizeNode"
      @image-loaded="handleImageLoaded"
      @create-edge="handleCreateEdge"
      @delete-edge="handleDeleteEdge"
      @remove-connected-edge="handleRemoveConnectedEdge"
      @save-panel="handleSavePanel"
      @save-node-content="handleSaveNodeContent"
      @add-connected-node="handleAddConnectedNode"
      @update-asset="handleUpdateAsset"
      @remove-asset="handleRemoveAsset"
      @create-asset-from-screenshot="handleCreateAssetFromScreenshot"
      @image-generated="handleGeneratedAssets"
      @references-uploaded="handleReferenceAssetsUploaded"
      @grid-split="handleGridSplit"
    />

    <LeftToolbar
      :can-undo="canUndo"
      :can-redo="canRedo"
      :node-types="availableNodeTypes"
      @add-node="handleAddNode"
      @undo="undo"
      @redo="redo"
      @open-asset-manager="openAssetManager"
      @open-preset-manager="openPresetManager"
    />

    <BottomToolbar
      :zoom="viewport.zoom"
      :snap-to-grid="snapToGrid"
      @zoom-in="zoomIn"
      @zoom-out="zoomOut"
      @reset-view="resetView"
      @auto-arrange="autoArrange"
      @toggle-snap="snapToGrid = !snapToGrid"
    />

    <AssetManager v-if="showAssetManager" @close="showAssetManager = false" />
    <PresetManager v-if="showPresetManager" :canvas-id="canvasId ?? ''" @close="showPresetManager = false" />
    <RightDock
      :active-tab="dockTab"
      :canvas-id="canvasId ?? ''"
      :canvas-name="canvasName"
      :nodes="nodes"
      :edge-count="edges.length"
      :asset-count="assets.length"
      :selected-node-id="selectedNodeId"
      :selected-images="selectedAssistantImages"
      :dev-mode="isDev"
      @update:active-tab="dockTab = $event"
      @select-node="handleLayerSelect"
      @apply-prompt="handleApplyAssistantPrompt"
      @open-assets="openAssetManager"
      @load-test-canvas="handleLoadTestCanvas"
    />
  </div>
</template>
