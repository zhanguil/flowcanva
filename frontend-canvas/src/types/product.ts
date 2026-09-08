export const referenceRoles = [
  ['product', '产品'], ['structure', '结构'], ['material', '材质'], ['scene', '场景'],
  ['composition', '构图'], ['lighting', '光照'], ['style', '风格'], ['hardware', '五金'], ['detail', '细节'],
] as const
export type ReferenceRole = typeof referenceRoles[number][0]
export const generationTypes = [
  ['hero', '主图'], ['scene', '场景'], ['angle', '角度'], ['detail', '细节'],
  ['material', '材质'], ['structure', '结构'], ['sellingPoint', '卖点'], ['custom', '自定义'],
] as const
export type GenerationType = typeof generationTypes[number][0]
export const lockOptions = [
  ['structureLock', '结构'], ['materialLock', '材质'], ['textureLock', '木纹'],
  ['proportionLock', '比例'], ['hardwareLock', '五金'],
] as const
export interface ProductConstraint {
  structureLock: boolean
  materialLock: boolean
  textureLock: boolean
  proportionLock: boolean
  hardwareLock: boolean
  lockedFields: Record<string, string | number | boolean | string[]>
  customRules: string[]
}
export interface ProductMetadata {
  category: string
  dimensions: { unit: 'mm'; width: number | null; height: number | null; depth: number | null; panelThickness: number | null }
  materials: string[]
  drawerCount: number | null
  hardware: string
  textureDirection: string
  notes: string
}
export interface ProductReference {
  id: string
  assetId?: string
  url: string
  name?: string
  role: ReferenceRole
  origin?: 'uploaded' | 'generated'
  generationId?: string
}
export interface ProductAsset {
  id: string
  name: string
  coverImageId: string
  referenceIds: string[]
  constraints: ProductConstraint
  metadata: ProductMetadata
  createdAt: string
  updatedAt: string
}
export interface GenerationContext {
  schemaVersion: 1
  product: ProductAsset | null
  references: ProductReference[]
  constraints: ProductConstraint
  prompt: string
  outputOptions: { model: string; aspectRatio: string; imageSize: string; count: number; generationType: GenerationType }
}
export interface GenerationRecord {
  id: string
  parentGenerationId: string | null
  parentGenerationIds: string[]
  rootProductAssetId: string | null
  referenceIds: string[]
  prompt: string
  model: string
  aspectRatio: string
  generationType: GenerationType
  createdAt: string
  context: GenerationContext
  status: 'running' | 'succeeded' | 'failed' | 'cancelled'
  outputAssetIds: string[]
  error?: string
}
export function emptyConstraints(): ProductConstraint {
  return { structureLock: false, materialLock: false, textureLock: false, proportionLock: false, hardwareLock: false, lockedFields: {}, customRules: [] }
}
export function emptyMetadata(): ProductMetadata {
  return { category: '', dimensions: { unit: 'mm', width: null, height: null, depth: null, panelThickness: null }, materials: [], drawerCount: null, hardware: '', textureDirection: '', notes: '' }
}
