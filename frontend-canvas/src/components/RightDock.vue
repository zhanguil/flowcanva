<script setup lang="ts">
import type { Node } from '../types'
import AIAssistantPanel from './AIAssistantPanel.vue'
import NodeLayerPanel from './NodeLayerPanel.vue'

export type RightDockTab = 'assistant' | 'project' | 'layers'

defineProps<{
  activeTab: RightDockTab | null
  canvasId: string
  canvasName: string
  nodes: Node[]
  edgeCount: number
  assetCount: number
  selectedNodeId: string | null
  selectedImages: { nodeId: string; url: string; name: string }[]
  devMode: boolean
}>()

const emit = defineEmits<{
  (e: 'update:activeTab', tab: RightDockTab | null): void
  (e: 'select-node', nodeId: string): void
  (e: 'apply-prompt', prompt: string): void
  (e: 'open-assets'): void
  (e: 'load-test-canvas'): void
}>()

const tabs: { value: RightDockTab; label: string }[] = [
  { value: 'assistant', label: 'AI 助手' },
  { value: 'project', label: '项目' },
  { value: 'layers', label: '图层' },
]
</script>

<template>
  <aside
    v-if="activeTab"
    data-testid="right-dock"
    class="fixed right-0 top-12 bottom-0 z-40 flex w-[390px] max-w-[calc(100vw-56px)] flex-col border-l border-white/10 bg-neutral-950/95 text-white shadow-2xl backdrop-blur-xl"
    @pointerdown.stop
    @wheel.stop
  >
    <div class="flex h-11 shrink-0 items-center gap-1 border-b border-white/10 px-2" role="tablist" aria-label="右侧面板">
      <button
        v-for="tab in tabs"
        :key="tab.value"
        :data-testid="`dock-tab-${tab.value}`"
        class="h-8 flex-1 rounded-lg text-xs transition-colors"
        :class="activeTab === tab.value ? 'bg-white/12 text-white' : 'text-white/45 hover:bg-white/5 hover:text-white/75'"
        role="tab"
        :aria-selected="activeTab === tab.value"
        @click="emit('update:activeTab', tab.value)"
      >{{ tab.label }}</button>
      <button
        data-testid="dock-close"
        class="h-8 w-8 shrink-0 rounded-lg text-white/45 hover:bg-white/10 hover:text-white"
        title="关闭右侧面板"
        aria-label="关闭右侧面板"
        @click="emit('update:activeTab', null)"
      >×</button>
    </div>

    <div class="min-h-0 flex-1 overflow-hidden">
      <AIAssistantPanel
        v-if="activeTab === 'assistant'"
        :open="true"
        :canvas-id="canvasId"
        :selected-images="selectedImages"
        @close="emit('update:activeTab', null)"
        @apply-prompt="emit('apply-prompt', $event)"
      />

      <section v-else-if="activeTab === 'project'" data-testid="project-panel" class="flex h-full flex-col overflow-y-auto p-4">
        <div class="text-sm font-semibold">{{ canvasName || '未命名画布' }}</div>
        <div class="mt-1 text-[11px] text-white/40">当前项目概览</div>
        <div class="mt-4 grid grid-cols-3 gap-2">
          <div class="rounded-xl border border-white/10 bg-white/5 p-3 text-center">
            <div class="text-lg font-semibold">{{ nodes.length }}</div><div class="text-[10px] text-white/40">节点</div>
          </div>
          <div class="rounded-xl border border-white/10 bg-white/5 p-3 text-center">
            <div class="text-lg font-semibold">{{ edgeCount }}</div><div class="text-[10px] text-white/40">连线</div>
          </div>
          <div class="rounded-xl border border-white/10 bg-white/5 p-3 text-center">
            <div class="text-lg font-semibold">{{ assetCount }}</div><div class="text-[10px] text-white/40">资产</div>
          </div>
        </div>
        <button data-testid="open-assets" class="mt-4 h-10 rounded-xl border border-white/10 bg-white/5 text-xs text-white/75 hover:bg-white/10" @click="emit('open-assets')">
          打开资产库
        </button>
        <button
          v-if="devMode"
          data-testid="load-test-canvas"
          class="mt-2 h-10 rounded-xl border border-cyan-400/20 bg-cyan-400/10 text-xs text-cyan-200 hover:bg-cyan-400/15"
          @click="emit('load-test-canvas')"
        >加载数据流测试画布</button>
      </section>

      <NodeLayerPanel
        v-else
        :nodes="nodes"
        :selected-node-id="selectedNodeId"
        @select="emit('select-node', $event)"
      />
    </div>
  </aside>
</template>
