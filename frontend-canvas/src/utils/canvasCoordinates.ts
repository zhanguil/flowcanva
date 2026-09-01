import type { ViewportState } from '../types'

export interface CanvasPoint {
  wx: number
  wy: number
}

export function screenToCanvasPoint(viewport: ViewportState, screenX: number, screenY: number): CanvasPoint {
  return {
    wx: (screenX - viewport.ox) / viewport.zoom,
    wy: (screenY - viewport.oy) / viewport.zoom,
  }
}

export function staggerCanvasPoint(origin: CanvasPoint, index: number): CanvasPoint {
  const columns = 3
  const horizontalGap = 360
  const verticalGap = 340
  return {
    wx: origin.wx + (index % columns) * horizontalGap,
    wy: origin.wy + Math.floor(index / columns) * verticalGap,
  }
}

export function isSupportedCanvasImage(file: Pick<File, 'name' | 'type'>): boolean {
  const mime = file.type.toLowerCase()
  if (mime === 'image/png' || mime === 'image/jpeg' || mime === 'image/webp') return true
  return /\.(png|jpe?g|webp)$/i.test(file.name)
}
