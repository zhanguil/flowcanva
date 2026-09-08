import type { GenerationRecord } from '../types/product'

export interface GeneratedImageAssetLike {
  id: string
  filename: string
  url: string
  size: number
  mime_type: string
  width: number
  height: number
  generation?: GenerationRecord
}

function isRecord(value: unknown): value is Record<string, any> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

/**
 * Replace the image node's latest output batch while preserving unrelated node
 * fields. The derived output IDs must stay in sync with generated_images so
 * downstream consumers and persisted content describe the same batch.
 */
export function serializeGeneratedNodeContent(
  currentContent: string | null | undefined,
  generatedAssets: readonly GeneratedImageAssetLike[],
) {
  let current: Record<string, any> = {}
  try {
    const parsed = JSON.parse(currentContent || '{}')
    if (isRecord(parsed)) current = parsed
  } catch {
    // Keep malformed legacy content replaceable instead of throwing in the
    // completion handler.
  }

  const generatedImages = generatedAssets.map(asset => ({
    id: asset.id,
    asset_id: asset.id,
    name: asset.filename,
    url: asset.url,
    size: asset.size,
    width: asset.width,
    height: asset.height,
    mime_type: asset.mime_type,
    ...(asset.generation ? { generation: asset.generation } : {}),
  }))
  const existingOutput = isRecord(current.output) ? current.output : {}

  return JSON.stringify({
    ...current,
    generated_images: generatedImages,
    output: {
      ...existingOutput,
      generated_asset_ids: generatedImages.map(asset => asset.asset_id),
    },
  })
}
