<script setup lang="ts">
import { ref } from 'vue'
import type { ProductAsset, GenerationType } from '../types/product'
import { generationTypes } from '../types/product'
import { useProductAssets } from '../composables/useProductAssets'
import ProductLockEditor from './ProductLockEditor.vue'
defineProps<{ product: ProductAsset | null; generationType: GenerationType; error: string }>()
defineEmits<{ (event: 'select', id: string): void; (event: 'type', value: GenerationType): void }>()
const store = useProductAssets()
const editing = ref<ProductAsset | null>(null)
const errorMessage = ref('')
function save(product: ProductAsset) {
  try { store.saveProduct(product); editing.value = null; errorMessage.value = '' }
  catch (error) { errorMessage.value = (error as Error).message }
}
</script>
<template>
  <div class="mb-3">
    <div class="flex flex-wrap items-center gap-2 text-xs text-white/70">
      <select :value="product?.id || ''" aria-label="选择产品" data-testid="generation-product" class="max-w-56 rounded border border-white/15 bg-neutral-900 px-2 py-1" @change="$emit('select', ($event.target as HTMLSelectElement).value)">
        <option value="">未关联产品</option>
        <option v-if="product && !store.products.value.some(p => p.id === product?.id)" :value="product.id">{{ product.name }}</option>
        <option v-for="item in store.products.value" :key="item.id" :value="item.id">{{ item.name }}</option>
      </select>
      <button v-if="product" data-testid="generation-product-lock" class="text-cyan-200" @click="editing = product">产品锁</button>
      <select :value="generationType" aria-label="生成类型" class="rounded border border-white/15 bg-neutral-900 px-2 py-1" @change="$emit('type', ($event.target as HTMLSelectElement).value as GenerationType)">
        <option v-for="[value, label] in generationTypes" :key="value" :value="value">{{ label }}</option>
      </select>
    </div>
    <p v-if="error || store.storageError.value" role="alert" class="mt-1 text-xs text-red-300">{{ error || store.storageError.value }}</p>
    <ProductLockEditor :product="editing" :error="errorMessage" @close="editing = null" @save="save" />
  </div>
</template>
