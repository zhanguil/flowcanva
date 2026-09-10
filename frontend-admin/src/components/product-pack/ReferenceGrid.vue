<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Asset, ReferenceAsset } from '../../types'

const props = defineProps<{ references: ReferenceAsset[]; assets: Asset[]; busy?: boolean }>()
const emit = defineEmits<{
  upload: [payload: { files: File[]; role: string }]
  replace: [payload: { index: number; file: File }]
  remove: [index: number]
  addEmpty: []
}>()

const uploadRole = ref('product_main')
const assetById = computed(() => new Map(props.assets.map(asset => [asset.id, asset])))
const roles = [
  { value: 'product_main', label: '产品主图' },
  { value: 'product_structure', label: '产品结构参考' },
  { value: 'dimension', label: '尺寸图' },
  { value: 'material', label: '材质参考' },
  { value: 'scene', label: '场景参考' },
  { value: 'composition', label: '构图参考' },
]

function onUpload(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  if (files.length) emit('upload', { files, role: uploadRole.value })
  input.value = ''
}

function onReplace(event: Event, index: number) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (file) emit('replace', { index, file })
  input.value = ''
}

function importanceLabel(weight: number) {
  if (weight >= 1.5) return '高'
  if (weight <= 0.75) return '低'
  return '标准'
}
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h3 class="text-base font-bold">参考图</h3>
        <p class="mt-1 text-xs text-base-content/45">图片会随生成任务一起提交，锁定项优先保持一致。</p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <select v-model="uploadRole" class="select select-bordered select-sm bg-base-100">
          <option v-for="role in roles" :key="role.value" :value="role.value">{{ role.label }}</option>
        </select>
        <label class="btn btn-primary btn-sm" :class="{ 'btn-disabled': busy }">
          添加参考图
          <input type="file" multiple accept="image/jpeg,image/png,image/webp" class="hidden" :disabled="busy" @change="onUpload" />
        </label>
      </div>
    </div>

    <div v-if="references.length" class="grid gap-4 sm:grid-cols-2 2xl:grid-cols-3">
      <article v-for="(reference, index) in references" :key="reference.id || `${reference.asset_id}-${index}`" class="overflow-hidden rounded-2xl border border-base-200/80 bg-base-100">
        <div class="relative flex aspect-[4/3] items-center justify-center bg-base-200/45 p-3">
          <img v-if="assetById.get(reference.asset_id)?.url" :src="assetById.get(reference.asset_id)?.url" class="h-full w-full object-contain" alt="参考图" />
          <div v-else class="px-4 text-center text-xs text-base-content/35">素材尚未关联或已失效</div>
          <span v-if="reference.locked" class="absolute left-3 top-3 rounded-full bg-base-100/95 px-2.5 py-1 text-[11px] font-semibold shadow-sm">已锁定</span>
        </div>
        <div class="space-y-3 p-4">
          <select v-model="reference.role" class="select select-ghost select-sm w-full px-0 font-semibold">
            <option v-for="role in roles" :key="role.value" :value="role.value">{{ role.label }}</option>
            <option v-if="!roles.some(role => role.value === reference.role)" :value="reference.role">{{ reference.role }}</option>
          </select>
          <div class="flex items-center gap-3 text-xs text-base-content/55">
            <span class="shrink-0">重要程度 {{ importanceLabel(reference.weight) }}</span>
            <input v-model.number="reference.weight" type="range" min="0.5" max="2" step="0.25" class="range range-primary range-xs" />
          </div>
          <div class="flex items-center justify-between gap-2 border-t border-base-200/70 pt-3">
            <label class="flex cursor-pointer items-center gap-2 text-xs font-medium"><input v-model="reference.locked" type="checkbox" class="toggle toggle-primary toggle-xs" />锁定</label>
            <div class="flex items-center gap-1">
              <label class="btn btn-ghost btn-xs">替换<input type="file" accept="image/jpeg,image/png,image/webp" class="hidden" :disabled="busy" @change="onReplace($event, index)" /></label>
              <button class="btn btn-ghost btn-xs text-error/75" type="button" @click="emit('remove', index)">删除</button>
            </div>
          </div>
          <details class="text-[11px] text-base-content/35">
            <summary class="cursor-pointer">开发者详情</summary>
            <input v-model="reference.asset_id" class="input input-bordered input-xs mt-2 w-full font-mono" aria-label="素材编号" />
          </details>
        </div>
      </article>
    </div>

    <div v-else class="rounded-2xl border border-dashed border-base-300 px-6 py-12 text-center">
      <p class="text-sm font-semibold">还没有参考图</p>
      <p class="mt-1 text-xs text-base-content/45">先上传产品主图，智能生成才能稳定保持家具结构和材质。</p>
    </div>

    <details class="text-xs text-base-content/45">
      <summary class="cursor-pointer">开发者选项</summary>
      <button class="btn btn-ghost btn-xs mt-2" type="button" @click="emit('addEmpty')">通过素材编号关联</button>
    </details>
  </div>
</template>
