<!-- [plus] 反馈状态徽章（沿用全局 badge 样式） -->
<template>
  <span class="badge whitespace-nowrap" :class="STATUS_META[status].tone">{{ STATUS_META[status].label }}</span>
</template>

<script setup lang="ts">
import type { Status } from '../api'
import { STATUS_META } from '../format'

defineProps<{ status: Status }>()
</script>
