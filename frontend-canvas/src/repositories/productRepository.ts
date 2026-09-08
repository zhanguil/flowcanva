import type { ProductAsset, ProductReference, GenerationRecord } from '../types/product'

export interface ProductDatabase {
  schemaVersion: 1
  products: Record<string, ProductAsset>
  references: Record<string, ProductReference>
  generations: Record<string, GenerationRecord>
}
export interface ProductRepository {
  load(): ProductDatabase
  save(data: ProductDatabase): void
}
export const PRODUCT_STORAGE_KEY = 'flowcanva.product-core.v1'
function isRecord(value: unknown): value is Record<string, any> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}
export function emptyProductDatabase(): ProductDatabase {
  return { schemaVersion: 1, products: {}, references: {}, generations: {} }
}
// The UI/store uses this boundary; a server repository can replace local storage.
// Fail closed on corrupt data or exhausted storage instead of silently losing locks.
export function createProductRepository(storage: Pick<Storage, 'getItem' | 'setItem'>): ProductRepository {
  return {
    load() {
      const raw = storage.getItem(PRODUCT_STORAGE_KEY)
      if (!raw) return emptyProductDatabase()
      const data = JSON.parse(raw)
      if (!isRecord(data) || data.schemaVersion !== 1 || !isRecord(data.products) || !isRecord(data.references) || !isRecord(data.generations)) throw new Error('产品库格式无法读取，请保留浏览器数据并检查版本')
      for (const product of Object.values(data.products)) {
        if (!product?.id || !product.name || !Array.isArray(product.referenceIds) || !product.metadata?.dimensions || !Array.isArray(product.metadata.materials) || !isRecord(product.constraints?.lockedFields) || !Array.isArray(product.constraints?.customRules)) throw new Error('产品资料不完整，已停止加载以保护现有数据')
      }
      for (const reference of Object.values(data.references)) {
        if (!reference?.id || !reference.url || !reference.role) throw new Error('参考图片记录不完整')
      }
      for (const generation of Object.values(data.generations)) {
        if (!generation?.id || !Array.isArray(generation.outputAssetIds) || !Array.isArray(generation.context?.references)) throw new Error('生成血缘记录不完整')
      }
      return { schemaVersion: 1, products: data.products, references: data.references, generations: data.generations }
    },
    save(data) { storage.setItem(PRODUCT_STORAGE_KEY, JSON.stringify(data)) },
  }
}
