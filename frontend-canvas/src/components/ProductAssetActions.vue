<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Node } from '../types'
import type { ProductAsset, GenerationType, ProductReference } from '../types/product'
import { resolveNodeOutput } from '../utils/nodeInputResolver'
import { continuationActions, readContent, referenceId } from '../utils/generationContext'
import { useProductAssets } from '../composables/useProductAssets'
import ProductLockEditor from './ProductLockEditor.vue'
const props = defineProps<{ node: Node; selection?: Node[] }>()
const emit = defineEmits<{ (event: 'save', content: string): void; (event: 'continue', type: GenerationType): void; (event: 'bind-product', product: ProductAsset, nodeIds: string[]): void }>()
const store = useProductAssets()
const content = computed(() => readContent(props.node.content))
const product = computed(() => store.getProduct(content.value.product_asset_id || content.value.generation?.rootProductAssetId) || content.value.product_snapshot || content.value.generation?.context?.product)
const creating = ref(false)
const editing = ref(false)
const name = ref('')
const error = ref('')
function bind(product: ProductAsset) {
  emit('save', JSON.stringify({ ...content.value, product_asset_id: product.id, product_snapshot: product }))
}
function create() {
  try {
    const selected = props.selection?.length ? props.selection : [props.node]
    const references: ProductReference[] = selected.flatMap(node => resolveNodeOutput(node).images).map(image => ({
      id: referenceId(image), assetId: image.assetId, url: image.url, name: image.name, role: 'product', origin: image.origin,
    }))
    const created = store.createProduct(name.value, references)
    bind(created); emit('bind-product', created, selected.map(node => node.id))
    creating.value = false; editing.value = true; error.value = ''
  } catch (e) { error.value = (e as Error).message }
}
function save(product: ProductAsset) {
  try { store.saveProduct(product); bind(product); editing.value = false; error.value = '' }
  catch (e) { error.value = (e as Error).message }
}
</script>
<template>
  <div class="flex items-center gap-1" @pointerdown.stop>
    <button v-if="!product" class="rounded px-2 py-1 text-xs text-cyan-200 hover:bg-white/10" data-testid="create-product-asset" @click="creating = true">建立产品</button>
    <button v-else class="max-w-32 truncate rounded px-2 py-1 text-xs text-cyan-200 hover:bg-white/10" data-testid="asset-product-lock" :title="product.name" @click="editing = true">产品锁 · {{ product.name }}</button>
    <details v-if="content.generation || content.parent_generation_node_id || product" class="relative">
      <summary class="cursor-pointer px-2 text-xs text-white/70" aria-label="产品生成操作">继续生成</summary>
      <div class="absolute left-0 top-full z-10 mt-2 w-28 rounded-lg border border-white/15 bg-neutral-900 p-1 shadow-xl">
        <button v-for="action in continuationActions" :key="action.type" :data-testid="`continue-${action.type}`" class="block w-full rounded px-2 py-2 text-left text-xs text-white/80 hover:bg-white/10" @click="emit('continue', action.type)">{{ action.label }}</button>
      </div>
    </details>
    <Teleport to="body">
      <div v-if="creating" class="fixed inset-0 z-[12010] flex items-center justify-center bg-black/70" @pointerdown.stop>
        <form class="w-80 rounded-xl border border-white/20 bg-neutral-900 p-5 text-white" @submit.prevent="create">
          <label>建立 Product Asset<input v-model="name" aria-label="新产品名称" required class="my-3 w-full rounded border border-white/20 bg-white/5 p-2" placeholder="产品名称" /></label>
          <p v-if="error" class="text-xs text-red-300">{{ error }}</p>
          <div class="flex justify-between"><button type="button" @click="creating = false">取消</button><button type="submit" class="rounded bg-cyan-500 px-3 py-1 text-black">创建产品</button></div>
        </form>
      </div>
    </Teleport>
    <ProductLockEditor :product="editing ? product : null" :error="error" @close="editing = false" @save="save" />
  </div>
</template>
