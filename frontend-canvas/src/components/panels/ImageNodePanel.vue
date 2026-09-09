<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import type { Node } from '../../types'
import MentionDropdown from '../MentionDropdown.vue'
import type { ResolvedNodeInput } from '../../utils/nodeInputResolver'
import { useAssets } from '../../composables/useAssets'
import { useImageGenerationTasks } from '../../composables/useImageGenerationTasks'
import { createGenerationTaskId } from '../../utils/generationTaskId'
import ReferenceImageCards from '../ReferenceImageCards.vue'
import { imageRatioOptions } from '../../utils/imageOptions'
import { useImageReferences } from '../../composables/useImageReferences'
import { useGenerationProduct } from '../../composables/useGenerationProduct'
import ProductGenerationBinding from '../ProductGenerationBinding.vue'
import { buildGenerationContext } from '../../utils/generationContext'
import type { GenerationType, ReferenceRole } from '../../types/product'

const props = defineProps<{
  node: Node | null
  panelStyle: Record<string, string>
  nodeInputs?: ResolvedNodeInput[]
}>()

const emit = defineEmits<{
  (e: 'save', payload: { nodeId: string; content: string }): void
  (e: 'remove-connected-edge', edgeId: string): void
  (e: 'generated', payload: { sourceNodeId: string; assets: GeneratedAsset[] }): void
  (e: 'references-uploaded', payload: { targetNodeId: string; assets: GeneratedAsset[]; onComplete?: (error?: string) => void }): void
}>()

interface GeneratedAsset {
  id: string
  filename: string
  url: string
  size: number
  width: number
  height: number
  mime_type?: string
}

const { addAsset } = useAssets()
const { tasks, startImageGeneration } = useImageGenerationTasks()

const prompt = ref('')
const promptHtml = ref('')
const loading = computed(() => Boolean(props.node?.id && tasks[props.node.id]?.status === 'running'))
const generationError = computed(() => props.node?.id ? tasks[props.node.id]?.error || '' : '')
const modalOpen = ref(false)
const generatedImages = ref<{ id: number | string; asset_id?: string; name?: string; url: string; size?: number; width?: number; height?: number }[]>([])
const productBinding = useGenerationProduct(() => props.node, () => props.nodeInputs || [])
const { images: allDisplayImages, excluded, remove: removeReference, roles, setRole, references } = useImageReferences(() => props.nodeInputs || [], () => props.node, () => productBinding.inherited.value)
const generationType = ref<GenerationType>('custom')
const uploading = ref(false)
const uploadError = ref('')
const previewImg = ref<{ id: string; url: string } | null>(null)

const selectedModel = ref('fast')
const models = [
  { value: 'fast', label: 'Nano Banana 2' },
  { value: 'pro', label: 'Nano Banana Pro' },
  { value: 'edit', label: 'GPT Image 2' },
]
const selectedRatio = ref('1:1')
const selectedResolution = ref('2K')
const selectedCount = ref(1)
const selectedPreset = ref('')

const ratioOptions = imageRatioOptions
const resolutionOptions = ['1K', '2K', '4K']
const countOptions = [1, 2, 4]

const presets = ref<{ id: string; name: string; prompt: string; category: string; scope: string }[]>([])
const presetCategories = ref<string[]>([])

const editableRef = ref<HTMLDivElement | null>(null)
const modalContentEditable = ref<HTMLDivElement | null>(null)
const showMention = ref(false)
const mentionFilter = ref('')
const mentionAnchor = ref<'inline' | 'modal'>('inline')

let promptSaveTimer: ReturnType<typeof setTimeout> | null = null
let syncingNode = false

function parseNodeContent(content: string | undefined): Record<string, any> {
  try {
    const parsed = JSON.parse(content || '{}')
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {}
  } catch {
    return {}
  }
}

function syncGeneratedImages(content: string | undefined) {
  const data = parseNodeContent(content)
  generatedImages.value = Array.isArray(data.generated_images) ? data.generated_images : []
}

// The canvas store mutates a selected node in place. Watch its ID for editor
// initialization and its content separately so async generation results reach
// the panel without resetting an active contenteditable selection.
watch(() => props.node?.id, () => {
  if (promptSaveTimer) {
    clearTimeout(promptSaveTimer)
    promptSaveTimer = null
  }
  const n = props.node
  if (!n) return

  const data = parseNodeContent(n.content)
  syncingNode = true
  prompt.value = data.prompt || (/^\s*\{/.test(n.content || '') ? '' : n.content || '')
  promptHtml.value = data.promptHtml || ''
  syncGeneratedImages(n.content)
  syncingNode = false
  selectedRatio.value = data.aspect_ratio || '1:1'
  selectedResolution.value = data.image_size || '2K'
  generationType.value = data.generation_type || 'custom'
  nextTick(() => {
    if (!editableRef.value) return
    if (promptHtml.value) editableRef.value.innerHTML = promptHtml.value
    else editableRef.value.textContent = prompt.value
  })
  loadPresets()
}, { immediate: true })

watch(() => props.node?.content, content => {
  syncGeneratedImages(content)
}, { immediate: true })

watch(prompt, () => {
  if (promptSaveTimer) clearTimeout(promptSaveTimer)
  const nodeId = props.node?.id
  if (!nodeId || syncingNode) return
  promptSaveTimer = setTimeout(() => {
    const node = props.node
    if (!node || node.id !== nodeId) return
    const existing = parseNodeContent(node.content)
    const outputs = Array.isArray(existing.generated_images)
      ? existing.generated_images
      : generatedImages.value
    emit('save', { nodeId, content: buildContent({ generated_images: outputs }) })
    promptSaveTimer = null
  }, 800)
})

async function loadPresets() {
  if (!props.node?.canvas_id) return
  try {
    const [globalRes, canvasRes] = await Promise.all([
      fetch('/api/admin/presets?scope=global'),
      fetch(`/api/admin/presets?scope=canvas&canvas_id=${props.node.canvas_id}`),
    ])
    const g = await globalRes.json(); const c = await canvasRes.json()
    const all = [...(Array.isArray(c) ? c : []), ...(Array.isArray(g) ? g : [])].filter((p: any) => (p.preset_type || 'image') === 'image')
    presets.value = all
    presetCategories.value = [...new Set(all.map((p: any) => p.category))]
  } catch {}
}

function syncPrompt(anchor?: 'inline' | 'modal') {
  const el = anchor === 'modal' ? modalContentEditable.value : editableRef.value
  if (el) {
    const div = document.createElement('div')
    div.innerHTML = el.innerHTML
    prompt.value = (div.textContent || '').trim()
    promptHtml.value = el.innerHTML
  }
}

function buildContent(extras: Record<string, any> = {}) {
  const outputs = Array.isArray(extras.generated_images) ? extras.generated_images : generatedImages.value
  const referenceAssetIds = [...new Set(allDisplayImages.value.flatMap(image => image.assetId ? [image.assetId] : []))]
  const generatedAssetIds = outputs.flatMap((image: any) => image.asset_id || image.id ? [image.asset_id || image.id] : [])
  return JSON.stringify({
    ...parseNodeContent(props.node?.content),
    aspect_ratio: selectedRatio.value,
    image_size: selectedResolution.value,
    generation_type: generationType.value,
    product_asset_id: productBinding.product.value?.id,
    product_snapshot: productBinding.product.value || undefined,
    product_references: productBinding.inherited.value,
    prompt: prompt.value,
    promptHtml: promptHtml.value,
    input: { reference_asset_ids: referenceAssetIds, excluded_reference_keys: excluded.value, reference_roles: roles.value },
    output: { generated_asset_ids: generatedAssetIds },
    ...extras,
  })
}

function saveSelection() {
  if (props.node) emit('save', { nodeId: props.node.id, content: buildContent({ generated_images: generatedImages.value }) })
}

function removeImage(id: string) {
  removeReference(id)
  saveSelection()
}

function changeRole(id: string, role: ReferenceRole) { setRole(id, role); saveSelection() }
function selectProduct(id: string) {
  if (!props.node) return
  const product = productBinding.getProduct(id)
  emit('save', { nodeId: props.node.id, content: buildContent({
    product_asset_id: product?.id, product_snapshot: product,
    product_references: product ? productBinding.getReferences(product) : [],
    generated_images: generatedImages.value,
  }) })
}

function onAddImage() {
  const targetNodeId = props.node?.id
  if (!targetNodeId || uploading.value) return
  const input = document.createElement('input')
  input.type = 'file'; input.accept = '.png,.jpg,.jpeg,.webp,image/png,image/jpeg,image/webp'; input.multiple = true
  input.onchange = async () => {
    if (!input.files) return
    uploading.value = true
    uploadError.value = ''
    const files = Array.from(input.files)
    if (files.length + allDisplayImages.value.length > 8) {
      uploadError.value = '参考图片最多支持 8 张，请减少选择数量'
      uploading.value = false
      return
    }
    try {
      const uploaded: GeneratedAsset[] = []
      for (const file of files) {
        const asset = await addAsset(file)
        if (asset) uploaded.push(asset)
        else uploadError.value = '部分图片上传失败，请重试'
      }
      if (uploaded.length > 0) await new Promise<void>(resolve => {
        emit('references-uploaded', { targetNodeId, assets: uploaded, onComplete: error => {
          if (error) uploadError.value = error
          resolve()
        } })
      })
    } finally { uploading.value = false }
  }
  input.click()
}

function applyPreset() {
  if (!selectedPreset.value) return
  const p = presets.value.find(pr => pr.id === selectedPreset.value)
  if (p) {
    prompt.value = p.prompt + ', ' + prompt.value
    if (editableRef.value) editableRef.value.textContent = prompt.value
    selectedPreset.value = ''
    syncPrompt()
  }
}

function onInput() {
  syncPrompt()
  handleMention('inline')
}

function onModalInput() {
  syncPrompt('modal')
  handleMention('modal')
}

function handleMention(anchor: 'inline' | 'modal') {
  const sel = window.getSelection()
  if (!sel || !sel.rangeCount) { showMention.value = false; return }
  const range = sel.getRangeAt(0)
  const node = range.startContainer
  if (node.nodeType !== Node.TEXT_NODE) { showMention.value = false; return }
  const text = node.textContent || ''
  const cursorPos = range.startOffset
  const before = text.slice(0, cursorPos)
  const m = before.match(/@([^\s@]*)$/)
  if (m) { mentionFilter.value = m[1]; showMention.value = true; mentionAnchor.value = anchor }
  else { showMention.value = false }
}

function insertMention(img: { id: any; name: string; src: string }) {
  const el = mentionAnchor.value === 'modal' ? modalContentEditable.value : editableRef.value; if (!el) return
  const sel = window.getSelection(); if (!sel?.rangeCount) return
  const range = sel.getRangeAt(0); const node = range.startContainer
  if (node.nodeType !== Node.TEXT_NODE) return
  const text = node.textContent || ''; const pos = range.startOffset
  const m = text.slice(0, pos).match(/@([^\s@]*)$/); if (!m) return
  range.setStart(node, pos - m[0].length); range.setEnd(node, pos); range.deleteContents()
  const chip = document.createElement('span')
  chip.className = 'inline-flex items-center gap-0.5 align-middle'; chip.contentEditable = 'false'
  chip.innerHTML = `<img src="${img.src}" class="inline w-4 h-4 rounded object-cover" /><span class="text-blue-400">@${img.name}</span>`
  chip.setAttribute('data-mention', img.name)
  const space = document.createTextNode(' ')
  range.insertNode(space); range.insertNode(chip)
  sel.removeAllRanges(); range.setStartAfter(space); range.collapse(true); sel.addRange(range)
  showMention.value = false; syncPrompt(mentionAnchor.value)
}

async function generate() {
  const requestPrompt = (prompt.value || editableRef.value?.textContent || '').trim()
  const node = props.node
  if (!node || !requestPrompt || loading.value || uploading.value || allDisplayImages.value.length > 8 || productBinding.error.value) return
  if (!prompt.value) prompt.value = requestPrompt
  if (promptSaveTimer) {
    clearTimeout(promptSaveTimer)
    promptSaveTimer = null
  }
  const existing = parseNodeContent(node.content)
  const outputs = Array.isArray(existing.generated_images) ? existing.generated_images : generatedImages.value
  emit('save', {
    nodeId: node.id,
    content: buildContent({ generated_images: outputs }),
  })
  await startImageGeneration({
    taskId: createGenerationTaskId(),
    canvasId: node.canvas_id,
    nodeId: node.id,
    profile: selectedModel.value,
    prompt: requestPrompt,
    count: selectedCount.value,
    aspectRatio: selectedRatio.value,
    imageSize: selectedResolution.value,
    referenceImages: allDisplayImages.value.map(image => image.url),
    context: buildGenerationContext(productBinding.product.value, references.value, requestPrompt, {
      model: selectedModel.value, aspectRatio: selectedRatio.value, imageSize: selectedResolution.value,
      count: selectedCount.value, generationType: generationType.value,
    }),
    parentGenerationIds: productBinding.parents.value.map(parent => parent.id),
  })
}

watch(modalOpen, async (v) => {
  if (v) {
    await nextTick()
    if (modalContentEditable.value) {
      modalContentEditable.value.innerHTML = promptHtml.value || prompt.value
    }
  } else {
    if (editableRef.value && modalContentEditable.value) {
      editableRef.value.innerHTML = modalContentEditable.value.innerHTML
      syncPrompt()
    }
  }
})
</script>

<template>
  <div v-if="node" data-testid="image-node-panel" :style="panelStyle" class="pointer-events-auto" @pointerdown.stop>
    <div class="bg-neutral-900/95 backdrop-blur rounded-2xl border border-white/20 p-3 relative">
      <button class="btn btn-xs btn-square absolute top-2 right-2 bg-white/10 border border-white/30 text-white hover:bg-white/20 z-10" title="放大编辑" @click="modalOpen = true">
        <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="15 3 21 3 21 9"/><polyline points="9 21 3 21 3 15"/><line x1="21" y1="3" x2="14" y2="10"/><line x1="3" y1="21" x2="10" y2="14"/>
          </svg>
      </button>

      <ProductGenerationBinding :product="productBinding.product.value" :generation-type="generationType" :error="productBinding.error.value" @select="selectProduct" @type="generationType = $event; saveSelection()" />
      <ReferenceImageCards :images="allDisplayImages" :uploading="uploading" @add="onAddImage" @remove="removeImage" @preview="previewImg = $event" @role="changeRole" />
      <p v-if="uploadError" role="alert" class="text-xs text-red-400">{{ uploadError }}</p>
      <p v-if="allDisplayImages.length > 8" role="alert" class="text-xs text-red-400">参考图最多支持 8 张，请删除多余图片后生成。</p>
      <div class="mb-2 flex items-center gap-2" aria-label="电商输出比例">
        <button v-for="ratio in ratioOptions.filter(option => option.primary)" :key="ratio.value" :data-testid="'ratio-' + ratio.value" :aria-pressed="selectedRatio === ratio.value" class="rounded-lg border px-3 py-1 text-xs" :class="selectedRatio === ratio.value ? 'border-cyan-400 bg-cyan-400/20 text-cyan-200' : 'border-white/20 text-white/60'" @click="selectedRatio = ratio.value; saveSelection()">{{ ratio.label }}</button>
      </div>

      <div v-if="showMention" class="relative">
        <MentionDropdown :connected-images="allDisplayImages.map((i, index) => ({ id: index, name: i.label, dataUrl: i.url }))" :filter="mentionFilter" @insert="insertMention" />
      </div>

      <div ref="editableRef" data-testid="image-prompt" contenteditable="true" class="w-full bg-transparent outline-0 text-white text-sm px-1 min-h-[60px] max-h-[100px] overflow-y-auto whitespace-pre-wrap break-words empty:before:content-['输入图片提示词...'] empty:before:text-white/30" @input="onInput" />

      <div class="flex flex-wrap items-center justify-between gap-2 mt-3 pt-3 border-t border-white/10">
        <div class="flex flex-wrap items-center gap-1">
          <span class="text-[10px] text-white/30 ml-1">模型</span>
          <div class="relative inline-flex items-center">
            <select v-model="selectedModel" class="text-xs bg-transparent border-0 text-white/70 hover:text-white h-6 py-0 pl-0 pr-6 w-[124px] outline-none appearance-none cursor-pointer [color-scheme:dark]">
              <option v-for="m in models" :key="m.value" :value="m.value" class="bg-neutral-900 text-white">{{ m.label }}</option>
            </select>
            <svg class="pointer-events-none absolute right-0 top-1/2 -translate-y-1/2 text-white/70" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
          </div>
          <span class="text-[10px] text-white/30">比例</span>
          <div class="relative inline-flex items-center">
            <select v-model="selectedRatio" @change="saveSelection" class="text-xs bg-transparent border-0 text-white/70 hover:text-white h-6 py-0 pl-0 pr-5 w-[42px] outline-none appearance-none cursor-pointer [color-scheme:dark]">
              <option v-for="r in ratioOptions" :key="r.value" :value="r.value" class="bg-neutral-900 text-white">{{ r.label }}</option>
            </select>
            <svg class="pointer-events-none absolute right-0 top-1/2 -translate-y-1/2 text-white/70" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
          </div>
          <span class="text-[10px] text-white/30">像素</span>
          <div class="relative inline-flex items-center">
            <select v-model="selectedResolution" class="text-xs bg-transparent border-0 text-white/70 hover:text-white h-6 py-0 pl-0 pr-5 w-[36px] outline-none appearance-none cursor-pointer [color-scheme:dark]">
              <option v-for="r in resolutionOptions" :key="r" :value="r" class="bg-neutral-900 text-white">{{ r }}</option>
            </select>
            <svg class="pointer-events-none absolute right-0 top-1/2 -translate-y-1/2 text-white/70" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
          </div>
          <span class="text-[10px] text-white/30">数量</span>
          <div class="relative inline-flex items-center">
            <select v-model="selectedCount" aria-label="生成数量" class="text-xs bg-transparent border-0 text-white/70 hover:text-white h-6 py-0 pl-0 pr-5 w-[36px] outline-none appearance-none cursor-pointer [color-scheme:dark]">
              <option v-for="c in countOptions" :key="c" :value="c" class="bg-neutral-900 text-white">{{ c }}x</option>
            </select>
            <svg class="pointer-events-none absolute right-0 top-1/2 -translate-y-1/2 text-white/70" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
          </div>
          <span class="text-[10px] text-white/30">预设</span>
          <div class="relative inline-flex items-center">
            <select v-model="selectedPreset" @change="applyPreset" class="text-xs bg-transparent border-0 text-white/50 hover:text-white h-6 py-0 pl-0 pr-6 w-[56px] outline-none appearance-none cursor-pointer [color-scheme:dark]">
              <option value="" class="bg-neutral-900 text-white/40">预设</option>
              <optgroup v-for="cat in presetCategories" :key="cat" :label="cat">
                <option v-for="pr in presets.filter(p => p.category === cat)" :key="pr.id" :value="pr.id" class="bg-neutral-900 text-white">{{ pr.name }}</option>
              </optgroup>
            </select>
            <svg class="pointer-events-none absolute right-0 top-1/2 -translate-y-1/2 text-white/70" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
          </div>
        </div>
        <button data-testid="generate-image" class="shrink-0 w-9 h-9 rounded-full bg-white flex items-center justify-center text-neutral-900 hover:bg-neutral-200 transition-colors disabled:opacity-50" title="生成图片" aria-label="生成图片" :disabled="loading || uploading || allDisplayImages.length > 8 || !!productBinding.error.value" @pointerdown.stop @click.stop="generate">
          <svg v-if="!loading" xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="19" x2="12" y2="5"/><polyline points="5 12 12 5 19 12"/></svg>
          <span v-else class="loading loading-spinner loading-xs" />
        </button>
      </div>
      <div v-if="generationError" data-testid="image-generation-error" class="mt-2 px-1 text-[11px] text-red-300 break-words">{{ generationError }}</div>
    </div>
  </div>

  <Teleport to="body">
    <div v-if="modalOpen" class="fixed inset-0 z-[9998] flex items-center justify-center bg-black/70" @click.self="modalOpen = false">
      <div class="bg-neutral-900 border border-white/20 rounded-2xl w-[600px] max-w-[calc(100vw-24px)] max-h-[85vh] flex flex-col shadow-xl">
        <div class="flex items-center justify-between px-4 py-3 border-b border-white/10">
          <span class="text-sm font-medium text-white/70">图片生成</span>
          <button class="btn btn-xs btn-square bg-white/10 border border-white/30 text-white hover:bg-white/20" @click="modalOpen = false">
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>
        <div class="flex-1 overflow-y-auto p-5 space-y-3">
          <div ref="modalContentEditable" contenteditable="true" class="w-full bg-transparent outline-0 text-white text-sm min-h-[120px] overflow-y-auto whitespace-pre-wrap break-words empty:before:content-['输入图片提示词...'] empty:before:text-white/30" @input="onModalInput" />
          <ProductGenerationBinding :product="productBinding.product.value" :generation-type="generationType" :error="productBinding.error.value" @select="selectProduct" @type="generationType = $event; saveSelection()" />
      <ReferenceImageCards :images="allDisplayImages" :uploading="uploading" @add="onAddImage" @remove="removeImage" @preview="previewImg = $event" @role="changeRole" />
      <p v-if="uploadError" role="alert" class="text-xs text-red-400">{{ uploadError }}</p>
      <div class="mb-2 flex items-center gap-2" aria-label="电商输出比例">
        <button v-for="ratio in ratioOptions.filter(option => option.primary)" :key="ratio.value" :data-testid="'ratio-' + ratio.value" :aria-pressed="selectedRatio === ratio.value" class="rounded-lg border px-3 py-1 text-xs" :class="selectedRatio === ratio.value ? 'border-cyan-400 bg-cyan-400/20 text-cyan-200' : 'border-white/20 text-white/60'" @click="selectedRatio = ratio.value; saveSelection()">{{ ratio.label }}</button>
      </div>
        </div>
        <div class="flex flex-wrap items-center justify-between gap-2 px-4 py-3 border-t border-white/10">
          <div class="flex flex-wrap items-center gap-1">
            <span class="text-[10px] text-white/30">模型</span>
            <select v-model="selectedModel" class="appearance-none bg-white/5 border border-white/10 rounded text-xs text-white/80 pl-1.5 pr-4 py-1.5"><option v-for="m in models" :key="m.value" :value="m.value" class="bg-neutral-800">{{ m.label }}</option></select>
            <span class="text-[10px] text-white/30">比例</span>
            <select v-model="selectedRatio" @change="saveSelection" class="appearance-none bg-white/5 border border-white/10 rounded text-xs text-white/80 pl-1.5 pr-3 py-1.5"><option v-for="r in ratioOptions" :key="r.value" :value="r.value" class="bg-neutral-800">{{ r.label }}</option></select>
            <span class="text-[10px] text-white/30">像素</span>
            <select v-model="selectedResolution" class="appearance-none bg-white/5 border border-white/10 rounded text-xs text-white/80 pl-1.5 pr-3 py-1.5"><option v-for="r in resolutionOptions" :key="r" :value="r" class="bg-neutral-800">{{ r }}</option></select>
            <span class="text-[10px] text-white/30">数量</span>
            <select v-model="selectedCount" aria-label="生成数量" class="appearance-none bg-white/5 border border-white/10 rounded text-xs text-white/80 pl-1.5 pr-3 py-1.5"><option v-for="c in countOptions" :key="c" :value="c" class="bg-neutral-800">{{ c }}x</option></select>
            <span class="text-[10px] text-white/30">预设</span>
            <select v-model="selectedPreset" @change="applyPreset" class="appearance-none bg-white/5 border border-white/10 rounded text-xs text-white/50 pl-1.5 pr-4 py-1.5"><option value="" class="bg-neutral-800 text-white/40">预设</option><optgroup v-for="cat in presetCategories" :key="cat" :label="cat"><option v-for="pr in presets.filter(p => p.category === cat)" :key="pr.id" :value="pr.id" class="bg-neutral-800 text-white">{{ pr.name }}</option></optgroup></select>
          </div>
          <button class="shrink-0 w-9 h-9 rounded-full bg-white flex items-center justify-center text-neutral-900 hover:bg-neutral-200 disabled:opacity-50" title="生成图片" aria-label="生成图片" :disabled="loading || uploading || allDisplayImages.length > 8 || !!productBinding.error.value" @pointerdown.stop @click.stop="generate">
            <svg v-if="!loading" xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="12" y1="19" x2="12" y2="5"/><polyline points="5 12 12 5 19 12"/></svg>
            <span v-else class="loading loading-spinner loading-xs" />
          </button>
        </div>
      </div>
    </div>

    <div v-if="previewImg" class="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80" @click="previewImg = null">
      <img :src="previewImg.url" class="max-w-[90vw] max-h-[90vh] object-contain rounded-lg" @click.stop />
      <button class="absolute top-4 right-4 btn btn-sm btn-circle bg-white/10 border border-white/30 text-white hover:bg-white/20" @click="previewImg = null">
        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
      </button>
    </div>
  </Teleport>
</template>
