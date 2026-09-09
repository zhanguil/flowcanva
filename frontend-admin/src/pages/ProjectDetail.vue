<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { cancelGenerationJob, createRecipeRun, fetchAssets, fetchGenerationProviders, fetchProject, fetchProjectRecipeRuns, fetchRecipeRun, fetchRecipes, retryFailedJobs, saveProductPack, uploadAsset } from '../api'
import type { Asset, GenerationProviderInfo, ProductPack, Recipe, RecipeRun, StudioProjectDetail } from '../types'

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
const selectedOutputTypes = ref<string[]>([])
const structuresJSON = ref('{}')
const materialsJSON = ref('{}')
const forbiddenText = ref('')
const allowedText = ref('')
const run = ref<RecipeRun | null>(null)
const assets = ref<Asset[]>([])
const uploadRole = ref('product_main')
const busy = ref(false)
const errorMessage = ref('')
const savedMessage = ref('')
let pollTimer: number | undefined

const selectedRecipe = computed(() => recipes.value.find(item => item.id === selectedRecipeId.value))
const selectedSKUCount = computed(() => selectedSKUIds.value.length || pack.value?.skus.length || 0)
const estimatedJobs = computed(() => selectedSKUCount.value * selectedOutputTypes.value.length)
const estimatedCost = computed(() => estimatedJobs.value * (selectedRecipe.value?.estimated_cost_per_job || 0))
const failedCount = computed(() => run.value?.jobs.filter(job => job.status === 'failed' || job.status === 'rejected').length || 0)
const runActive = computed(() => ['queued', 'running'].includes(run.value?.status || ''))
const assetById = computed(() => new Map(assets.value.map(asset => [asset.id, asset])))

function setPack(next: ProductPack) {
  pack.value = structuredClone(next)
  const dna = pack.value.product_dna
  structuresJSON.value = JSON.stringify(dna.structuralFeatures || {}, null, 2)
  materialsJSON.value = JSON.stringify(dna.materials || {}, null, 2)
  forbiddenText.value = (dna.forbiddenChanges || []).join('\n')
  allowedText.value = (dna.allowedChanges || []).join('\n')
  selectedSKUIds.value = pack.value.skus.map(sku => sku.id).filter((id): id is string => Boolean(id))
}

async function load() {
  errorMessage.value = ''
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
		selectedOutputTypes.value = selectedRecipe.value?.outputs.map(output => output.outputType) || []
		if (runActive.value) schedulePoll()
  } catch (error) {
    errorMessage.value = `项目读取失败：${(error as Error).message}`
  }
}

function addSKU() {
  pack.value?.skus.push({ name: '', label: '', width: null, height: null, depth: null, reference_asset_ids: [] })
}

function addReference() {
  pack.value?.reference_pack.references.push({ asset_id: '', role: 'product_main', weight: 1, locked: true })
}

async function uploadReferences(event: Event) {
	const input = event.target as HTMLInputElement
	const files = Array.from(input.files || [])
	if (!pack.value || files.length === 0) return
	busy.value = true
	errorMessage.value = ''
	try {
		for (const file of files) {
			const asset = await uploadAsset(file, { project_id: projectId, source_type: 'upload', role: uploadRole.value, created_by: project.value?.created_by || 'member' })
			assets.value.unshift(asset)
			if (!pack.value.reference_pack.references.some(reference => reference.asset_id === asset.id)) {
				pack.value.reference_pack.references.push({ asset_id: asset.id, role: uploadRole.value, weight: 1, locked: true })
			}
		}
		await save()
	} catch (error) {
		errorMessage.value = `参考图上传失败：${(error as Error).message}`
	} finally {
		busy.value = false
		input.value = ''
	}
}

async function save() {
  if (!pack.value) return
  busy.value = true
  errorMessage.value = ''
  savedMessage.value = ''
  try {
    pack.value.product_dna.structuralFeatures = JSON.parse(structuresJSON.value || '{}')
    pack.value.product_dna.materials = JSON.parse(materialsJSON.value || '{}')
    pack.value.product_dna.forbiddenChanges = forbiddenText.value.split('\n').map(value => value.trim()).filter(Boolean)
    pack.value.product_dna.allowedChanges = allowedText.value.split('\n').map(value => value.trim()).filter(Boolean)
    const saved = await saveProductPack(projectId, pack.value)
    setPack(saved)
    savedMessage.value = 'Product Pack 已保存'
  } catch (error) {
    errorMessage.value = `保存失败：${(error as Error).message}`
  } finally {
    busy.value = false
  }
}

async function startRun() {
  if (!selectedRecipe.value || busy.value) return
  if (!confirm(`确认创建 ${estimatedJobs.value} 个独立任务？预计费用 ${selectedRecipe.value.currency} ${estimatedCost.value.toFixed(2)}。`)) return
  busy.value = true
  errorMessage.value = ''
  try {
    run.value = await createRecipeRun(projectId, {
      recipe_id: selectedRecipe.value.id,
      request_id: crypto.randomUUID(),
      created_by: project.value?.created_by || 'member',
      sku_ids: selectedSKUIds.value,
			output_types: selectedOutputTypes.value,
			provider: selectedProviderId.value,
    })
    schedulePoll()
  } catch (error) {
    errorMessage.value = `创建生成批次失败：${(error as Error).message}`
  } finally {
    busy.value = false
  }
}

async function refreshRun() {
  if (!run.value) return
  try {
    run.value = await fetchRecipeRun(run.value.id)
  } catch (error) {
    errorMessage.value = `任务状态刷新失败：${(error as Error).message}`
  }
  if (runActive.value) schedulePoll()
}

function schedulePoll() {
  window.clearTimeout(pollTimer)
  pollTimer = window.setTimeout(refreshRun, 1200)
}

async function retryFailed() {
  if (!run.value || busy.value) return
  busy.value = true
  try {
    const response = await retryFailedJobs(run.value.id)
    run.value = response.run
    schedulePoll()
  } catch (error) {
    errorMessage.value = `重试失败：${(error as Error).message}`
  } finally {
    busy.value = false
  }
}

async function cancelJob(id: string) {
  await cancelGenerationJob(id)
  await refreshRun()
}

function openCanvas() {
  if (project.value) window.open(`/canvas/#/?canvas=${project.value.canvas_id}`, '_blank')
}

const statusLabel: Record<string, string> = {
  queued: '排队中', running: '生成中', generated: '已生成', validating: '校验中', success: '完成',
  failed: '失败', rejected: '未通过', cancelled: '已取消', retrying: '重试中',
}

onMounted(load)
onBeforeUnmount(() => window.clearTimeout(pollTimer))
watch(selectedRecipeId, () => { selectedOutputTypes.value = selectedRecipe.value?.outputs.map(output => output.outputType) || [] })
</script>

<template>
  <div class="mx-auto max-w-7xl space-y-6">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <button class="btn btn-ghost btn-xs -ml-2 mb-2" @click="router.push('/')">← 返回项目</button>
        <h1 class="text-2xl font-extrabold">{{ project?.name || '产品项目' }}</h1>
        <p class="mt-1 text-sm text-base-content/50">{{ project?.product_name }} · {{ project?.created_by }}</p>
      </div>
      <button class="btn btn-primary btn-sm" :disabled="!project" @click="openCanvas">打开无限画布</button>
    </div>

    <div v-if="errorMessage" class="alert alert-error text-sm">{{ errorMessage }}</div>
    <div v-if="savedMessage" class="alert alert-success text-sm">{{ savedMessage }}</div>

    <template v-if="pack">
      <div class="grid gap-6 xl:grid-cols-[1.2fr_0.8fr]">
        <section class="card border border-base-200 bg-base-100 shadow-sm">
          <div class="card-body gap-5">
            <div class="flex items-center justify-between"><h2 class="card-title text-base">Product Pack</h2><button class="btn btn-primary btn-sm" :disabled="busy" @click="save">保存资料</button></div>
            <label class="form-control"><span class="label-text mb-1">产品名称</span><input v-model="pack.product_name" class="input input-bordered input-sm" /></label>
            <label class="form-control"><span class="label-text mb-1">产品类型</span><input v-model="pack.product_dna.productType" class="input input-bordered input-sm" placeholder="例如：移动电视柜" /></label>
            <div class="grid gap-3 md:grid-cols-2">
              <label class="form-control"><span class="label-text mb-1">结构特征（JSON）</span><textarea v-model="structuresJSON" class="textarea textarea-bordered min-h-36 font-mono text-xs" /></label>
              <label class="form-control"><span class="label-text mb-1">材质（JSON）</span><textarea v-model="materialsJSON" class="textarea textarea-bordered min-h-36 font-mono text-xs" /></label>
              <label class="form-control"><span class="label-text mb-1">禁止改变（每行一条）</span><textarea v-model="forbiddenText" class="textarea textarea-bordered min-h-28 text-sm" /></label>
              <label class="form-control"><span class="label-text mb-1">允许改变（每行一条）</span><textarea v-model="allowedText" class="textarea textarea-bordered min-h-28 text-sm" /></label>
            </div>

            <div>
              <div class="mb-2 flex items-center justify-between"><h3 class="font-bold">SKU</h3><button class="btn btn-ghost btn-xs" @click="addSKU">＋ 添加 SKU</button></div>
              <div v-for="(sku, index) in pack.skus" :key="sku.id || index" class="mb-2 grid grid-cols-[1fr_1fr_repeat(3,0.8fr)_auto] gap-2">
                <input v-model="sku.name" class="input input-bordered input-sm" placeholder="1300" />
                <input v-model="sku.label" class="input input-bordered input-sm" placeholder="1300mm" />
                <input v-model.number="sku.width" type="number" class="input input-bordered input-sm" placeholder="宽" />
                <input v-model.number="sku.height" type="number" class="input input-bordered input-sm" placeholder="高" />
                <input v-model.number="sku.depth" type="number" class="input input-bordered input-sm" placeholder="深" />
                <button class="btn btn-ghost btn-sm text-error" @click="pack.skus.splice(index, 1)">×</button>
              </div>
            </div>

            <div>
              <div class="mb-2 flex items-center justify-between"><h3 class="font-bold">Reference Pack</h3><button class="btn btn-ghost btn-xs" @click="addReference">＋ 关联资产</button></div>
              <div class="mb-3 flex flex-wrap items-center gap-2">
				<select v-model="uploadRole" class="select select-bordered select-sm"><option>product_main</option><option>product_angle</option><option>product_structure</option><option>material</option><option>scene</option><option>style</option></select>
				<label class="btn btn-outline btn-sm">上传参考图<input type="file" multiple accept="image/jpeg,image/png,image/webp" class="hidden" @change="uploadReferences" /></label>
			</div>
              <p class="mb-2 text-xs text-base-content/45">上传后自动加入 Reference Pack；也可以填写素材库中的 Asset ID。</p>
              <div v-for="(reference, index) in pack.reference_pack.references" :key="reference.id || index" class="mb-2 grid grid-cols-[1.4fr_1fr_0.7fr_auto_auto] gap-2">
				<div class="flex min-w-0 items-center gap-2"><img v-if="assetById.get(reference.asset_id)?.url" :src="assetById.get(reference.asset_id)?.url" class="h-9 w-9 rounded object-contain bg-base-200" /><input v-model="reference.asset_id" class="input input-bordered input-sm min-w-0" placeholder="Asset ID" /></div>
                <select v-model="reference.role" class="select select-bordered select-sm"><option>product_main</option><option>product_angle</option><option>product_structure</option><option>material</option><option>scene</option><option>style</option></select>
                <input v-model.number="reference.weight" type="number" step="0.1" class="input input-bordered input-sm" />
                <label class="flex items-center gap-1 text-xs"><input v-model="reference.locked" type="checkbox" class="checkbox checkbox-xs" />锁定</label>
                <button class="btn btn-ghost btn-sm text-error" @click="pack.reference_pack.references.splice(index, 1)">×</button>
              </div>
            </div>
          </div>
        </section>

        <section class="space-y-6">
          <div class="card border border-base-200 bg-base-100 shadow-sm"><div class="card-body">
            <h2 class="card-title text-base">Company Recipe</h2>
            <select v-model="selectedRecipeId" class="select select-bordered w-full"><option v-for="recipe in recipes" :key="recipe.id" :value="recipe.id">{{ recipe.name }} · V{{ recipe.version }}</option></select>
			<select v-model="selectedProviderId" class="select select-bordered w-full"><option v-for="provider in providers" :key="provider.id" :value="provider.id">{{ provider.name }}</option></select>
            <div v-if="selectedRecipe" class="flex flex-wrap gap-2"><label v-for="output in selectedRecipe.outputs" :key="output.outputType" class="badge badge-outline gap-1"><input v-model="selectedOutputTypes" :value="output.outputType" type="checkbox" class="checkbox checkbox-xs" />{{ output.outputType }} · {{ output.aspectRatio }}</label></div>
            <div class="rounded-xl bg-base-200/50 p-4 text-sm"><p>预计生成：<b>{{ estimatedJobs }} Jobs</b></p><p class="mt-1">预计费用：<b>{{ selectedRecipe?.currency }} {{ estimatedCost.toFixed(2) }}</b></p></div>
            <button class="btn btn-primary" :disabled="busy || estimatedJobs === 0 || runActive" @click="startRun">确认并开始生成</button>
          </div></div>

          <div v-if="run" class="card border border-base-200 bg-base-100 shadow-sm"><div class="card-body">
            <div class="flex items-center justify-between"><h2 class="card-title text-base">生成批次 · {{ run.status }}</h2><button v-if="failedCount" class="btn btn-warning btn-xs" :disabled="busy" @click="retryFailed">只重试失败 {{ failedCount }} 项</button></div>
            <div class="space-y-2">
              <div v-for="job in run.jobs" :key="job.id" class="grid grid-cols-[1fr_0.8fr_0.8fr_auto] items-center gap-2 rounded-lg border border-base-200 p-2 text-xs">
                <span class="truncate">{{ pack.skus.find(sku => sku.id === job.sku_id)?.name || job.sku_id }}</span><span>{{ job.output_type }} · {{ job.aspect_ratio }}</span>
                <span :class="{ 'text-success': job.status === 'success', 'text-error': ['failed','rejected'].includes(job.status) }">{{ statusLabel[job.status] || job.status }}</span>
                <button v-if="['queued','running','retrying'].includes(job.status)" class="btn btn-ghost btn-xs" @click="cancelJob(job.id)">取消</button>
                <p v-if="job.error_message" class="col-span-4 text-error">{{ job.error_message }}</p>
              </div>
            </div>
          </div></div>
        </section>
      </div>
    </template>
  </div>
</template>
