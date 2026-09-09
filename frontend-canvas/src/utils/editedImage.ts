import type { Asset } from '../types'
import { readContent } from './generationContext'
export function editedImageContent(content: string, asset: Pick<Asset, 'id' | 'url' | 'filename' | 'width' | 'height' | 'size' | 'mime_type'>) {
  const original = readContent(content)
  return JSON.stringify({ ...original, asset_id: asset.id, url: asset.url, name: asset.filename,
    width: asset.width, height: asset.height, size: asset.size, mime_type: asset.mime_type, mediaType: 'image',
    edit: { sourceAssetId: original.asset_id || null, sourceUrl: original.url || null, editedAt: new Date().toISOString(), kind: 'manual' },
  })
}
