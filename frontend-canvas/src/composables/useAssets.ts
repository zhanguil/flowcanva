import { ref } from 'vue'
import type { Asset } from '../types'
import { fetchAssets, uploadAsset, updateAsset, deleteAsset } from '../api'

export const ASSET_CATEGORIES = ['全部', '人物', '场景', '物品', '视频', '音频', '风格', '其他'] as const
export type AssetCategory = typeof ASSET_CATEGORIES[number]

const assets = ref<Asset[]>([])
const loaded = ref(false)

async function loadAssets(force = false) {
  if (loaded.value && !force) return
  try {
    assets.value = await fetchAssets()
    loaded.value = true
  } catch (e) {
    console.error('load assets failed', e)
  }
}

async function addAsset(file: File): Promise<Asset | null> {
  try {
    const a = await uploadAsset(file)
    assets.value.unshift(a)
    return a
  } catch (e) {
    console.error('upload asset failed', e)
    return null
  }
}

async function setCategory(id: string, category: string) {
  await updateAsset(id, { category })
  const a = assets.value.find(x => x.id === id)
  if (a) a.category = category
}

async function removeAsset(id: string) {
  await deleteAsset(id)
  assets.value = assets.value.filter(x => x.id !== id)
}

export function useAssets() {
  return { ASSET_CATEGORIES, assets, loadAssets, addAsset, setCategory, removeAsset }
}
