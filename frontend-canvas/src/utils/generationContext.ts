import { emptyConstraints, type ProductAsset, type ProductReference, type GenerationRecord, type GenerationContext, type GenerationType } from '../types/product'

export function readContent(content?: string): Record<string, any> {
  try { const data = JSON.parse(content || '{}'); return data && typeof data === 'object' && !Array.isArray(data) ? data : {} }
  catch { return {} }
}
export function referenceId(image: { assetId?: string; url: string }) {
  // URLs without asset IDs remain usable for legacy nodes. Never re-upload them.
  return image.assetId ? `ref_${image.assetId}` : `ref_url_${image.url}`
}
export function mergeReferences(...groups: ProductReference[][]): ProductReference[] {
  const references = new Map<string, ProductReference>()
  for (const group of groups) for (const reference of group) references.set(reference.url, reference)
  return [...references.values()]
}
export function inheritedReferences(parents: GenerationRecord[]): ProductReference[] {
  // Retain product/material/etc. references; replacing old generated views with
  // the directly connected view keeps ten generations within the image limit.
  return mergeReferences(...parents.map(parent => parent.context.references.filter(r => !r.generationId || ['product', 'material', 'lighting', 'style', 'hardware'].includes(r.role))))
}
export function resolveProduct(explicit: ProductAsset | undefined, products: ProductAsset[], parents: GenerationRecord[]) {
  const choices = [...products, ...parents.flatMap(p => p.context.product ? [p.context.product] : [])]
  const ids = new Set([...choices.map(p => p.id), ...(explicit ? [explicit.id] : [])])
  if (ids.size > 1) throw new Error('参考图属于不同产品，请断开冲突连线后选择同一个产品')
  return explicit || choices[0] || null
}
export function buildGenerationContext(product: ProductAsset | null, references: ProductReference[], prompt: string, outputOptions: GenerationContext['outputOptions']): GenerationContext {
  if (references.length > 8) throw new Error('参考图最多支持 8 张，请删除多余图片')
  return JSON.parse(JSON.stringify({ schemaVersion: 1, product, references, constraints: product?.constraints || emptyConstraints(), prompt, outputOptions }))
}
export function createGenerationRecord(id: string, context: GenerationContext, parentIds: string[]): GenerationRecord {
  const parentGenerationIds = [...new Set(parentIds)]
  return {
    id, parentGenerationId: parentGenerationIds[0] || null, parentGenerationIds,
    rootProductAssetId: context.product?.id || null, referenceIds: context.references.map(r => r.id),
    prompt: context.prompt, model: context.outputOptions.model, aspectRatio: context.outputOptions.aspectRatio,
    generationType: context.outputOptions.generationType, createdAt: new Date().toISOString(),
    context: JSON.parse(JSON.stringify(context)), status: 'running', outputAssetIds: [],
  }
}
export const continuationActions: { type: GenerationType; label: string; prompt: string }[] = [
  { type: 'custom', label: '继续生成', prompt: '保持产品一致性，继续生成家具电商图片。' },
  { type: 'scene', label: '换场景', prompt: '保持产品一致性，将产品放入客厅场景。' },
  { type: 'angle', label: '换角度', prompt: '保持产品一致性，展示产品的45°视角。' },
  { type: 'detail', label: '做细节', prompt: '保持产品一致性，展示产品细节特写。' },
  { type: 'sellingPoint', label: '做卖点', prompt: '保持产品一致性，仅根据已填写的产品事实展示卖点，不添加未经确认的声明。' },
]
