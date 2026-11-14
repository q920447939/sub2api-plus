<!-- [plus] 一根分段条：各账号的占比 -->
<template>
  <div class="flex h-2 gap-0.5 overflow-hidden rounded bg-gray-200 dark:bg-dark-600">
    <i v-for="p in parts.filter((x) => x.v > 0.3)" :key="p.id" class="block h-full" :style="{ width: `${p.v}%`, background: color(p.id) }" :title="`${name(p.id)} ${Math.round(p.v)}%`" />
  </div>
</template>

<script setup lang="ts">
defineProps<{ parts: { id: number; v: number }[]; color: (id: number) => string; name: (id: number) => string }>()
</script>
