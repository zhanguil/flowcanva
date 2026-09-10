<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ReferenceGrid from '../components/product-pack/ReferenceGrid.vue'
import ResultGrid from '../components/product-pack/ResultGrid.vue'
import { analyzeProductReferences, cancelGenerationJob, createRecipeRun, fetchAssets, fetchGenerationProviders, fetchProject, fetchProjectRecipeRuns, fetchRecipeRun, fetchRecipes, retryGenerationJob, saveProductPack, uploadAsset } from '../api'
import type { Asset, GenerationProviderInfo, ProductPack, Recipe, RecipeOutput, RecipeRun, StudioProjectDetail } from '../types'

const route = useRoute()
const router = useRouter()
const projectId = String(route.params.id)
const project = ref<StudioProjectDetail | null>(null)
const pack = ref<ProductPack | null>(null)
const recipes = ref<Recipe[]>([])
const selectedRecipeId = ref('furniture_sku_main_images')
const selectedProviderId = ref('legacy')
const providers = ref<GenerationProviderInfo[]>([])
const selectedSKUIds = ref<string[]>([])
const selectedOutputKeys = ref<string[]>([])
const copiesPerItem = ref(1)
const expandedSKUId = ref('')
const structuresJSON = ref('{}')
const materialsJSON = ref('{}')
const forbiddenText = ref('')
const allowedText = ref('')
const newStructureTag = ref('')
const newMaterialTag = ref('')
const newForbiddenTag = ref('')
const newAllowedTag = ref('')
const run = ref<RecipeRun | null>(null)
const assets = ref<Asset[]>([])
const busy = ref(false)
const analyzing = ref(false)
const modelSettingsOpen = ref(false)
const errorMessage = ref('')
const technicalError = ref('')
const savedMessage = ref('')
let pollTimer: number | undefined

const selectedRecipe = computed(() => recipes.value.find(item => item.id === selectedRecipeId.value))
const selectedSKUCount = computed(() => selectedSKUIds.value.length)
const selectedOutputs = computed(() => selectedRecipe.value?.outputs.filter(output => selectedOutputKeys.value.includes(outputKey(output))) || [])
const estimatedJobs = computed(() => selectedSKUCount.value * selectedOutputs.value.length * copiesPerItem.value)
const estimatedCost = computed(() => estimatedJobs.value * (selectedRecipe.value?.estimated_cost_per_job || 0))
const runActive = computed(() => ['queued', 'running'].includes(run.value?.status || ''))
const currentProvider = computed(() => providers.value.find(provider => provider.id === selectedProviderId.value))
const currentModelLabel = computed(() => selectedProviderId.value === 'legacy' ? '默认生图模型' : (currentProvider.value?.name || '当前生图模型'))
const currencyLabel = computed(() => selectedRecipe.value?.currency === 'USD' ? '美元' : (selectedRecipe.value?.currency || ''))
const structureEntries = computed(() => Object.entries(pack.value?.product_dna.structuralFeatures || {}))
const materialEntries = computed(() => Object.entries(pack.value?.product_dna.materials || {}))
const referenceImages = computed(() => {
  const byId = new Map(assets.value.map(asset => [asset.id, asset]))
  return (pack.value?.reference_pack.references || []).map(reference => byId.get(reference.asset_id)?.url).filter((url): url is string => Boolean(url)).slice(0, 8)
})
const generationSummary = computed(() => {
  const base = `${selectedSKUCount.value} 个规格 × ${selectedOutputs.value.length} 个画幅`
  return copiesPerItem.value > 1 ? `${base} × ${copiesPerItem.value} 张 = ${estimatedJobs.value} 张` : `${base} = ${estimatedJobs.value} 张`
})
const runStatusLabel = computed(() => ({ queued: '排队中', running: '生成中', completed: '已完成', failed: '部分失败', cancelled: '已取消' }[run.value?.status || ''] || run.value?.status || ''))

function outputKey(output: RecipeOutput) { return `${output.outputType}::${output.aspectRatio}` }

function syncAdvancedTextFromPack() {
  if (!pack.value) return
  const dna = pack.value.product_dna
  structuresJSON.value = JSON.stringify(dna.structuralFeatures || {}, null, 2)
  materialsJSON.value = JSON.stringify(dna.materials || {}, null, 2)
  forbiddenText.value = (dna.forbiddenChanges || []).join('\n')
  allowedText.value = (dna.allowedChanges || []).join('\n')
}

function setPack(next: ProductPack, preserveSelection = false) {
  const previousSelection = new Set(selectedSKUIds.value)
  pack.value = structuredClone(next)
  syncAdvancedTextFromPack()
  selectedSKUIds.value = pack.value.skus.map(sku => sku.id).filter((id): id is string => typeof id === 'string' && id.length > 0 && (!preserveSelection || previousSelection.has(id)))
}

function clearMessages() { errorMessage.value = ''; technicalError.value = ''; savedMessage.value = '' }
function showError(message: string, error: unknown) { errorMessage.value = message; technicalError.value = error instanceof Error ? error.message : String(error) }

async function refreshAssets() {
  const response = await fetchAssets({ project_id: projectId, page_size: 100 })
  assets.value = response.items
}

async function load() {
  clearMessages()
  try {
    const [detail, recipeItems, providerItems, assetItems, history] = await Promise.all([
      fetchProject(projectId), fetchRecipes(), fetchGenerationProviders(), fetchAssets({ project_id: projectId, page_size: 100 }), fetchProjectRecipeRuns(projectId),
    ])
    project.value = detail
    recipes.value = recipeItems
    providers.value = providerItems
    assets.value = assetItems.items
    run.value = history[0] || null
    if (detail.product_pack) setPack(detail.product_pack)
    if (!recipes.value.some(item => item.id === selectedRecipeId.value) && recipes.value[0]) selectedRecipeId.value = recipes.value[0].id
    selectedOutputKeys.value = selectedRecipe.value?.outputs.map(outputKey) || []
    if (runActive.value) schedulePoll()
  } catch (error) { showError('项目暂时无法读取，请稍后重试。', error) }
}

function addSKU() {
  if (!pack.value) return
  const id = `sku_${crypto.randomUUID().slice(0, 8)}`
  pack.value.skus.push({ id, name: '', label: '', width: null, height: null, depth: null, reference_asset_ids: [] })
  selectedSKUIds.value.push(id)
  expandedSKUId.value = id
}

function removeSKU(index: number) {
  const id = pack.value?.skus[index]?.id
  if (id) selectedSKUIds.value = selectedSKUIds.value.filter(item => item !== id)
  pack.value?.skus.splice(index, 1)
}

function skuDisplayName(name: string, label: string) {
  const numeric = Number(name)
  if (Number.isFinite(numeric) && numeric >= 1000) return `${numeric / 10}cm`
  return label || name || '新规格'
}

function addReference() { pack.value?.reference_pack.references.push({ asset_id: '', role: 'product_main', weight: 1, locked: true }) }

async function uploadReferences(payload: { files: File[]; role: string }) {
  if (!pack.value || payload.files.length === 0) return
  busy.value = true; clearMessages()
  try {
    for (const file of payload.files) {
      const asset = await uploadAsset(file, { project_id: projectId, source_type: 'upload', role: payload.role, created_by: project.value?.created_by || 'member' })
      assets.value.unshift(asset)
      if (!pack.value.reference_pack.references.some(reference => reference.asset_id === asset.id)) pack.value.reference_pack.references.push({ asset_id: asset.id, role: payload.role, weight: 1, locked: true })
    }
    await persistPack(true); savedMessage.value = '参考图已加入项目'
  } catch (error) { showError('参考图上传失败，请检查图片后重试。', error) } finally { busy.value = false }
}

async function replaceReference(payload: { index: number; file: File }) {
  if (!pack.value) return
  busy.value = true; clearMessages()
  try {
    const reference = pack.value.reference_pack.references[payload.index]
    const asset = await uploadAsset(payload.file, { project_id: projectId, source_type: 'upload', role: reference.role, created_by: project.value?.created_by || 'member' })
    assets.value.unshift(asset); reference.asset_id = asset.id
    await persistPack(true); savedMessage.value = '参考图已替换'
  } catch (error) { showError('参考图替换失败，请稍后重试。', error) } finally { busy.value = false }
}

async function persistPack(silent = false) {
  if (!pack.value) return false
  try {
    pack.value.product_dna.structuralFeatures = JSON.parse(structuresJSON.value || '{}')
    pack.value.product_dna.materials = JSON.parse(materialsJSON.value || '{}')
    pack.value.product_dna.forbiddenChanges = forbiddenText.value.split('\n').map(value => value.trim()).filter(Boolean)
    pack.value.product_dna.allowedChanges = allowedText.value.split('\n').map(value => value.trim()).filter(Boolean)
    const saved = await saveProductPack(projectId, pack.value)
    setPack(saved, true)
    if (!silent) savedMessage.value = '产品资料已保存'
    return true
  } catch (error) { showError('产品资料保存失败，请检查高级设置。', error); return false }
}

async function save() { if (busy.value) return; busy.value = true; clearMessages(); await persistPack(); busy.value = false }

function addObjectTag(target: 'structure' | 'material') {
  if (!pack.value) return
  const input = target === 'structure' ? newStructureTag : newMaterialTag
  const value = input.value.trim(); if (!value) return
  if (target === 'structure') pack.value.product_dna.structuralFeatures[`feature_${Date.now()}`] = value
  else pack.value.product_dna.materials[`material_${Date.now()}`] = value
  input.value = ''; syncAdvancedTextFromPack()
}

function removeObjectTag(target: 'structure' | 'material', key: string) {
  if (!pack.value) return
  if (target === 'structure') delete pack.value.product_dna.structuralFeatures[key]
  else delete pack.value.product_dna.materials[key]
  syncAdvancedTextFromPack()
}

function addListTag(target: 'forbidden' | 'allowed') {
  if (!pack.value) return
  const input = target === 'forbidden' ? newForbiddenTag : newAllowedTag
  const value = input.value.trim(); if (!value) return
  const list = target === 'forbidden' ? pack.value.product_dna.forbiddenChanges : pack.value.product_dna.allowedChanges
  if (!list.includes(value)) list.push(value)
  input.value = ''; syncAdvancedTextFromPack()
}

function removeListTag(target: 'forbidden' | 'allowed', index: number) {
  if (!pack.value) return
  const list = target === 'forbidden' ? pack.value.product_dna.forbiddenChanges : pack.value.product_dna.allowedChanges
  list.splice(index, 1); syncAdvancedTextFromPack()
}

async function analyzeReferences() {
  if (!pack.value || referenceImages.value.length === 0 || analyzing.value) return
  analyzing.value = true; clearMessages()
  try {
    const response = await analyzeProductReferences({
      canvas_id: project.value?.canvas_id,
      selected_images: referenceImages.value,
      messages: [{ role: 'user', content: '请只根据参考图分析家具，并仅返回 JSON：{"structuralFeatures":{"字段":"值"},"materials":{"main":"主材质","secondary":"辅材质"},"forbiddenChanges":["必须保持项"],"allowedChanges":["允许修改项"]}。看不清或无法确认的内容不要猜测。' }],
    })
    const jsonText = response.content.match(/\{[\s\S]*\}/)?.[0]
    if (!jsonText) throw new Error('智能分析未返回可识别的结构化结果')
    const result = JSON.parse(jsonText) as Partial<ProductPack['product_dna']>
    if (result.structuralFeatures && typeof result.structuralFeatures === 'object') Object.assign(pack.value.product_dna.structuralFeatures, result.structuralFeatures)
    if (result.materials && typeof result.materials === 'object') Object.assign(pack.value.product_dna.materials, result.materials)
    if (Array.isArray(result.forbiddenChanges)) pack.value.product_dna.forbiddenChanges = [...new Set([...pack.value.product_dna.forbiddenChanges, ...result.forbiddenChanges])]
    if (Array.isArray(result.allowedChanges)) pack.value.product_dna.allowedChanges = [...new Set([...pack.value.product_dna.allowedChanges, ...result.allowedChanges])]
    syncAdvancedTextFromPack(); savedMessage.value = '智能分析完成，请确认标签后保存'
  } catch (error) { showError('智能分析暂时不可用，你仍可以手动添加标签。', error) } finally { analyzing.value = false }
}

function toggleOutput(output: RecipeOutput) {
  const key = outputKey(output)
  selectedOutputKeys.value = selectedOutputKeys.value.includes(key) ? selectedOutputKeys.value.filter(item => item !== key) : [...selectedOutputKeys.value, key]
}

async function startRun() {
  if (!selectedRecipe.value || busy.value || estimatedJobs.value === 0) return
  busy.value = true; clearMessages()
  if (!await persistPack(true)) { busy.value = false; return }
  try {
    run.value = await createRecipeRun(projectId, {
      recipe_id: selectedRecipe.value.id, request_id: crypto.randomUUID(), created_by: project.value?.created_by || 'member',
      sku_ids: selectedSKUIds.value, selected_outputs: selectedOutputs.value, copies_per_item: copiesPerItem.value, provider: selectedProviderId.value,
    })
    schedulePoll()
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error)
    showError(detail.includes('未配置') ? '生成服务尚未配置，暂时无法开始生成' : '生成任务创建失败，请稍后重试。', error)
  } finally { busy.value = false }
}

async function refreshRun() {
  if (!run.value) return
  try { run.value = await fetchRecipeRun(run.value.id); if (runActive.value) schedulePoll(); else await refreshAssets() }
  catch (error) { showError('任务状态暂时无法刷新。', error) }
}
function schedulePoll() { window.clearTimeout(pollTimer); pollTimer = window.setTimeout(refreshRun, 1200) }

async function retryJob(id: string) {
  if (busy.value) return
  busy.value = true; clearMessages()
  try { const response = await retryGenerationJob(id); run.value = response.run; schedulePoll() }
  catch (error) { showError('这个任务暂时无法重新生成。', error) }
  finally { busy.value = false }
}

async function cancelJob(id: string) {
  try { await cancelGenerationJob(id); await refreshRun() }
  catch (error) { showError('任务取消失败。', error) }
}

async function useAsReference(assetId: string) {
  if (!pack.value || busy.value) return
  if (pack.value.reference_pack.references.some(reference => reference.asset_id === assetId)) { savedMessage.value = '这张图片已经在参考图中'; return }
  pack.value.reference_pack.references.push({ asset_id: assetId, role: 'product_main', weight: 1, locked: true })
  busy.value = true; clearMessages()
  if (await persistPack(true)) savedMessage.value = '生成结果已设为参考图'
  busy.value = false
}

function openCanvas() { if (project.value) window.open(`/canvas/#/?canvas=${project.value.canvas_id}`, '_blank') }
function openModelSettings() { modelSettingsOpen.value = true; window.setTimeout(() => document.getElementById('model-settings')?.scrollIntoView({ behavior: 'smooth', block: 'center' }), 0) }

onMounted(load)
onBeforeUnmount(() => window.clearTimeout(pollTimer))
watch(selectedRecipeId, () => { selectedOutputKeys.value = selectedRecipe.value?.outputs.map(outputKey) || [] })
</script>

<template>
  <div class="mx-auto max-w-[1440px] space-y-8 px-4 py-6 md:px-8 md:py-8">
    <header class="flex flex-wrap items-end justify-between gap-4 border-b border-base-200/70 pb-6">
      <div>
        <button class="btn btn-ghost btn-xs -ml-2 mb-3 text-base-content/50" @click="router.push('/')">← 返回项目</button>
        <p class="text-xs font-bold tracking-[0.18em] text-primary/70">家具电商生成工作台</p>
        <h1 class="mt-2 text-3xl font-black tracking-tight">{{ project?.name || '产品项目' }}</h1>
        <p class="mt-2 text-sm text-base-content/45">{{ project?.product_name }} · {{ project?.created_by }}</p>
      </div>
      <button class="btn btn-outline btn-sm" :disabled="!project" @click="openCanvas">打开无限画布</button>
    </header>

    <div v-if="errorMessage" class="rounded-2xl border border-base-300 bg-base-200/50 p-4 text-sm">
      <div class="flex flex-wrap items-center justify-between gap-3"><p class="font-semibold">{{ errorMessage }}</p><button v-if="errorMessage.includes('生成服务')" class="btn btn-primary btn-sm" @click="openModelSettings">前往模型设置</button></div>
      <details v-if="technicalError" class="mt-3 text-xs text-base-content/45"><summary class="cursor-pointer">查看技术详情</summary><p class="mt-2 break-words font-mono">{{ technicalError }}</p></details>
    </div>
    <div v-if="savedMessage" class="rounded-xl bg-primary/5 px-4 py-3 text-sm font-medium text-primary">{{ savedMessage }}</div>

    <template v-if="pack">
      <section class="workspace-section">
        <div class="section-heading"><span class="step-number">1</span><div><h2>产品资料</h2><p>确认智能生成必须认识并保持的产品信息。</p></div><button class="btn btn-primary btn-sm ml-auto" :disabled="busy" @click="save">保存资料</button></div>
        <div class="grid gap-5 md:grid-cols-2">
          <label class="form-control"><span class="field-label">产品名称</span><input v-model="pack.product_name" class="input input-bordered bg-base-100" /></label>
          <label class="form-control"><span class="field-label">产品类型</span><input v-model="pack.product_dna.productType" class="input input-bordered bg-base-100" placeholder="例如：电视柜" /></label>
        </div>
        <div class="mt-7 grid gap-6 lg:grid-cols-2">
          <div class="tag-group"><div class="tag-group-title"><span>主体结构</span><span class="tag-hint">例如：6 抽、落地式、无柜脚</span></div><div class="tag-list"><span v-for="([key, value]) in structureEntries" :key="key" class="info-tag">{{ value }}<button @click="removeObjectTag('structure', key)">×</button></span></div><div class="tag-input"><input v-model="newStructureTag" placeholder="添加结构特征" @keyup.enter="addObjectTag('structure')" /><button @click="addObjectTag('structure')">添加</button></div></div>
          <div class="tag-group"><div class="tag-group-title"><span>产品材质</span><span class="tag-hint">主材质与辅材质</span></div><div class="tag-list"><span v-for="([key, value]) in materialEntries" :key="key" class="info-tag">{{ value }}<button @click="removeObjectTag('material', key)">×</button></span></div><div class="tag-input"><input v-model="newMaterialTag" placeholder="添加材质" @keyup.enter="addObjectTag('material')" /><button @click="addObjectTag('material')">添加</button></div></div>
          <div class="tag-group"><div class="tag-group-title"><span>必须保持</span><span class="tag-hint">结构、木纹、抽屉、五金等</span></div><div class="tag-list"><span v-for="(value, index) in pack.product_dna.forbiddenChanges" :key="`${value}-${index}`" class="info-tag locked-tag">{{ value }}<button @click="removeListTag('forbidden', index)">×</button></span></div><div class="tag-input"><input v-model="newForbiddenTag" placeholder="添加必须保持项" @keyup.enter="addListTag('forbidden')" /><button @click="addListTag('forbidden')">添加</button></div></div>
          <div class="tag-group"><div class="tag-group-title"><span>本次允许修改</span><span class="tag-hint">场景、视角、软装等</span></div><div class="tag-list"><span v-for="(value, index) in pack.product_dna.allowedChanges" :key="`${value}-${index}`" class="info-tag">{{ value }}<button @click="removeListTag('allowed', index)">×</button></span></div><div class="tag-input"><input v-model="newAllowedTag" placeholder="添加允许修改项" @keyup.enter="addListTag('allowed')" /><button @click="addListTag('allowed')">添加</button></div></div>
        </div>
        <div class="mt-6 flex flex-wrap items-center gap-3 border-t border-base-200/70 pt-5">
          <button class="btn btn-outline btn-sm" :disabled="analyzing || referenceImages.length === 0" @click="analyzeReferences"><span v-if="analyzing" class="loading loading-spinner loading-xs" />{{ analyzing ? '正在分析参考图' : '智能分析并补全标签' }}</button>
          <span v-if="referenceImages.length === 0" class="text-xs text-base-content/40">添加参考图后可使用智能分析</span>
          <details class="ml-auto w-full md:w-auto"><summary class="cursor-pointer text-xs font-semibold text-base-content/45">高级设置</summary><div class="mt-4 grid min-w-0 gap-3 md:min-w-[680px] md:grid-cols-2"><label class="form-control"><span class="field-label">原始结构数据</span><textarea v-model="structuresJSON" class="textarea textarea-bordered min-h-32 font-mono text-xs" /></label><label class="form-control"><span class="field-label">原始材质数据</span><textarea v-model="materialsJSON" class="textarea textarea-bordered min-h-32 font-mono text-xs" /></label><label class="form-control"><span class="field-label">必须保持（每行一条）</span><textarea v-model="forbiddenText" class="textarea textarea-bordered min-h-24 text-xs" /></label><label class="form-control"><span class="field-label">允许修改（每行一条）</span><textarea v-model="allowedText" class="textarea textarea-bordered min-h-24 text-xs" /></label></div></details>
        </div>
      </section>

      <section class="workspace-section">
        <div class="section-heading"><span class="step-number">2</span><div><h2>参考图与生成要求</h2><p>按图片用途分类，决定哪些内容需要重点保持。</p></div></div>
        <ReferenceGrid :references="pack.reference_pack.references" :assets="assets" :busy="busy" @upload="uploadReferences" @replace="replaceReference" @remove="pack.reference_pack.references.splice($event, 1)" @add-empty="addReference" />
      </section>

      <section class="workspace-section">
        <div class="section-heading"><span class="step-number">3</span><div><h2>产品规格 / 画幅 / 批量生成</h2><p>选择需要生产的规格和图片画幅。</p></div></div>
        <div class="grid items-start gap-8 xl:grid-cols-[minmax(0,1fr)_360px]">
          <div>
            <div class="mb-4 flex items-center justify-between"><h3 class="font-bold">选择产品规格</h3><button class="btn btn-ghost btn-sm" @click="addSKU">＋ 添加规格</button></div>
            <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              <article v-for="(sku, index) in pack.skus" :key="sku.id || index" class="sku-card" :class="{ selected: sku.id && selectedSKUIds.includes(sku.id) }">
                <div class="flex items-start justify-between gap-3"><label class="flex cursor-pointer items-center gap-3"><input v-if="sku.id" v-model="selectedSKUIds" :value="sku.id" type="checkbox" class="checkbox checkbox-primary checkbox-sm" /><span><b class="block text-lg">{{ skuDisplayName(sku.name, sku.label) }}</b><span class="text-xs text-base-content/40">电视柜长度规格</span></span></label><button class="btn btn-ghost btn-xs" @click="expandedSKUId = expandedSKUId === sku.id ? '' : (sku.id || '')">{{ expandedSKUId === sku.id ? '收起' : '详情' }}</button></div>
                <div v-if="expandedSKUId === sku.id" class="mt-4 space-y-3 border-t border-base-200/70 pt-4"><label class="form-control"><span class="field-label">长度（毫米）</span><input v-model.number="sku.width" type="number" class="input input-bordered input-sm" placeholder="例如 1300" /></label><div class="grid grid-cols-2 gap-2"><label class="form-control"><span class="field-label">高度（毫米）</span><input v-model.number="sku.height" type="number" class="input input-bordered input-sm" /></label><label class="form-control"><span class="field-label">深度（毫米）</span><input v-model.number="sku.depth" type="number" class="input input-bordered input-sm" /></label></div><div class="grid grid-cols-2 gap-2"><label class="form-control"><span class="field-label">内部名称</span><input v-model="sku.name" class="input input-bordered input-sm" /></label><label class="form-control"><span class="field-label">显示名称</span><input v-model="sku.label" class="input input-bordered input-sm" /></label></div><button class="btn btn-ghost btn-xs text-error/75" @click="removeSKU(index)">删除这个规格</button></div>
              </article>
            </div>
          </div>

          <aside class="generation-console">
            <p class="text-xs font-bold tracking-[0.16em] text-primary/70">生成控制台</p><h3 class="mt-2 text-xl font-black">商品主图</h3>
            <div class="console-row"><span>已选择规格</span><b>{{ selectedSKUCount }} 个</b></div>
            <div class="mt-5"><p class="field-label">输出画幅</p><div class="mt-2 grid grid-cols-2 gap-2"><button v-for="output in selectedRecipe?.outputs" :key="outputKey(output)" class="aspect-button" :class="{ selected: selectedOutputKeys.includes(outputKey(output)) }" @click="toggleOutput(output)">{{ output.aspectRatio }}</button></div></div>
            <label class="form-control mt-5"><span class="field-label">每个规格每种画幅生成</span><select v-model.number="copiesPerItem" class="select select-bordered w-full"><option v-for="count in 4" :key="count" :value="count">{{ count }} 张</option></select></label>
            <div class="mt-6 rounded-2xl bg-base-200/55 p-4"><p class="text-sm font-bold">{{ generationSummary }}</p><div class="mt-3 flex items-center justify-between text-xs text-base-content/50"><span>预计费用</span><b>{{ currencyLabel }} {{ estimatedCost.toFixed(2) }}</b></div><div class="mt-2 flex items-center justify-between text-xs text-base-content/50"><span>当前模型</span><b>{{ currentModelLabel }}</b></div></div>
            <button class="btn btn-primary mt-5 w-full" :disabled="busy || estimatedJobs === 0 || runActive" @click="startRun">{{ runActive ? '任务生成中' : `开始生成 ${estimatedJobs} 张` }}</button>
            <p v-if="estimatedJobs === 0" class="mt-2 text-center text-xs text-base-content/40">请至少选择一个产品规格和一个画幅</p>
            <div id="model-settings" class="mt-5 border-t border-base-200/70 pt-4"><button class="flex w-full items-center justify-between text-xs font-semibold text-base-content/50" @click="modelSettingsOpen = !modelSettingsOpen"><span>模型设置</span><span>{{ modelSettingsOpen ? '收起' : '展开' }}</span></button><div v-if="modelSettingsOpen" class="mt-4 space-y-3"><label class="form-control"><span class="field-label">生成模式</span><select v-model="selectedRecipeId" class="select select-bordered select-sm"><option v-for="recipe in recipes" :key="recipe.id" :value="recipe.id">{{ recipe.name }} · 第 {{ recipe.version }} 版</option></select></label><label class="form-control"><span class="field-label">生成服务</span><select v-model="selectedProviderId" class="select select-bordered select-sm"><option v-for="provider in providers" :key="provider.id" :value="provider.id">{{ provider.id === 'legacy' ? '默认生图服务' : provider.name }}</option></select></label><button class="btn btn-ghost btn-xs justify-start px-0 text-primary" @click="router.push('/admin/nodes/image')">打开完整模型设置</button></div></div>
          </aside>
        </div>
      </section>

      <section v-if="run" class="workspace-section">
        <div class="section-heading"><div><p class="text-xs font-bold tracking-[0.16em] text-primary/70">生成结果</p><h2 class="mt-1">{{ runStatusLabel }}</h2><p>{{ run.jobs.length }} 个任务 · 可按单张继续处理</p></div></div>
        <ResultGrid :jobs="run.jobs" :assets="assets" :skus="pack.skus" :busy="busy" @retry="retryJob" @edit="openCanvas" @use-as-reference="useAsReference" @cancel="cancelJob" />
      </section>
    </template>
  </div>
</template>
