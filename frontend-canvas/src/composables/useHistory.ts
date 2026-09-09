import { shallowRef, computed, type Ref } from 'vue'
import type { Node, Edge } from '../types'
import { shareRecords } from '../utils/canvasPerformance'

interface Snapshot {
  nodes: Node[]
  edges: Edge[]
}

export function useHistory(nodes: Ref<Node[]>, edges: Ref<Edge[]>) {
  const undoStack = shallowRef<Snapshot[]>([])
  const redoStack = shallowRef<Snapshot[]>([])

  const canUndo = computed(() => undoStack.value.length > 0)
  const canRedo = computed(() => redoStack.value.length > 0)

  let previous: Snapshot = { nodes: [], edges: [] }
  function clone(n: Node[], e: Edge[]): Snapshot {
    previous = { nodes: shareRecords(n, previous.nodes), edges: shareRecords(e, previous.edges) }
    return previous
  }

  function push() {
    undoStack.value = [...undoStack.value.slice(-49), clone(nodes.value, edges.value)]
    redoStack.value = []
  }

  function undo() {
    const snapshot = undoStack.value[undoStack.value.length - 1]
    undoStack.value = undoStack.value.slice(0, -1)
    if (!snapshot) return
    redoStack.value = [...redoStack.value.slice(-49), clone(nodes.value, edges.value)]
    nodes.value = snapshot.nodes.map(node => ({ ...node }))
    edges.value = snapshot.edges.map(edge => ({ ...edge }))
  }

  function redo() {
    const snapshot = redoStack.value[redoStack.value.length - 1]
    redoStack.value = redoStack.value.slice(0, -1)
    if (!snapshot) return
    undoStack.value.push(clone(nodes.value, edges.value))
    nodes.value = snapshot.nodes.map(node => ({ ...node }))
    edges.value = snapshot.edges.map(edge => ({ ...edge }))
  }

  return { canUndo, canRedo, push, undo, redo }
}
