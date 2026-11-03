<template>
  <div v-if="cfg" class="space-y-6">
    <div class="card divide-y divide-gray-100 dark:divide-dark-700">
      <div class="flex items-center justify-between p-5">
        <div>
          <p class="font-medium text-gray-900 dark:text-white">签到开关</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">关闭后用户无法签到，登录弹窗和顶栏签到按钮也不再出现</p>
        </div>
        <Toggle v-model="cfg.enabled" />
      </div>
      <div class="flex items-center justify-between p-5">
        <div>
          <p class="font-medium text-gray-900 dark:text-white">登录后弹窗提醒</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">用户当天没签到时，登录后弹出签到卡片，可在弹窗里直接签到</p>
        </div>
        <Toggle v-model="cfg.popup_enabled" />
      </div>
      <div class="p-5">
        <p class="mb-3 font-medium text-gray-900 dark:text-white">7 天签到奖励（积分）</p>
        <div class="grid grid-cols-7 gap-2">
          <label v-for="(_, i) in cfg.rewards" :key="i" class="flex flex-col items-center gap-1 text-xs text-gray-500">
            第 {{ i + 1 }} 天
            <input v-model.number="cfg.rewards[i]" type="number" min="0" step="1" class="input w-full text-center" />
          </label>
        </div>
        <div class="mt-4 flex flex-wrap items-center gap-6 text-sm text-gray-600 dark:text-gray-400">
          <label class="flex items-center gap-2">
            第 7 天额外奖励
            <input v-model.number="cfg.week_bonus" type="number" min="0" class="input w-24" />
            积分
          </label>
          <span>每轮最多 {{ weekTotal }} 积分；断签后从第 1 天重新计</span>
        </div>
      </div>
      <div class="flex flex-wrap items-center gap-3 p-5 text-sm text-gray-600 dark:text-gray-400">
        <span class="font-medium text-gray-900 dark:text-white">同一 IP 每天最多签到账号数</span>
        <input v-model.number="cfg.ip_daily_max" type="number" min="0" class="input w-24" />
        <span class="text-xs">0 表示不限，用于防止批量小号刷积分</span>
      </div>
    </div>

    <!-- 连签加成 -->
    <div class="card space-y-4 p-5">
      <div class="flex items-center justify-between">
        <div>
          <p class="font-medium text-gray-900 dark:text-white">连签加成</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            连续签到达到天数后，幸运九宫格里勾选的奖品中奖概率提高，提高的部分从“谢谢惠顾”里扣。奖品仍受奖金罐约束，不会增加成本上限。
          </p>
        </div>
        <Toggle v-model="cfg.streak_boost.enabled" />
      </div>
      <label class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
        连续签到满
        <input v-model.number="cfg.streak_boost.min_streak" type="number" min="1" max="365" class="input w-20" />
        天生效
      </label>
      <table class="w-full text-sm">
        <thead class="text-left text-xs text-gray-400">
          <tr>
            <th class="pb-2">加成</th>
            <th class="pb-2">奖品</th>
            <th class="pb-2">基础概率</th>
            <th class="pb-2">增加</th>
            <th class="pb-2">加成后</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
          <tr v-for="(t, i) in tiers" v-show="t.prize_type !== 'none'" :key="i">
            <td class="py-2"><input type="checkbox" class="accent-primary-500" :checked="!!boostOf(i)" @change="toggleBoost(i)" /></td>
            <td class="py-2 text-gray-800 dark:text-gray-200">{{ t.emoji }} {{ t.label }}</td>
            <td class="py-2 tabular-nums text-gray-500">{{ fmtBP(t.prob_bp) }}</td>
            <td class="py-2">
              <span v-if="boostOf(i)" class="flex items-center gap-1">
                +<input
                  type="number"
                  min="0.01"
                  max="50"
                  step="0.01"
                  class="input w-20"
                  :value="bpToPct(boostOf(i)!.add_bp)"
                  @input="boostOf(i)!.add_bp = pctToBp(($event.target as HTMLInputElement).value)"
                />%
              </span>
              <span v-else class="text-gray-300">—</span>
            </td>
            <td class="py-2 tabular-nums" :class="boostOf(i) ? 'font-medium text-primary-600 dark:text-primary-400' : 'text-gray-400'">
              {{ fmtBP(t.prob_bp + (boostOf(i)?.add_bp ?? 0)) }}
            </td>
          </tr>
        </tbody>
      </table>
      <p class="text-xs" :class="boostTotal > noneBP ? 'text-rose-500' : 'text-gray-500 dark:text-gray-400'">
        加成合计 +{{ fmtBP(boostTotal) }}，“谢谢惠顾”当前 {{ fmtBP(noneBP) }}，加成后剩 {{ fmtBP(Math.max(0, noneBP - boostTotal)) }}
        <template v-if="boostTotal > noneBP">。不够扣，加成会按比例缩小</template>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Toggle from '@/components/common/Toggle.vue'
import { fmtBP } from '../slot/api'
import { bpToPct, gridTiers, park, pctToBp } from './adminState'

const cfg = computed(() => park.checkin)
const tiers = computed(() => gridTiers())

const weekTotal = computed(() => (cfg.value ? cfg.value.rewards.reduce((s, v) => s + (v || 0), 0) + (cfg.value.week_bonus || 0) : 0))

const boostOf = (i: number) => cfg.value?.streak_boost.tiers.find((b) => b.tier === i)

function toggleBoost(i: number) {
  const list = cfg.value!.streak_boost.tiers
  const idx = list.findIndex((b) => b.tier === i)
  if (idx >= 0) list.splice(idx, 1)
  else list.push({ tier: i, add_bp: 100 })
}

const boostTotal = computed(() => cfg.value?.streak_boost.tiers.reduce((s, b) => s + (b.add_bp || 0), 0) ?? 0)
const noneBP = computed(() => tiers.value.find((t) => t.prize_type === 'none')?.prob_bp ?? 0)
</script>
