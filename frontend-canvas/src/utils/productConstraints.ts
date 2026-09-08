import type { ProductConstraint, ProductMetadata } from '../types/product'

export function constraintsFromMetadata(metadata: ProductMetadata, locks: ProductConstraint): ProductConstraint {
  const lockedFields = { ...locks.lockedFields }
  const fields = [
    ['drawerCount', metadata.drawerCount, locks.structureLock],
    ['panelThickness', metadata.dimensions.panelThickness, locks.structureLock],
    ['width', metadata.dimensions.width, locks.proportionLock],
    ['height', metadata.dimensions.height, locks.proportionLock],
    ['depth', metadata.dimensions.depth, locks.proportionLock],
    ['materials', metadata.materials, locks.materialLock],
    ['textureDirection', metadata.textureDirection, locks.textureLock],
    ['hardware', metadata.hardware, locks.hardwareLock],
  ] as const
  for (const [key, value, locked] of fields) {
    delete lockedFields[key]
    if (locked && value !== null && value !== '' && (!Array.isArray(value) || value.length)) lockedFields[key] = value
  }
  return { ...locks, lockedFields, customRules: locks.customRules.map(rule => rule.trim()).filter(Boolean) }
}
