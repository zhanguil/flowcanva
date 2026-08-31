<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { chatWithAssistant } from '../api'

defineProps<{ open: boolean }>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'apply-prompt', prompt: string): void
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
    const result = await chatWithAssistant(history)
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
    class="fixed right-0 top-12 bottom-0 z-40 w-[390px] border-l border-white/10 bg-neutral-950/95 text-white shadow-2xl backdrop-blur-xl flex flex-col"
    @pointerdown.stop
    @wheel.stop
  >
    <div class="h-14 shrink-0 px-4 flex items-center justify-between border-b border-white/10">
      <div>
        <div class="text-sm font-semibold">AI Assistant</div>
        <div class="text-[11px] text-white/40">GPT-5.6 Sol · 电商视觉</div>
      </div>
      <button class="w-8 h-8 rounded-lg text-white/50 hover:text-white hover:bg-white/10" title="关闭 AI Assistant" @click="emit('close')">×</button>
    </div>

    <div class="px-3 py-2 flex gap-1.5 overflow-x-auto border-b border-white/10">
      <button
        v-for="action in quickActions"
        :key="action[0]"
        class="shrink-0 px-2.5 py-1 rounded-full border border-white/10 text-[11px] text-white/55 hover:text-white hover:bg-white/10"
        @click="useQuickAction(action[1])"
      >{{ action[0] }}</button>
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
            @click="emit('apply-prompt', message.content)"
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
