<script setup lang="ts">
import { computed } from 'vue'
import type { Asset, GenerationJob, ProductSKU } from '../../types'

const props = defineProps<{ jobs: GenerationJob[]; assets: Asset[]; skus: ProductSKU[]; busy?: boolean }>()
const emit = defineEmits<{
  retry: [id: string]
  edit: []
  useAsReference: [assetId: string]
  cancel: [id: string]
}>()

const assetById = computed(() => new Map(props.assets.map(asset => [asset.id, asset])))
const skuById = computed(() => new Map(props.skus.map(sku => [sku.id, sku])))
const statusLabel: Record<string, string> = {
  queued: '排队中', running: '生成中', generated: '处理中', validating: '检查中', success: '已完成',
  failed: '生成失败', rejected: '未通过', cancelled: '已取消', retrying: '重试中',
}

function assetFor(job: GenerationJob) {
  return assetById.value.get(job.result_asset_id)
}

function friendlyError(job: GenerationJob) {
  if (!job.error_message) return ''
  return job.error_message.includes('未配置') ? '生成服务尚未配置，暂时无法开始生成' : '这张图片生成失败，可以单独重试。'
}
</script>

<template>
  <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
    <article v-for="job in jobs" :key="job.id" class="overflow-hidden rounded-2xl border border-base-200/80 bg-base-100">
      <div class="relative flex aspect-[4/3] items-center justify-center bg-base-200/45">
        <img v-if="assetFor(job)?.url" :src="assetFor(job)?.url" class="h-full w-full object-contain" :alt="`${skuById.get(job.sku_id)?.name || '产品规格'}生成结果`" />
        <div v-else class="px-6 text-center">
          <span v-if="['queued','running','retrying','generated','validating'].includes(job.status)" class="loading loading-spinner loading-md text-primary" />
          <p v-else class="text-xs text-base-content/40">暂无结果图片</p>
        </div>
        <span class="absolute right-3 top-3 rounded-full bg-base-100/95 px-2.5 py-1 text-[11px] font-semibold shadow-sm">{{ statusLabel[job.status] || job.status }}</span>
      </div>
      <div class="space-y-3 p-4">
        <div class="flex items-center justify-between gap-3">
          <div>
            <p class="font-bold">{{ skuById.get(job.sku_id)?.label || skuById.get(job.sku_id)?.name || job.sku_id }}</p>
            <p class="mt-0.5 text-xs text-base-content/45">{{ job.aspect_ratio }} · 商品主图</p>
          </div>
        </div>

        <div v-if="friendlyError(job)" class="rounded-xl bg-base-200/60 p-3 text-xs">
          <p>{{ friendlyError(job) }}</p>
          <details class="mt-2 text-base-content/45"><summary class="cursor-pointer">查看技术详情</summary><p class="mt-1 break-words font-mono">{{ job.error_message }}</p></details>
        </div>

        <div class="flex flex-wrap gap-1 border-t border-base-200/70 pt-3">
          <button v-if="['failed','rejected','cancelled','success'].includes(job.status)" class="btn btn-ghost btn-xs" :disabled="busy" type="button" @click="emit('retry', job.id)">重新生成</button>
          <button v-if="job.status === 'success'" class="btn btn-ghost btn-xs" type="button" @click="emit('edit')">继续编辑</button>
          <button v-if="job.status === 'success' && job.result_asset_id" class="btn btn-ghost btn-xs" :disabled="busy" type="button" @click="emit('useAsReference', job.result_asset_id)">设为参考</button>
          <a v-if="assetFor(job)?.url" class="btn btn-ghost btn-xs" :href="assetFor(job)?.url" :download="assetFor(job)?.filename || '生成图片'">下载</a>
          <button v-if="['queued','running','retrying'].includes(job.status)" class="btn btn-ghost btn-xs" type="button" @click="emit('cancel', job.id)">取消</button>
        </div>
      </div>
    </article>
  </div>
</template>
