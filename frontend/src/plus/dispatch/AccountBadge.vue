<!-- [plus] 账号列表上的小标签：这个账号在调度台被调整了（启用但被压住时，运营在账号列表里也能看到） -->
<template>
  <router-link v-for="b in list" :key="b.group_id" :to="{ path: '/admin/dispatch/adjust', query: { tab: 'active' } }" class="mt-0.5 inline-block self-start rounded bg-blue-50 px-1.5 text-[11px] text-blue-700 hover:underline dark:bg-blue-900/30 dark:text-blue-300" :title="`上游调度台在「${b.group}」里调整了这个账号`">
    调度台：{{ b.text }}{{ multi ? `（${b.group}）` : '' }}
  </router-link>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { badges, loadBadges } from './badgeStore'

const props = defineProps<{ accountId: number }>()
void loadBadges()
const list = computed(() => badges.value[String(props.accountId)] ?? [])
const multi = computed(() => list.value.length > 1)
</script>
