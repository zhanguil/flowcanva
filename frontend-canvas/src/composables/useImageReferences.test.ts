import { effectScope, nextTick, ref } from 'vue'
import { describe, expect, it } from 'vitest'
import { useImageReferences } from './useImageReferences'
import type { ResolvedNodeInput } from '../utils/nodeInputResolver'

describe('reference selection', () => {
  it('removes one image from a batch, deduplicates paths and accepts the next output', async () => {
    const inputs = ref<ResolvedNodeInput[]>([
      { edgeId: 'batch', sourceNodeId: 'a', sourceNodeType: 'image', data: null, texts: [], assets: [], images: [{ url: '/a.png' }, { url: '/b.png' }] },
      { edgeId: 'asset', sourceNodeId: 'b', sourceNodeType: 'asset', data: null, texts: [], assets: [], images: [{ url: '/a.png' }] },
    ])
    const node = ref({ id: 'target', content: '{}' })
    const scope = effectScope()
    const refs = scope.run(() => useImageReferences(() => inputs.value, () => node.value))!
    expect(refs.images.value.map(i => i.url)).toEqual(['/a.png', '/b.png'])
    refs.remove('batch:/a.png')
    expect(refs.images.value.map(i => i.url)).toEqual(['/b.png'])
    inputs.value[0].images = [{ url: '/new.png' }]
    expect(refs.images.value.map(i => i.url)).toEqual(['/new.png'])
    node.value = { id: 'another', content: '{}' }
    await nextTick()
    expect(refs.images.value.map(i => i.url)).toEqual(['/new.png', '/a.png'])
    scope.stop()
  })
})
