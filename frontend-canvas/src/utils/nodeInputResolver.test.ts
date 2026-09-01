import { describe, expect, it } from 'vitest'
import type { Edge, Node } from '../types'
import { resolveNodeInputs } from './nodeInputResolver'

function node(id: string, nodeType: Node['node_type'], content: any): Node {
  return {
    id, canvas_id: 'cv_test', node_type: nodeType,
    x: 0, y: 0, width: 100, height: 100,
    content: typeof content === 'string' ? content : JSON.stringify(content),
    config: '{}', created_at: '', updated_at: '',
  }
}

function edge(id: string, source: string, target: string): Edge {
  return { id, canvas_id: 'cv_test', source_node_id: source, target_node_id: target, created_at: '' }
}

describe('resolveNodeInputs', () => {
  it('resolves multiple incoming asset images in edge order', () => {
    const nodes = [
      node('A', 'asset', { asset_id: 'asset_a', url: '/uploads/a.png' }),
      node('B', 'asset', { asset_id: 'asset_b', url: '/uploads/b.png' }),
      node('C', 'image', {}),
    ]
    const resolved = resolveNodeInputs(nodes, [edge('A_C', 'A', 'C'), edge('B_C', 'B', 'C')])
    expect(resolved.C.flatMap(input => input.images.map(image => image.url))).toEqual(['/uploads/a.png', '/uploads/b.png'])
  })

  it('removes an input immediately when its edge is deleted', () => {
    const nodes = [node('A', 'asset', { url: '/uploads/a.png' }), node('B', 'asset', { url: '/uploads/b.png' }), node('C', 'image', {})]
    const resolved = resolveNodeInputs(nodes, [edge('B_C', 'B', 'C')])
    expect(resolved.C.flatMap(input => input.images.map(image => image.url))).toEqual(['/uploads/b.png'])
  })

  it('recomputes the current image output instead of caching by edge id', () => {
    const connection = [edge('B_C', 'B', 'C')]
    const first = resolveNodeInputs([
      node('B', 'image', { generated_images: [{ url: '/uploads/b1.png' }] }), node('C', 'image', {}),
    ], connection)
    const second = resolveNodeInputs([
      node('B', 'image', { generated_images: [{ url: '/uploads/b2.png' }] }), node('C', 'image', {}),
    ], connection)
    expect(first.C[0].images[0].url).toBe('/uploads/b1.png')
    expect(second.C[0].images[0].url).toBe('/uploads/b2.png')
  })

  it('preserves all outputs from the latest generation batch', () => {
    const resolved = resolveNodeInputs([
      node('B', 'image', { generated_images: [{ url: '/uploads/b1.png' }, { url: '/uploads/b2.png' }] }), node('C', 'image', {}),
    ], [edge('B_C', 'B', 'C')])
    expect(resolved.C[0].images.map(image => image.url)).toEqual(['/uploads/b1.png', '/uploads/b2.png'])
  })
})
