<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { chatWithAssistant } from '../api'
import { useAssets } from '../composables/useAssets'
import { isSupportedCanvasImage } from '../utils/canvasCoordinates'

const props = defineProps<{
  open: boolean
  canvasId: string
	  selectedImages: { nodeId: string; assetId: string; url: string; name: string; mimeType: string; width: number; height: number }[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
	  (e: 'apply-prompt', payload: { prompt: string; referenceNodeIds: string[] }): void
}>()

interface Message {
  role: 'user' | 'assistant'
  content: string
  error?: boolean
}

const messages = ref<Message[]>([
  { role: 'assistant', content: '你好，我是 AI 电商视觉助手。可以帮你做产品、结构、材质、场景与多图对比分析，也可以编写或修改生图提示词。' },
])
const input = ref('')
const loading = ref(false)
const scrollRef = ref<HTMLDivElement | null>(null)
const { addAsset } = useAssets()

interface VisualContextImage {
	  key: string
	  nodeId?: string
	  assetId: string
	  url: string
	  name: string
	  mimeType: string
	  width: number
	  height: number
}

const visualContext = ref<VisualContextImage[]>([])
const contextUploading = ref(false)

const quickActions = [
  ['产品分析', '请分析当前产品的外观特征与电商视觉卖点。'],
  ['结构分析', '请分析当前产品的可见结构，并把无法确认的部分标记为待确认。'],
  ['材质分析', '请分析当前产品的可见材质、纹理、粗糙度与反射特征。'],
  ['场景参考', '请分析参考场景的构图、摄影机、光影和道具。'],
  ['多图比较', '请比较提供的图片，列出相同点、差异和可复用元素。'],
  ['生图提示词', '请把我的要求整理为可直接用于家具电商生图的提示词。'],
]

function useQuickAction(value: string) {
  input.value = value
}

function addSelectedImages() {
	  const existing = new Set(visualContext.value.map(image => image.key))
	  for (const image of props.selectedImages) {
	    const key = `node:${image.nodeId}:${image.assetId || image.url}`
	    if (existing.has(key)) continue
	    existing.add(key)
	    visualContext.value.push({ key, ...image })
	  }
}

function removeContextImage(key: string) {
	  visualContext.value = visualContext.value.filter(image => image.key !== key)
}

async function addLocalFiles(files: File[]) {
	  const supported = files.filter(isSupportedCanvasImage)
	  if (supported.length === 0) return
	  contextUploading.value = true
	  try {
	    for (const file of supported) {
	      const asset = await addAsset(file)
	      if (!asset) continue
	      const key = `asset:${asset.id}`
	      if (visualContext.value.some(image => image.key === key)) continue
	      visualContext.value.push({
	        key,
	        assetId: asset.id,
	        url: asset.url,
	        name: asset.filename,
	        mimeType: asset.mime_type,
	        width: asset.width,
	        height: asset.height,
	      })
	    }
	  } finally {
	    contextUploading.value = false
	  }
}

function chooseLocalImages() {
	  const input = document.createElement('input')
	  input.type = 'file'
	  input.accept = '.png,.jpg,.jpeg,.webp,image/png,image/jpeg,image/webp'
	  input.multiple = true
  input.onchange = () => addLocalFiles(Array.from(input.files || []))
	  input.click()
}

function handleContextDragOver(event: DragEvent) {
	  if (Array.from(event.dataTransfer?.items || []).some(item => item.kind === 'file')) {
	    event.preventDefault()
	    if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
	  }
}

async function handleContextDrop(event: DragEvent) {
	  event.preventDefault()
	  event.stopPropagation()
	  await addLocalFiles(Array.from(event.dataTransfer?.files || []))
}

async function scrollToBottom() {
  await nextTick()
  if (scrollRef.value) scrollRef.value.scrollTop = scrollRef.value.scrollHeight
}

async function send() {
  const content = input.value.trim()
  if (!content || loading.value) return
  messages.value.push({ role: 'user', content })
  input.value = ''
  loading.value = true
  await scrollToBottom()
  try {
    const history = messages.value
      .filter(message => !message.error)
      .map(({ role, content }) => ({ role, content }))
    const result = await chatWithAssistant(
      history,
	      visualContext.value.filter(image => !image.nodeId).map(image => image.url),
      props.canvasId,
	      [...new Set(visualContext.value.flatMap(image => image.nodeId ? [image.nodeId] : []))],
    )
    messages.value.push({ role: 'assistant', content: result.content })
  } catch (error: any) {
    messages.value.push({ role: 'assistant', content: error?.message || 'AI Assistant 调用失败', error: true })
  } finally {
    loading.value = false
    await scrollToBottom()
  }
}
</script>

<template>
  <aside
    v-show="open"
    data-testid="assistant-panel"
    class="flex h-full min-h-0 w-full flex-col bg-neutral-950/95 text-white"
    @pointerdown.stop
    @wheel.stop
  >
    <div class="h-14 shrink-0 px-4 flex items-center justify-between border-b border-white/10">
      <div>
        <div class="text-sm font-semibold">AI Assistant</div>
        <div class="text-[11px] text-white/40">GPT-5.6 Sol · 电商视觉</div>
      </div>
      <button class="w-8 h-8 rounded-lg text-white/50 hover:text-white hover:bg-white/10" title="关闭 AI Assistant" aria-label="关闭 AI Assistant" @click="emit('close')">×</button>
    </div>

    <div class="px-3 py-2 flex gap-1.5 overflow-x-auto border-b border-white/10">
      <button
        v-for="action in quickActions"
        :key="action[0]"
        class="shrink-0 px-2.5 py-1 rounded-full border border-white/10 text-[11px] text-white/55 hover:text-white hover:bg-white/10"
        @click="useQuickAction(action[1])"
      >{{ action[0] }}</button>
    </div>

	    <div
	      data-testid="assistant-visual-context"
	      class="shrink-0 px-3 py-2 border-b border-white/10"
	      @dragover.stop="handleContextDragOver"
	      @drop="handleContextDrop"
	    >
      <div class="flex items-center justify-between mb-1.5">
        <span class="text-[11px] text-white/45">视觉上下文</span>
	        <span class="text-[10px]" :class="visualContext.length ? 'text-cyan-300' : 'text-white/25'">{{ visualContext.length ? `${visualContext.length} 张` : '未添加' }}</span>
      </div>
	      <div class="mb-2 flex gap-1.5">
	        <button data-testid="add-selected-images" class="h-7 rounded-lg border border-cyan-400/25 bg-cyan-400/10 px-2 text-[10px] text-cyan-200 disabled:opacity-35" :disabled="selectedImages.length === 0" @click="addSelectedImages">添加选中图片</button>
	        <button data-testid="add-local-context" class="h-7 rounded-lg border border-white/10 bg-white/5 px-2 text-[10px] text-white/60" :disabled="contextUploading" @click="chooseLocalImages">{{ contextUploading ? '上传中…' : '添加图片' }}</button>
	      </div>
	      <div v-if="visualContext.length" data-testid="visual-context-list" class="flex gap-1.5 overflow-x-auto pb-1">
	        <div v-for="(image, index) in visualContext" :key="image.key" data-testid="visual-context-item" class="relative shrink-0 w-14 h-14 rounded-lg overflow-hidden border border-cyan-400/30 bg-white/5">
          <img :src="image.url" class="w-full h-full object-cover" />
          <span class="absolute left-0 right-0 bottom-0 bg-black/65 text-[8px] text-center text-white/80">图{{ index + 1 }}</span>
	          <button class="absolute right-0 top-0 h-4 w-4 bg-black/70 text-[10px] text-white/80" :aria-label="`移除${image.name}`" @click="removeContextImage(image.key)">×</button>
        </div>
      </div>
	      <div v-else class="rounded-lg border border-dashed border-white/10 px-2 py-2 text-center text-[10px] text-white/25">可拖入 PNG / JPG / WEBP；添加后不会自动发送</div>
    </div>

    <div ref="scrollRef" class="flex-1 overflow-y-auto p-4 space-y-4">
      <div v-for="(message, index) in messages" :key="index" class="flex" :class="message.role === 'user' ? 'justify-end' : 'justify-start'">
        <div
          class="max-w-[92%] rounded-2xl px-3.5 py-2.5 text-sm leading-6 whitespace-pre-wrap break-words"
          :class="message.role === 'user' ? 'bg-blue-600 text-white rounded-br-md' : message.error ? 'bg-red-500/10 text-red-300 border border-red-400/20 rounded-bl-md' : 'bg-white/[0.07] text-white/85 border border-white/10 rounded-bl-md'"
        >
          {{ message.content }}
          <button
            v-if="message.role === 'assistant' && index > 0 && !message.error"
            class="mt-3 w-full h-8 rounded-lg border border-blue-400/25 bg-blue-500/10 text-xs text-blue-300 hover:bg-blue-500/20"
	            @click="emit('apply-prompt', { prompt: message.content, referenceNodeIds: [...new Set(visualContext.flatMap(image => image.nodeId ? [image.nodeId] : []))] })"
          >应用到生图提示词</button>
        </div>
      </div>
      <div v-if="loading" class="flex justify-start">
        <div class="rounded-2xl rounded-bl-md border border-white/10 bg-white/[0.07] px-3.5 py-3 flex gap-1">
          <span class="loading loading-dots loading-sm text-white/50" />
        </div>
      </div>
    </div>

    <div class="shrink-0 border-t border-white/10 p-3">
      <textarea
        v-model="input"
        rows="3"
        maxlength="20000"
        placeholder="输入产品分析、比较或提示词要求…"
        class="w-full resize-none rounded-xl border border-white/10 bg-white/[0.06] px-3 py-2.5 text-sm text-white outline-none placeholder:text-white/25 focus:border-blue-400/40"
        @keydown.enter.exact.prevent="send"
      />
      <div class="mt-2 flex items-center justify-between">
        <span class="text-[10px] text-white/30">Enter 发送 · Shift+Enter 换行</span>
        <button
          class="h-8 px-4 rounded-lg bg-white text-xs font-medium text-neutral-900 hover:bg-neutral-200 disabled:opacity-40"
          :disabled="loading || !input.trim()"
          @click="send"
        >发送</button>
      </div>
    </div>
  </aside>
</template>
