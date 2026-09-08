import { computed, ref, watch } from 'vue'
import type { ResolvedNodeInput } from '../utils/nodeInputResolver'
import type { ProductReference, ReferenceRole } from '../types/product'
import { readContent, referenceId } from '../utils/generationContext'

export function useImageReferences(getInputs: () => ResolvedNodeInput[], getNode: () => { id: string; content: string } | null, getInherited: () => ProductReference[] = () => []) {
  const excluded = ref<string[]>([])
  const roles = ref<Record<string, ReferenceRole>>({})
  watch(() => getNode()?.id, () => {
    try { excluded.value = JSON.parse(getNode()?.content || '{}').input?.excluded_reference_keys || [] }
    catch { excluded.value = [] }
    roles.value = readContent(getNode()?.content).input?.reference_roles || {}
  }, { immediate: true })

  const candidates = computed(() => getInputs().flatMap(input => input.images.map(image => ({
    ...image,
    id: `${input.edgeId}:${image.url}`,
    edgeId: input.edgeId,
    referenceId: referenceId(image),
    role: roles.value[referenceId(image)] || image.role || (image.generation ? 'scene' : 'product') as ReferenceRole,
    generationId: image.generation?.id,
    label: `${image.origin === 'generated' ? 'AI生成' : image.origin === 'uploaded' ? '用户上传' : '参考图片'} · ${image.name || '图片'}`,
  }))).concat(getInherited().map(image => ({
    ...image, id: `inherited:${image.id}`, edgeId: '', referenceId: image.id,
    generationId: image.generationId,
    role: roles.value[image.id] || image.role,
    label: `${image.origin === 'generated' ? 'AI生成' : '产品参考'} · ${image.name || '图片'}`,
  }))))
  const images = computed(() => {
    const seen = new Set<string>()
    return candidates.value.filter(image => {
      if (excluded.value.includes(image.id) || excluded.value.includes(`reference:${image.referenceId}`) || seen.has(image.url)) return false
      seen.add(image.url)
      return true
    })
  })
  function remove(id: string) {
    const image = candidates.value.find(item => item.id === id)
    if (!image) return
    // A generation node may expose a batch. Remove only this image, including
    // duplicate paths to the same asset; preserve the other images on its edge.
    excluded.value = [...new Set([...excluded.value, `reference:${image.referenceId}`, ...candidates.value.filter(item => item.url === image.url).map(item => item.id)])]
  }
  function setRole(id: string, role: ReferenceRole) {
    const image = candidates.value.find(item => item.id === id)
    if (image) roles.value = { ...roles.value, [image.referenceId]: role }
  }
  const references = computed<ProductReference[]>(() => images.value.map(image => ({
    id: image.referenceId, assetId: image.assetId, url: image.url, name: image.name, role: image.role,
    origin: image.origin, generationId: image.generationId,
  })))
  return { images, excluded, remove, roles, setRole, references }
}
