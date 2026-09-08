import { computed } from 'vue'
import type { Node } from '../types'
import type { ResolvedNodeInput } from '../utils/nodeInputResolver'
import type { GenerationRecord, ProductAsset } from '../types/product'
import { inheritedReferences, readContent, resolveProduct, mergeReferences } from '../utils/generationContext'
import { useProductAssets } from './useProductAssets'

export function useGenerationProduct(getNode: () => Node | null, getInputs: () => ResolvedNodeInput[]) {
  const store = useProductAssets()
  const inputs = computed(() => getInputs().flatMap(input => input.images))
  const parents = computed(() => {
    const records = new Map<string, GenerationRecord>()
    for (const image of inputs.value) if (image.generation) records.set(image.generation.id, image.generation)
    return [...records.values()]
  })
  const binding = computed(() => {
    const content = readContent(getNode()?.content)
    const snapshots = inputs.value.flatMap(image => image.product ? [image.product] : [])
    const selected = store.getProduct(content.product_asset_id) || content.product_snapshot
    try {
      const resolved = resolveProduct(selected, snapshots, parents.value)
      return { product: resolved ? store.getProduct(resolved.id) || resolved : null, error: '' }
    } catch (error) { return { product: null, error: (error as Error).message } }
  })
  const product = computed<ProductAsset | null>(() => binding.value.product)
  const references = computed(() => {
    // Unbound legacy image workflows keep their explicit-reference behavior.
    if (!product.value) return []
    const canonical = product.value ? store.getReferences(product.value) : []
    const seeded = readContent(getNode()?.content).product_references || []
    return mergeReferences(inheritedReferences(parents.value), seeded, canonical)
  })
  return { ...store, product, parents, inherited: references, error: computed(() => binding.value.error) }
}
