<template>
  <div class="overflow-x-auto">
    <table class="w-full text-sm" :class="showUser ? 'min-w-[860px]' : 'min-w-[640px]'">
      <thead class="text-left text-xs text-gray-400">
        <tr>
          <th class="py-2">时间</th>
          <th v-if="showUser" class="py-2">用户</th>
          <th class="py-2">类型</th>
          <th class="py-2 text-right">变动</th>
          <th class="py-2 text-right">变动后余额</th>
          <th class="py-2 pl-4">说明</th>
        </tr>
      </thead>
      <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
        <tr v-for="it in items" :key="it.id">
          <td class="py-2 text-gray-500">{{ fmtTime(it.created_at) }}</td>
          <td v-if="showUser" class="py-2">{{ it.email }} <span class="text-xs text-gray-400">#{{ it.user_id }}</span></td>
          <td class="py-2">{{ reasonIcon(it.reason) }} {{ reasonLabel(it.reason) }}</td>
          <td class="py-2 text-right font-medium tabular-nums" :class="it.delta >= 0 ? 'text-emerald-600' : 'text-rose-500'">
            {{ it.delta >= 0 ? '+' : '' }}{{ it.delta }}
          </td>
          <td class="py-2 text-right tabular-nums text-gray-500">{{ it.balance }}</td>
          <td class="py-2 pl-4 text-gray-500">
            {{ it.note }}
            <span v-if="it.remark" class="block text-xs text-amber-600">
              原因：{{ it.remark }}<template v-if="it.operator_email"> · 操作人 {{ it.operator_email }}</template>
            </span>
          </td>
        </tr>
        <tr v-if="!items.length">
          <td :colspan="showUser ? 6 : 5" class="py-8 text-center text-gray-400">{{ loading ? '加载中…' : '暂无流水' }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { reasonIcon, reasonLabel, type AdminLedgerItem } from '../points/api'
import { fmtTime } from './adminFormat'

defineProps<{ items: AdminLedgerItem[]; loading?: boolean; showUser?: boolean }>()
</script>
