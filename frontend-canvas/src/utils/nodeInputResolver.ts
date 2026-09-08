import type { Edge, Node } from '../types'
import type { ProductAsset, GenerationRecord, ReferenceRole } from '../types/product'

export interface ResolvedNodeImage {
  assetId?: string
  name?: string
  url: string
  origin?: 'uploaded' | 'generated'
  product?: ProductAsset
  generation?: GenerationRecord
  role?: ReferenceRole
}

export interface ResolvedNodeInput {
  edgeId: string
  sourceNodeId: string
  sourceNodeType: string
  data: any
  images: ResolvedNodeImage[]
  texts: string[]
  assets: string[]
}

export function resolveNodeInputs(nodes: Node[], edges: Edge[]): Record<string, ResolvedNodeInput[]> {
  const nodesById = new Map(nodes.map(node => [node.id, node]))
  const result: Record<string, ResolvedNodeInput[]> = {}

  for (const edge of edges) {
    const source = nodesById.get(edge.source_node_id)
    if (!source) continue
    const output = resolveNodeOutput(source)
    if (!result[edge.target_node_id]) result[edge.target_node_id] = []
    result[edge.target_node_id].push({
      edgeId: edge.id,
      sourceNodeId: source.id,
      sourceNodeType: source.node_type,
      data: output.data,
      images: output.images,
      texts: output.texts,
      assets: output.assets,
    })
  }
  return result
}

export function resolveNodeOutput(node: Node): { data: any; images: ResolvedNodeImage[]; texts: string[]; assets: string[] } {
  const empty = { data: null as any, images: [] as ResolvedNodeImage[], texts: [] as string[], assets: [] as string[] }
  if (!node.content) return empty

  if (node.node_type === 'text') {
    return { ...empty, data: node.content, texts: [node.content] }
  }

  let content: any
  try {
    content = JSON.parse(node.content)
  } catch {
    return { ...empty, data: node.content }
  }

  if (node.node_type === 'asset') {
    if (content.mediaType === 'video' || content.mediaType === 'audio' || /^(video|audio)\//.test(content.mime_type || '')) return empty
    const url = content.url || content.dataUrl || ''
    const image: ResolvedNodeImage | null = url ? {
      assetId: content.asset_id, name: content.name, url, origin: content.parent_generation_node_id ? 'generated' : content.origin,
      product: content.product_snapshot || content.generation?.context?.product,
      generation: content.generation, role: content.reference_role,
    } : null
    return {
      ...empty,
      data: content,
      images: image ? [image] : [],
      assets: image?.assetId ? [image.assetId] : [],
    }
  }

  if (node.node_type === 'image') {
    const generated = Array.isArray(content.generated_images) ? content.generated_images : []
    const images = generated
      .map((item: any) => ({ assetId: item?.asset_id, name: item?.name, url: item?.url || '', origin: 'generated' as const,
        generation: item?.generation, product: item?.generation?.context?.product,
      }))
      .filter((item: ResolvedNodeImage) => Boolean(item.url))
    return {
      ...empty,
      data: images.length > 0 ? { dataUrl: images[0].url, images } : null,
      images,
      assets: images.flatMap((image: ResolvedNodeImage) => image.assetId ? [image.assetId] : []),
    }
  }

  return { ...empty, data: content }
}
