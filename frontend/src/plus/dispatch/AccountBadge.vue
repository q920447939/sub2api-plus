<!-- [plus] 账号列表上的小标签：这个账号在调度台被暂停或调了权重（启用但被压住时，运营在账号列表里也能看到） -->
<template>
  <router-link v-if="text" :to="`/admin/dispatch/account/${accountId}`" class="mt-0.5 inline-block self-start rounded bg-blue-50 px-1.5 text-[11px] text-blue-700 hover:underline dark:bg-blue-900/30 dark:text-blue-300" title="在上游调度台查看">
    调度台：{{ text }}
  </router-link>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { badges, loadBadges } from './badgeStore'

const props = defineProps<{ accountId: number }>()
void loadBadges()
const text = computed(() => badges.value[String(props.accountId)] ?? '')
</script>
