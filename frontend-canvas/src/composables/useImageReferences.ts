import { computed, ref, watch } from 'vue'
import type { ResolvedNodeInput } from '../utils/nodeInputResolver'

export function useImageReferences(getInputs: () => ResolvedNodeInput[], getNode: () => { id: string; content: string } | null) {
  const excluded = ref<string[]>([])
  watch(() => getNode()?.id, () => {
    try { excluded.value = JSON.parse(getNode()?.content || '{}').input?.excluded_reference_keys || [] }
    catch { excluded.value = [] }
  }, { immediate: true })

  const candidates = computed(() => getInputs().flatMap(input => input.images.map(image => ({
    ...image,
    id: `${input.edgeId}:${image.url}`,
    edgeId: input.edgeId,
    label: `${image.origin === 'generated' ? 'AI生成' : image.origin === 'uploaded' ? '用户上传' : '参考图片'} · ${image.name || '图片'}`,
  }))))
  const images = computed(() => {
    const seen = new Set<string>()
    return candidates.value.filter(image => {
      if (excluded.value.includes(image.id) || seen.has(image.url)) return false
      seen.add(image.url)
      return true
    })
  })
  function remove(id: string) {
    const image = candidates.value.find(item => item.id === id)
    if (!image) return
    // A generation node may expose a batch. Remove only this image, including
    // duplicate paths to the same asset; preserve the other images on its edge.
    excluded.value = [...new Set([...excluded.value, ...candidates.value.filter(item => item.url === image.url).map(item => item.id)])]
  }
  return { images, excluded, remove }
}
