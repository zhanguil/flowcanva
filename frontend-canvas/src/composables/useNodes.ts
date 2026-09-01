import { ref, computed, reactive } from 'vue'
import type { Node, Edge } from '../types'
import { fetchCanvas, createCanvas, updateCanvas, createNode, updateNode, deleteNode } from '../api'

export function useNodes() {
  const nodes = ref<Node[]>([])
  const selectedNodeId = ref<string | null>(null)
  const selectedNodeIds = ref<string[]>([])
  const canvasId = ref<string | null>(null)
  const canvasName = ref('')
  const nodeZIndices = reactive<Record<string, number>>({})
  let zCounter = 0

  const selectedNode = computed(() =>
    nodes.value.find(n => n.id === selectedNodeId.value) ?? null
  )

  async function loadOrCreateCanvas(preferredId?: string) {
    const res = await fetch('/api/canvases?page_size=100')
    const data = await res.json()
    const canvases = data.items || data

    if (preferredId && canvases.some((c: any) => c.id === preferredId)) {
      canvasId.value = preferredId
    } else if (canvases.length > 0) {
      canvasId.value = canvases[0].id
    } else {
      const cv = await createCanvas('未命名画布')
      canvasId.value = cv.id
    }

    const cv = await fetchCanvas(canvasId.value!)
    canvasName.value = cv.name
    nodes.value = cv.nodes ?? []
    for (const n of nodes.value) {
      if (!(n.id in nodeZIndices)) nodeZIndices[n.id] = ++zCounter
    }
    return cv.edges ?? []
  }

  async function renameCanvas(name: string) {
    if (!canvasId.value) return
    await updateCanvas(canvasId.value, name)
    canvasName.value = name
  }

  async function addNode(
    type: Node['node_type'],
    wx: number,
    wy: number,
    content = '',
  ) {
    if (!canvasId.value) return
    const defaults: Record<string, { w: number; h: number; color: string }> = {
      text: { w: 480, h: 270, color: '#FEF3C7' },
      image: { w: 400, h: 88, color: '#DBEAFE' },
      video: { w: 480, h: 320, color: '#EDE9FE' },
      table: { w: 960, h: 540, color: '#D1FAE5' },
      full_image: { w: 420, h: 280, color: '#FEE2E2' },
      agent: { w: 480, h: 270, color: '#E0E7FF' },
      workflow: { w: 360, h: 260, color: '#FCE7F3' },
      asset: { w: 320, h: 300, color: '#FEF9E7' },
      director: { w: 480, h: 340, color: '#E8F5E9' },
    }
    const d = defaults[type]
    const n = await createNode(canvasId.value, {
      node_type: type,
      x: wx - d.w / 2,
      y: wy - d.h / 2,
      width: d.w,
      height: d.h,
      content,
      config: '{}',
    })
    nodes.value.push(n)
    nodeZIndices[n.id] = ++zCounter
    selectedNodeId.value = n.id
    selectedNodeIds.value = [n.id]
    return n
  }

  async function moveNode(id: string, x: number, y: number) {
    const node = nodes.value.find(n => n.id === id)
    if (node) {
      node.x = x
      node.y = y
    }
    if (!canvasId.value) return
    await updateNode(canvasId.value, id, { x, y })
  }

  async function resizeNode(id: string, width: number, height: number, x: number, y: number) {
    const node = nodes.value.find(n => n.id === id)
    if (node) {
      node.width = width
      node.height = height
      node.x = x
      node.y = y
    }
    if (!canvasId.value) return
    await updateNode(canvasId.value, id, { width, height, x, y })
  }

  async function updateNodeContent(id: string, content: string) {
    const node = nodes.value.find(n => n.id === id)
    if (node) {
      node.content = content
    }
    if (!canvasId.value) return
    try { await updateNode(canvasId.value, id, { content }) } catch { /* ignore save errors during stream */ }
  }

  async function removeNode(id: string) {
    if (!canvasId.value) return
    await deleteNode(canvasId.value, id)
    nodes.value = nodes.value.filter(n => n.id !== id)
    if (selectedNodeId.value === id) {
      selectedNodeId.value = null
    }
    selectedNodeIds.value = selectedNodeIds.value.filter(selectedId => selectedId !== id)
    if (!selectedNodeId.value && selectedNodeIds.value.length > 0) {
      selectedNodeId.value = selectedNodeIds.value[selectedNodeIds.value.length - 1]
    }
  }

  function selectNode(id: string | null, additive = false) {
    if (!id) {
      selectedNodeId.value = null
      selectedNodeIds.value = []
      return
    }
    if (!additive) {
      selectedNodeId.value = id
      selectedNodeIds.value = [id]
    } else if (selectedNodeIds.value.includes(id)) {
      selectedNodeIds.value = selectedNodeIds.value.filter(selectedId => selectedId !== id)
      selectedNodeId.value = selectedNodeIds.value[selectedNodeIds.value.length - 1] ?? null
    } else {
      selectedNodeIds.value = [...selectedNodeIds.value, id]
      selectedNodeId.value = id
    }
    if (selectedNodeId.value) nodeZIndices[selectedNodeId.value] = ++zCounter
  }

  return {
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
  }
}
