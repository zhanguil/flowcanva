import type { Node } from '../types'
import type { GenerationType } from '../types/product'
import { continuationActions, readContent } from './generationContext'

export function continuationContent(source: Node | undefined, type: GenerationType = 'custom') {
  const data = readContent(source?.content)
  const generation = data.generation || data.generated_images?.find((image: any) => image.generation)?.generation
  const product = data.product_snapshot || generation?.context.product
  return JSON.stringify({
    prompt: continuationActions.find(action => action.type === type)?.prompt || '',
    generation_type: type,
    ...(product ? { product_asset_id: product.id, product_snapshot: product } : {}),
    // The edge resolver supplies the immutable parent record and related references.
  })
}
