<template>
  <div v-if="atm" class="space-y-6">
    <div class="card divide-y divide-gray-100 dark:divide-dark-700">
      <div class="flex items-center justify-between p-5">
        <div>
          <p class="font-medium text-gray-900 dark:text-white">中奖跑马灯</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">积分乐园顶部滚动今天最新的中奖记录，真实和虚拟记录混在一起，用户端不显示任何累计数字</p>
        </div>
        <Toggle v-model="atm.marquee_enabled" />
      </div>
      <label class="flex flex-wrap items-center gap-3 p-5 text-sm text-gray-600 dark:text-gray-400">
        <span class="font-medium text-gray-900 dark:text-white">滚动条数</span>
        <input v-model.number="atm.marquee_size" type="number" min="1" max="100" class="input w-24" />
        <span class="text-xs">只取今天最新的 N 条</span>
      </label>
    </div>

    <div class="card space-y-4 p-5">
      <div class="flex items-center justify-between">
        <div>
          <p class="font-medium text-gray-900 dark:text-white">虚拟中奖记录</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            后台每分钟按小时曲线（凌晨少、晚高峰多）生成，每天在目标条数的 0.7–1.3 倍之间浮动。昵称按注册邮箱白名单脱敏生成，与真实用户格式一致。
          </p>
        </div>
        <Toggle v-model="atm.virtual_enabled" />
      </div>
      <label class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
        每天约生成
        <input v-model.number="atm.virtual_daily_target" type="number" min="0" max="5000" step="10" class="input w-24" :disabled="!atm.virtual_enabled" />
        条
      </label>

      <div>
        <div class="mb-2 flex items-center justify-between">
          <p class="text-sm font-medium text-gray-800 dark:text-gray-200">虚拟记录的奖品分布</p>
          <span class="text-xs tabular-nums" :class="vTotal === FULL_BP ? 'text-emerald-600' : 'text-rose-500'">合计 {{ fmtBP(vTotal) }}</span>
        </div>
        <table class="w-full max-w-xl text-sm">
          <thead class="text-left text-xs text-gray-400">
            <tr>
              <th class="pb-2">奖品</th>
              <th class="pb-2">真实概率</th>
              <th class="pb-2">虚拟记录占比</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="(t, i) in tiers" v-show="t.prize_type !== 'none'" :key="i">
              <td class="py-2 text-gray-800 dark:text-gray-200">{{ t.emoji }} {{ t.label }}</td>
              <td class="py-2 tabular-nums text-gray-500">{{ fmtBP(t.prob_bp) }}</td>
              <td class="py-2">
                <span class="flex items-center gap-1">
                  <input
                    type="number"
                    min="0"
                    max="100"
                    step="0.01"
                    class="input w-24"
                    :value="bpToPct(atm.virtual_probs[i] ?? 0)"
                    @input="atm.virtual_probs[i] = pctToBp(($event.target as HTMLInputElement).value)"
                  />%
                </span>
              </td>
            </tr>
          </tbody>
        </table>
        <p class="mt-2 text-xs text-gray-500">
          虚拟记录不会出现“谢谢惠顾”。大奖占比不宜和真实概率差太多，否则用户会觉得别人总中大奖、自己总不中。
        </p>
      </div>
    </div>

    <!-- 最近中奖记录（后台可见真实 / 虚拟） -->
    <div class="card p-5">
      <div class="mb-3 flex items-center justify-between">
        <p class="font-medium text-gray-900 dark:text-white">最近中奖记录</p>
        <button type="button" class="btn btn-secondary btn-sm" @click="loadFeed">刷新</button>
      </div>
      <div class="max-h-80 overflow-y-auto">
        <table class="w-full text-xs">
          <thead class="sticky top-0 bg-white text-left text-gray-400 dark:bg-dark-900">
            <tr>
              <th class="py-1.5">时间</th>
              <th class="py-1.5">昵称</th>
              <th class="py-1.5">奖品</th>
              <th class="py-1.5">类型</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="f in feed" :key="f.id">
              <td class="py-1.5 text-gray-500">{{ new Date(f.created_at).toLocaleString() }}</td>
              <td class="py-1.5">{{ f.display_name }}</td>
              <td class="py-1.5">{{ f.emoji }} {{ f.label }}</td>
              <td class="py-1.5">
                <span class="rounded px-1.5 py-0.5" :class="f.is_virtual ? 'bg-gray-100 text-gray-500 dark:bg-dark-700' : 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/30'">
                  {{ f.is_virtual ? '虚拟' : '真实' }}
                </span>
              </td>
            </tr>
            <tr v-if="!feed.length">
              <td colspan="4" class="py-6 text-center text-gray-400">暂无记录</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminFeed, fmtBP, type AdminFeedItem } from '../slot/api'
import { FULL_BP, bpToPct, gridTiers, park, pctToBp } from './adminState'

const appStore = useAppStore()

const atm = computed(() => park.slot?.config.atmosphere)
const tiers = computed(() => gridTiers())
const vTotal = computed(() => atm.value?.virtual_probs.reduce((s, v) => s + (v || 0), 0) ?? 0)

const feed = ref<AdminFeedItem[]>([])
async function loadFeed() {
  try {
    feed.value = await adminFeed(100)
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '中奖记录加载失败'))
  }
}

onMounted(loadFeed)
</script>
