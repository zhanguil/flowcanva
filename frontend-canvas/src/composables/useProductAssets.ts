import { computed, ref } from 'vue'
import { createProductRepository, emptyProductDatabase, type ProductDatabase } from '../repositories/productRepository'
import { emptyConstraints, emptyMetadata, type ProductAsset, type ProductReference, type GenerationRecord } from '../types/product'

const database = ref<ProductDatabase>(emptyProductDatabase())
const storageError = ref('')
let loaded = false
function ensureLoaded() {
  if (loaded || typeof localStorage === 'undefined') return
  try { database.value = createProductRepository(localStorage).load(); loaded = true }
  catch (error) { storageError.value = `产品库读取失败：${(error as Error).message}` }
}
function commit(update: (draft: ProductDatabase) => void) {
  ensureLoaded()
  if (!loaded) throw new Error(storageError.value || '当前环境不支持产品持久化')
  const draft = JSON.parse(JSON.stringify(database.value)) as ProductDatabase
  update(draft)
  try {
    createProductRepository(localStorage).save(draft)
    database.value = draft
    storageError.value = ''
  } catch { storageError.value = '产品库保存失败，请检查浏览器存储空间；本次变更未保存'; throw new Error(storageError.value) }
}
export function useProductAssets() {
  ensureLoaded()
  const products = computed(() => Object.values(database.value.products))
  function createProduct(name: string, references: ProductReference[]): ProductAsset {
    if (!name.trim() || !references.length) throw new Error('请填写产品名称并选择至少一张图片')
    const now = new Date().toISOString()
    const product: ProductAsset = {
      id: `product_${crypto.randomUUID()}`, name: name.trim(), coverImageId: references[0].id,
      referenceIds: [...new Set(references.map(r => r.id))], metadata: emptyMetadata(), constraints: emptyConstraints(), createdAt: now, updatedAt: now,
    }
    commit(draft => {
      draft.products[product.id] = product
      for (const reference of references) draft.references[reference.id] = reference
    })
    return product
  }
  function saveProduct(product: ProductAsset) {
    if (!product.name.trim()) throw new Error('产品名称不能为空')
    commit(draft => { draft.products[product.id] = { ...product, updatedAt: new Date().toISOString() } })
  }
  function saveGeneration(generation: GenerationRecord) {
    commit(draft => { draft.generations[generation.id] = generation })
  }
  return {
    products, storageError, createProduct, saveProduct, saveGeneration,
    getProduct: (id: string) => database.value.products[id],
    getReferences: (product: ProductAsset) => product.referenceIds.map(id => database.value.references[id]).filter((r): r is ProductReference => Boolean(r)),
    getGeneration: (id: string) => database.value.generations[id],
    findGenerationForAsset: (assetId: string) => Object.values(database.value.generations).find(g => g.outputAssetIds.includes(assetId)),
  }
}
