<script setup lang="ts">
import { referenceRoles, type ReferenceRole } from '../types/product'
defineProps<{ images: { id: string; url: string; label: string; role?: ReferenceRole }[]; uploading: boolean }>()
defineEmits<{
  (event: 'remove', id: string): void
  (event: 'preview', image: { id: string; url: string }): void
  (event: 'add'): void
  (event: 'role', id: string, role: ReferenceRole): void
}>()
</script>

<template>
  <div class="mb-3 min-w-0">
    <div class="mb-2 text-xs text-white/60">参考图 · {{ images.length }}/8</div>
    <div class="flex gap-2 overflow-x-auto pb-2" data-testid="reference-images" @wheel.stop>
      <div v-for="img in images" :key="img.id" data-testid="reference-card" class="relative w-28 shrink-0 rounded-lg border border-white/15 bg-black/20 p-1">
        <button class="block h-24 w-full" :aria-label="`预览${img.label}`" @click=" $emit('preview', img)">
          <img :src="img.url" :alt="img.label" class="h-full w-full object-contain" draggable="false" />
        </button>
        <div class="truncate px-1 pt-1 text-[10px] text-white/70">{{ img.label }}</div>
        <select :value="img.role || 'product'" aria-label="参考角色" class="mt-1 w-full rounded bg-white/5 text-[10px] text-white/70" @change="$emit('role', img.id, ($event.target as HTMLSelectElement).value as ReferenceRole)">
          <option v-for="[value, label] in referenceRoles" :key="value" :value="value" class="bg-neutral-900">{{ label }}</option>
        </select>
        <button class="absolute right-1 top-1 h-6 w-6 rounded-full bg-black/80 text-white hover:bg-red-600" :aria-label="`删除${img.label}`" @click.stop="$emit('remove', img.id)">×</button>
      </div>
      <button class="h-32 w-20 shrink-0 rounded-lg border border-dashed border-white/25 text-xs text-white/60 hover:text-white disabled:opacity-40" :disabled="uploading || images.length >= 8" aria-label="添加参考图" @click="$emit('add')">{{ uploading ? '上传中…' : '+ 添加' }}</button>
    </div>
  </div>
</template>
