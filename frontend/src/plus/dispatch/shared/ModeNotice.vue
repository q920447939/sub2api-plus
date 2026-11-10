<!-- [plus] 档位不是「人工调整」时的一行提示（只在和调整有关的页面出现） -->
<template>
  <div v-if="mode && mode !== 'manual'" class="flex items-center gap-2 rounded-lg bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">
    <Icon name="infoCircle" size="sm" />
    <span>{{ mode === 'off' ? '调度台已关闭，不检测、不判定' : '当前为「只观察」，调整会保存但不生效' }}</span>
    <router-link class="ml-auto font-medium underline-offset-2 hover:underline" :to="{ path: `${DISPATCH_BASE}/config`, query: { tab: 'strategy' } }">调度设置</router-link>
  </div>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import { DISPATCH_BASE } from '../routes'
import { dispatchMode as mode, refreshMode } from './mode'

void refreshMode()
</script>
