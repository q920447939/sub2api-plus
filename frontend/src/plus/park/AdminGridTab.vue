<template>
  <div v-if="cfg && st" class="space-y-6">
    <div class="card grid gap-5 p-5 md:grid-cols-3">
      <div class="flex items-center justify-between md:col-span-3">
        <div>
          <p class="font-medium text-gray-900 dark:text-white">幸运九宫格开关</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">关闭后用户端显示“暂未开放”</p>
        </div>
        <Toggle v-model="cfg.enabled" />
      </div>
      <label class="block text-sm">
        <span class="block text-gray-600 dark:text-gray-400">每次消耗积分</span>
        <input v-model.number="cfg.cost_per_spin" type="number" min="1" class="input mt-1 w-full" />
      </label>
      <label class="block text-sm md:col-span-2">
        <span class="block text-gray-600 dark:text-gray-400">积分估值：多少积分算 $1</span>
        <input v-model.number="cfg.points_per_yuan" type="number" min="1" class="input mt-1 w-full max-w-[200px]" />
        <span class="mt-1 block text-xs text-gray-400">余额奖按这个比例折算成积分，从奖金罐里扣。例如 100 表示 $1 余额奖要从罐里扣 100 积分。用户看不到这个值。</span>
      </label>
    </div>

    <!-- 奖品与概率 -->
    <div class="card p-5">
      <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
        <div>
          <p class="font-medium text-gray-900 dark:text-white">奖品与中奖概率</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">九宫格外圈 8 格，顺时针从左上角开始；必须有且只有 1 个“谢谢惠顾”，概率合计必须是 100%</p>
        </div>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="noneIdx < 0 || diff === 0" @click="fillNone">
          差额补给谢谢惠顾
        </button>
      </div>
      <div class="overflow-x-auto">
        <table class="w-full min-w-[900px] text-sm">
          <thead class="text-left text-xs text-gray-400">
            <tr>
              <th class="pb-2 pr-2">#</th>
              <th class="pb-2 pr-2">图标</th>
              <th class="pb-2 pr-2">标题</th>
              <th class="pb-2 pr-2">副标题</th>
              <th class="pb-2 pr-2">角标</th>
              <th class="pb-2 pr-2">奖品类型</th>
              <th class="pb-2 pr-2">奖励</th>
              <th class="pb-2 pr-2">出奖罐</th>
              <th class="pb-2 pr-2">中奖概率</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="(t, i) in cfg.tiers" :key="i">
              <td class="py-2 pr-2 text-gray-400">{{ i + 1 }}</td>
              <td class="py-2 pr-2"><input v-model="t.emoji" class="input w-14 text-center" maxlength="8" /></td>
              <td class="py-2 pr-2"><input v-model="t.label" class="input w-28" maxlength="16" /></td>
              <td class="py-2 pr-2"><input v-model="t.desc" class="input w-28" maxlength="24" /></td>
              <td class="py-2 pr-2"><input v-model="t.badge" class="input w-20" maxlength="8" /></td>
              <td class="py-2 pr-2">
                <select v-model="t.prize_type" class="input w-28" @change="onTypeChange(t)">
                  <option v-for="k in st.prize_kinds" :key="k.type" :value="k.type">{{ k.name }}</option>
                </select>
              </td>
              <td class="py-2 pr-2">
                <span v-if="t.prize_type === 'points'" class="flex items-center gap-1">
                  <input v-model.number="t.prize_points" type="number" min="1" class="input w-24" />积分
                </span>
                <span v-else-if="t.prize_type === 'balance'" class="flex items-center gap-1">
                  $<input v-model.number="t.prize_balance" type="number" min="0.01" step="0.01" class="input w-24" />
                </span>
                <span v-else class="text-gray-300">—</span>
              </td>
              <td class="py-2 pr-2">
                <select v-if="t.prize_type !== 'none'" v-model="t.pot" class="input w-24">
                  <option value="small">小奖罐</option>
                  <option value="jackpot">大奖罐</option>
                </select>
                <span v-else class="text-gray-300">—</span>
              </td>
              <td class="py-2 pr-2">
                <span class="flex items-center gap-1">
                  <input
                    type="number"
                    min="0"
                    max="100"
                    step="0.01"
                    class="input w-24"
                    :value="bpToPct(t.prob_bp)"
                    @input="t.prob_bp = pctToBp(($event.target as HTMLInputElement).value)"
                  />%
                </span>
              </td>
            </tr>
          </tbody>
          <tfoot>
            <tr>
              <td colspan="8" class="pt-3 text-right text-sm text-gray-500">合计</td>
              <td class="pt-3 text-sm font-semibold tabular-nums" :class="diff === 0 ? 'text-emerald-600' : 'text-rose-500'">
                {{ fmtBP(totalBP) }}
                <span v-if="diff > 0" class="block text-xs font-normal">还差 {{ fmtBP(diff) }}</span>
                <span v-else-if="diff < 0" class="block text-xs font-normal">超出 {{ fmtBP(-diff) }}</span>
              </td>
            </tr>
          </tfoot>
        </table>
      </div>
    </div>

    <AdminPotsCard />

    <!-- 参与条件 -->
    <div class="card p-5">
      <div class="mb-3 flex items-center justify-between">
        <div>
          <p class="font-medium text-gray-900 dark:text-white">参与条件</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">按顺序检查，不满足的条件会在用户端提示原因；积分不足是固有条件，无需配置</p>
        </div>
        <select v-if="addableRules.length" class="input w-44" :value="''" @change="addRule(($event.target as HTMLSelectElement).value)">
          <option value="" disabled>+ 添加条件</option>
          <option v-for="m in addableRules" :key="m.type" :value="m.type">{{ m.name }}</option>
        </select>
      </div>
      <p v-if="!cfg.rules.length" class="text-sm text-gray-400">无条件，积分足够即可摇奖</p>
      <div v-for="(r, i) in cfg.rules" :key="r.type" class="flex flex-wrap items-center gap-4 border-t border-gray-100 py-3 first:border-0 dark:border-dark-700">
        <label class="flex w-36 items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-300">
          <input v-model="r.enabled" type="checkbox" class="accent-primary-500" />
          {{ ruleMeta(r.type)?.name ?? r.type }}
        </label>
        <label v-for="p in ruleMeta(r.type)?.params ?? []" :key="p.key" class="flex items-center gap-2 text-sm text-gray-500">
          {{ p.label }}
          <input v-model.number="r.params[p.key]" type="number" :min="p.min" :max="p.max" :step="p.step" class="input w-24" />
        </label>
        <div class="ml-auto flex gap-1">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="i === 0" aria-label="上移" @click="moveRule(i)">↑</button>
          <button type="button" class="btn btn-secondary btn-sm" @click="cfg.rules.splice(i, 1)">删除</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Toggle from '@/components/common/Toggle.vue'
import { fmtBP, type SlotTier } from '../slot/api'
import AdminPotsCard from './AdminPotsCard.vue'
import { FULL_BP, bpToPct, park, pctToBp } from './adminState'

const st = computed(() => park.slot)
const cfg = computed(() => park.slot?.config)

const totalBP = computed(() => cfg.value?.tiers.reduce((s, t) => s + (t.prob_bp || 0), 0) ?? 0)
const diff = computed(() => FULL_BP - totalBP.value)
const noneIdx = computed(() => cfg.value?.tiers.findIndex((t) => t.prize_type === 'none') ?? -1)

function fillNone() {
  const t = cfg.value?.tiers[noneIdx.value]
  if (t) t.prob_bp = Math.max(0, (t.prob_bp || 0) + diff.value)
}

function onTypeChange(t: SlotTier) {
  if (t.prize_type === 'none') t.pot = ''
  else if (!t.pot) t.pot = 'small'
  if (t.prize_type === 'points' && !t.prize_points) t.prize_points = 50
  if (t.prize_type === 'balance' && !t.prize_balance) t.prize_balance = 1
}

const ruleMeta = (type: string) => st.value?.rule_catalog.find((m) => m.type === type)
const addableRules = computed(() => st.value?.rule_catalog.filter((m) => !cfg.value?.rules.some((r) => r.type === m.type)) ?? [])

function addRule(type: string) {
  const meta = ruleMeta(type)
  if (!meta || !cfg.value) return
  cfg.value.rules.push({ type, enabled: true, params: Object.fromEntries(meta.params.map((p) => [p.key, p.default])) })
}

function moveRule(i: number) {
  const list = cfg.value!.rules
  ;[list[i - 1], list[i]] = [list[i], list[i - 1]]
}
</script>
