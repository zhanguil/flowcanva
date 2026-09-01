import { describe, expect, it } from 'vitest'
import { isSupportedCanvasImage, screenToCanvasPoint, staggerCanvasPoint } from './canvasCoordinates'

describe('canvas drop coordinates', () => {
  it('converts screen coordinates after pan and zoom', () => {
    expect(screenToCanvasPoint({ ox: -240, oy: 120, zoom: 2 }, 560, 520)).toEqual({ wx: 400, wy: 200 })
  })

  it('staggering multiple images never returns identical positions', () => {
    const points = Array.from({ length: 5 }, (_, index) => staggerCanvasPoint({ wx: 100, wy: 200 }, index))
    expect(new Set(points.map(point => `${point.wx}:${point.wy}`)).size).toBe(5)
  })

  it('accepts only PNG, JPEG and WEBP images', () => {
    expect(isSupportedCanvasImage({ name: 'product.png', type: 'image/png' })).toBe(true)
    expect(isSupportedCanvasImage({ name: 'scene.JPG', type: '' })).toBe(true)
    expect(isSupportedCanvasImage({ name: 'reference.webp', type: 'image/webp' })).toBe(true)
    expect(isSupportedCanvasImage({ name: 'clip.mp4', type: 'video/mp4' })).toBe(false)
  })
})
