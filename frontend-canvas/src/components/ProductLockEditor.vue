<script setup lang="ts">
import { ref, watch } from 'vue'
import { lockOptions, type ProductAsset } from '../types/product'
import { constraintsFromMetadata } from '../utils/productConstraints'
const props = defineProps<{ product: ProductAsset | null; error?: string }>()
const emit = defineEmits<{ (event: 'close'): void; (event: 'save', product: ProductAsset): void }>()
const draft = ref<ProductAsset | null>(null)
const materials = ref('')
const rules = ref('')
watch(() => props.product, value => {
  draft.value = value ? JSON.parse(JSON.stringify(value)) : null
  materials.value = value?.metadata.materials.join('、') || ''
  rules.value = value?.constraints.customRules.join('\n') || ''
}, { immediate: true })
const dimensions = [['width', '宽度'], ['depth', '深度'], ['height', '高度'], ['panelThickness', '板厚']] as const
function save() {
  if (!draft.value) return
  draft.value.metadata.materials = materials.value.split(/[、,，\n]/).map(value => value.trim()).filter(Boolean)
  draft.value.constraints.customRules = rules.value.split('\n')
  draft.value.constraints = constraintsFromMetadata(draft.value.metadata, draft.value.constraints)
  emit('save', draft.value)
}
</script>
<template>
  <Teleport to="body">
    <div v-if="draft" class="fixed inset-0 z-[12010] flex items-center justify-center bg-black/70 p-4" @pointerdown.stop>
      <form data-testid="product-lock-editor" class="max-h-[90vh] w-[520px] max-w-full overflow-y-auto rounded-2xl border border-white/15 bg-neutral-900 p-5 text-sm text-white" @submit.prevent="save">
        <div class="mb-4 flex justify-between"><strong>产品锁</strong><button type="button" aria-label="关闭产品锁" @click="emit('close')">×</button></div>
        <label class="block">产品名称<input v-model="draft.name" required aria-label="产品名称" class="product-input" /></label>
        <div class="my-3 flex flex-wrap gap-3">
          <label v-for="[key, label] in lockOptions" :key="key" class="flex items-center gap-1"><input v-model="draft.constraints[key]" type="checkbox" :aria-label="`锁定${label}`" class="accent-cyan-400" />{{ label }}</label>
        </div>
        <p class="mb-2 text-xs text-white/50">已填写且锁定的字段随每次生成传递；未填写的具体值保持待确认。</p>
        <details class="mb-3"><summary class="cursor-pointer text-cyan-200">产品资料（尺寸 / 材质）</summary>
        <label class="block">分类<input v-model="draft.metadata.category" aria-label="产品分类" class="product-input" placeholder="未知 / 待确认" /></label>
        <div class="grid grid-cols-2 gap-2">
          <label v-for="[key, label] in dimensions" :key="key">{{ label }}（mm）<input :value="draft.metadata.dimensions[key]" :aria-label="label" type="number" min="1" step="any" class="product-input" placeholder="待确认" @input="draft.metadata.dimensions[key] = ($event.target as HTMLInputElement).value === '' ? null : Number(($event.target as HTMLInputElement).value)" /></label>
          <label>抽屉数量<input :value="draft.metadata.drawerCount" aria-label="抽屉数量" type="number" min="0" step="1" class="product-input" placeholder="待确认" @input="draft.metadata.drawerCount = ($event.target as HTMLInputElement).value === '' ? null : Number(($event.target as HTMLInputElement).value)" /></label>
        </div>
        <label class="block">材质<input v-model="materials" aria-label="产品材质" class="product-input" placeholder="以顿号分隔；未确认请留空" /></label>
        <label class="block">木纹方向<input v-model="draft.metadata.textureDirection" aria-label="木纹方向" class="product-input" placeholder="未知 / 待确认" /></label>
        <label class="block">五金 / 插座结构<input v-model="draft.metadata.hardware" aria-label="五金" class="product-input" placeholder="未知 / 待确认" /></label>

        </details>
        <label class="block">补充约束（每行一条）<textarea v-model="rules" aria-label="补充约束" class="product-input" rows="3" /></label>
        <label class="block">备注<textarea v-model="draft.metadata.notes" aria-label="产品备注" class="product-input" rows="2" /></label>
        <p v-if="error" role="alert" class="mb-2 text-red-300">{{ error }}</p>
        <button type="submit" class="w-full rounded-lg bg-cyan-500 py-2 font-medium text-black">保存产品锁</button>
      </form>
    </div>
  </Teleport>
</template>
<style scoped>
.product-input { display: block; width: 100%; margin: 4px 0 10px; padding: 6px 8px; border: 1px solid #ffffff25; border-radius: 6px; background: #ffffff08; color: white; }
</style>
