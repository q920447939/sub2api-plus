<template>
  <div v-if="rb" class="space-y-6">
    <div class="card space-y-4 p-5">
      <div class="flex items-center justify-between">
        <div>
          <p class="font-medium text-gray-900 dark:text-white">兑换卡密送积分</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            用户自己兑换余额卡密成功后，按面值额外送积分。系统发奖码、在线支付和管理员代发的卡密不送；开启之前兑换的卡密不补发。
          </p>
        </div>
        <Toggle v-model="rb.enabled" />
      </div>
      <p v-if="rb.enabled && rb.enabled_at" class="text-xs text-gray-500">本次开启时间：{{ new Date(rb.enabled_at).toLocaleString() }}</p>

      <table class="w-full max-w-xl text-sm">
        <thead class="text-left text-xs text-gray-400">
          <tr>
            <th class="pb-2">卡密面值满</th>
            <th class="pb-2">送积分</th>
            <th class="pb-2"></th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
          <tr v-for="(t, i) in rb.tiers" :key="i">
            <td class="py-2"><span class="flex items-center gap-1">$<input v-model.number="t.threshold" type="number" min="0.01" step="0.01" class="input w-28" /></span></td>
            <td class="py-2"><input v-model.number="t.points" type="number" min="1" class="input w-28" /></td>
            <td class="py-2 text-right"><button type="button" class="btn btn-secondary btn-sm" @click="rb.tiers.splice(i, 1)">删除</button></td>
          </tr>
        </tbody>
      </table>
      <div class="flex items-center gap-3">
        <button type="button" class="btn btn-secondary btn-sm" :disabled="rb.tiers.length >= 20" @click="addTier">+ 添加档位</button>
        <span class="text-xs text-gray-500">按面值从小到大排列，命中最高的一档。例如面值 $60 命中“满 $50”档。</span>
      </div>
    </div>
    <p class="text-xs text-gray-400">以后新增的积分来源也会放在这里。</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Toggle from '@/components/common/Toggle.vue'
import { park } from './adminState'

const rb = computed(() => park.points?.redeem_bonus)

function addTier() {
  const list = rb.value!.tiers
  const last = list[list.length - 1]
  list.push({ threshold: last ? last.threshold * 2 : 10, points: last ? last.points * 2 : 100 })
}
</script>
