import { describe, expect, it } from 'vitest'
import { emptyConstraints, emptyMetadata, type ProductAsset, type ProductReference } from '../types/product'
import { constraintsFromMetadata } from './productConstraints'
import { buildGenerationContext, createGenerationRecord, inheritedReferences, mergeReferences, resolveProduct } from './generationContext'
import { createProductRepository } from '../repositories/productRepository'

// User-supplied acceptance fixture, not a claim inferred from the placeholder image.
function televisionCabinet(): ProductAsset {
  const metadata = { ...emptyMetadata(), category: '电视柜', dimensions: { unit: 'mm' as const, width: 2000, depth: 400, height: 250, panelThickness: 20 }, drawerCount: 4, materials: ['中古胡桃', '罗马洞石', '亮光黑'] }
  const constraints = constraintsFromMetadata(metadata, { ...emptyConstraints(), structureLock: true, materialLock: true, textureLock: true, proportionLock: true, hardwareLock: true, customRules: ['保持4格抽屉', '不得改变木纹方向、五金位置、插座结构'] })
  return { id: 'product_yanyu', name: '洞石砚屿电视柜', coverImageId: 'ref_white', referenceIds: ['ref_white'], metadata, constraints, createdAt: '2026-09-06T00:00:00Z', updatedAt: '2026-09-06T00:00:00Z' }
}
const white: ProductReference = { id: 'ref_white', assetId: 'white', url: '/uploads/white.png', role: 'product', origin: 'uploaded' }

describe('Product → References → Constraints → Generations → Lineage', () => {
  it('retains the television cabinet and locks for ten generations without accumulating output references', () => {
    const product = televisionCabinet()
    let parent = createGenerationRecord('generation_1', buildGenerationContext(product, [white], '客厅场景', { model: 'fast', aspectRatio: '3:4', imageSize: '1K', count: 1, generationType: 'scene' }), [])
    for (let index = 2; index <= 10; index++) {
      const output: ProductReference = { id: `ref_${parent.id}`, url: `/uploads/${parent.id}.png`, role: 'composition', origin: 'generated', generationId: parent.id }
      const references = mergeReferences(inheritedReferences([parent]), [output])
      const resolved = resolveProduct(undefined, [], [parent])
      const next = createGenerationRecord(`generation_${index}`, buildGenerationContext(resolved, references, index === 2 ? '45°视角' : '产品细节', { ...parent.context.outputOptions, generationType: index === 2 ? 'angle' : 'detail' }), [parent.id])
      expect(next.parentGenerationId).toBe(parent.id)
      expect(next.rootProductAssetId).toBe(product.id)
      expect(next.context.product).toEqual(product)
      expect(next.context.constraints).toEqual(product.constraints)
      expect(next.context.constraints.lockedFields).toMatchObject({ drawerCount: 4, width: 2000, height: 250, depth: 400, panelThickness: 20, materials: ['中古胡桃', '罗马洞石', '亮光黑'] })
      expect(next.referenceIds).toHaveLength(2)
      expect(next.context.references[0].role).toBe('product')
      parent = next
    }
    product.constraints.lockedFields.drawerCount = 8
    expect(parent.context.constraints.lockedFields.drawerCount).toBe(4)
  })

  it('round-trips products, roles, locks and lineage through the replaceable repository', () => {
    let raw = ''
    const storage = { getItem: () => raw || null, setItem: (_key: string, value: string) => { raw = value } }
    const repository = createProductRepository(storage)
    const database = repository.load()
    const product = televisionCabinet()
    database.products[product.id] = product
    database.references[white.id] = white
    const generation = createGenerationRecord('g1', buildGenerationContext(product, [white], '客厅', { model: 'fast', aspectRatio: '1:1', imageSize: '1K', count: 1, generationType: 'scene' }), [])
    database.generations[generation.id] = generation
    repository.save(database)
    expect(createProductRepository(storage).load()).toEqual(database)
    raw = '{broken'
    expect(() => repository.load()).toThrow()
  })

  it('blocks ambiguous product ancestry instead of silently choosing the first product', () => {
    const product = televisionCabinet()
    expect(() => resolveProduct(product, [{ ...product, id: 'another_product' }], [])).toThrow('不同产品')
  })

  it('keeps unknown dimensions unknown and removes controlled fields when a lock is disabled', () => {
    const product = televisionCabinet()
    const constraints = constraintsFromMetadata(emptyMetadata(), { ...product.constraints, proportionLock: false })
    expect(constraints.lockedFields.width).toBeUndefined()
    expect(constraints.lockedFields.drawerCount).toBeUndefined()
    expect(constraints.textureLock).toBe(true)
  })
})
