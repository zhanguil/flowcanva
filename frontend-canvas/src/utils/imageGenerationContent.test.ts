import { describe, expect, it } from 'vitest'
import { serializeGeneratedNodeContent } from './imageGenerationContent'

const asset = (id: string, url = `/uploads/${id}.png`) => ({
  id,
  filename: `${id}.png`,
  url,
  size: 12,
  mime_type: 'image/png',
  width: 100,
  height: 80,
})

describe('serializeGeneratedNodeContent', () => {
  it('replaces the latest batch and synchronizes derived output IDs', () => {
    const result = JSON.parse(serializeGeneratedNodeContent(JSON.stringify({
      prompt: 'keep this prompt',
      input: { reference_asset_ids: ['ref-1'] },
      output: { generated_asset_ids: ['stale-id'], provider: 'mock' },
      generated_images: [asset('stale-id')],
    }), [asset('fresh-a'), asset('fresh-b')]))

    expect(result.prompt).toBe('keep this prompt')
    expect(result.input).toEqual({ reference_asset_ids: ['ref-1'] })
    expect(result.output).toEqual({
      generated_asset_ids: ['fresh-a', 'fresh-b'],
      provider: 'mock',
    })
    expect(result.generated_images.map((item: any) => item.asset_id)).toEqual(['fresh-a', 'fresh-b'])
  })

  it('writes a valid empty batch when the provider returns no prior output', () => {
    const result = JSON.parse(serializeGeneratedNodeContent('', []))
    expect(result.generated_images).toEqual([])
    expect(result.output.generated_asset_ids).toEqual([])
  })
})
