<template>
  <div v-if="pages > 1" class="flex items-center justify-between gap-2" data-testid="nb-pager">
    <button
      type="button"
      class="nb-btn nb-surface nb-hover px-3 py-1.5 text-xs"
      :disabled="disabled || page <= 1"
      data-testid="nb-pager-prev"
      @click="$emit('change', page - 1)"
    >‹ 上一页</button>
    <span class="nb-head text-xs tabular-nums" data-testid="nb-pager-info">第 {{ page }} / {{ pages }} 页</span>
    <button
      type="button"
      class="nb-btn nb-surface nb-hover px-3 py-1.5 text-xs"
      :disabled="disabled || page >= pages"
      data-testid="nb-pager-next"
      @click="$emit('change', page + 1)"
    >下一页 ›</button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ page: number; total: number; pageSize: number; disabled?: boolean }>()
defineEmits<{ change: [page: number] }>()

const pages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))
</script>
