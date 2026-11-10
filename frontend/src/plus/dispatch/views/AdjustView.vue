<!-- [plus] 上游调度台 · 调度调整：系统建议（附推理）、生效中的调整、调整历史和每次调整的效果对比 -->
<template>
  <AppLayout>
    <div class="space-y-4" data-testid="dispatch-adjust">
      <ModeNotice />
      <Tabs v-model="tab" :tabs="TABS" />

      <!-- 调度建议 -->
      <template v-if="tab === 'suggest'">
        <div v-for="s in suggestions" :key="s.group.id" class="card p-0" data-testid="dispatch-suggestion">
          <div class="flex flex-wrap items-center gap-2 px-4 py-3">
            <b>{{ s.group.name }}</b>
            <span class="rounded-full px-2 text-xs" :class="PLATFORM_CLASS[s.group.platform]">{{ platformName(s.group.platform) }}</span>
            <span class="text-sm" :class="s.suggestion.moved ? 'text-amber-700' : 'text-gray-500'">{{ s.suggestion.summary }}</span>
          </div>
          <div class="border-t border-gray-100 px-4 py-3 text-sm dark:border-dark-700">
            <div v-if="s.suggestion.capped" class="mb-2 text-xs text-gray-500">需挪 {{ s.suggestion.wanted }}%，超过每小时上限，本小时挪 {{ cfg?.max_move }}%</div>
            <table class="w-full text-sm">
              <thead><tr class="text-left text-xs text-gray-500"><th class="py-1 font-normal">账号</th><th class="font-normal">当前份额</th><th class="font-normal">建议份额</th><th class="font-normal">调整依据</th><th class="font-normal">当前调整</th><th /></tr></thead>
              <tbody>
                <tr v-for="r in s.suggestion.rows" :key="r.account_id" class="border-t border-gray-50 dark:border-dark-700">
                  <td class="py-1.5"><b>{{ s.names[r.account_id] }}</b></td>
                  <td><ShareBar :value="r.before" /></td>
                  <td><ShareBar :value="r.after" tone="sug" /></td>
                  <td class="max-w-[420px] text-xs">
                    <template v-if="r.reasons.length"><div v-for="(x, i) in r.reasons" :key="i">{{ x.dimension }}{{ LEVEL_TEXT[x.level as Level] }}（{{ x.evidence }}）→ {{ x.action }}</div></template>
                    <span v-else class="text-gray-400">{{ KIND[r.kind] ?? '正常' }}</span>
                  </td>
                  <td class="text-xs">{{ s.adjustments[r.account_id] ? adjustText(s.adjustments[r.account_id].action, s.adjustments[r.account_id].factor) : '—' }}</td>
                  <td class="text-right">
                    <span v-if="r.settled" class="text-xs text-gray-400">已调整到位</span>
                    <button v-else-if="r.action !== 'normal'" type="button" class="btn btn-primary btn-sm" @click="openAdjust(s, r)">采纳…</button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </template>

      <!-- 生效中 -->
      <template v-else-if="tab === 'active'">
        <div v-if="!active.length" class="py-16 text-center text-sm text-gray-400">没有生效中的调整</div>
        <div v-for="a in active" :key="a.adjustment.group_id + '-' + a.adjustment.account_id" class="card flex flex-wrap items-center gap-3 p-4" data-testid="dispatch-active-adjust">
          <span class="rounded bg-blue-50 px-2 py-1 text-sm font-semibold text-blue-700">{{ a.text }}</span>
          <div class="min-w-[260px] flex-1">
            <div><b>{{ a.account }}</b> <span class="text-xs text-gray-500">· {{ a.group }} · 已持续 {{ a.duration }} · 现在流量 {{ a.share }}%</span></div>
            <div class="text-xs text-gray-500">原因：{{ a.adjustment.reason || '—' }}（{{ a.adjustment.source === 'suggestion' ? '采纳系统建议' : '人工' }}）</div>
            <div class="text-xs">现在：<LevelPill :level="a.worst" /> {{ a.one_line }}</div>
            <div v-if="a.restore_hint" class="mt-1 rounded bg-green-50 px-2 py-1 text-xs text-green-800">建议恢复：{{ a.restore_hint }}</div>
          </div>
          <router-link v-if="a.adjustment.log_id" class="btn btn-secondary btn-sm" :to="{ query: { tab: 'history', log: a.adjustment.log_id } }">效果对比</router-link>
          <button type="button" class="btn btn-secondary btn-sm" @click="restore(a)">恢复正常</button>
          <button type="button" class="btn btn-secondary btn-sm" @click="openAdjustActive(a)">改</button>
        </div>
      </template>

      <!-- 历史 + 效果 -->
      <template v-else>
        <div class="grid gap-4 xl:grid-cols-[420px_1fr]">
          <div class="card max-h-[75vh] overflow-y-auto p-0">
            <div v-if="!logs.length" class="py-16 text-center text-sm text-gray-400">还没有调整</div>
            <button v-for="l in logs" :key="l.log.id" type="button" class="block w-full border-b border-gray-100 px-4 py-2.5 text-left text-sm dark:border-dark-700" :class="l.log.id === logId ? 'bg-primary-50 dark:bg-primary-900/20' : 'hover:bg-gray-50 dark:hover:bg-dark-700'" @click="logId = l.log.id">
              <div><b>{{ l.account }}</b> <span class="text-xs text-gray-500">· {{ l.group }}</span></div>
              <div class="text-xs">{{ l.from }} → <b>{{ l.to }}</b> · {{ l.log.operator }} · {{ fullTime(l.log.created_at) }}</div>
              <div class="truncate text-xs text-gray-400">{{ l.log.reason }}</div>
            </button>
          </div>
          <EffectPanel v-if="logId" :log-id="logId" />
          <div v-else class="py-16 text-center text-sm text-gray-400">选择一条调整查看效果</div>
        </div>
      </template>
    </div>
    <AdjustDialog :target="adjustTarget" @close="adjustTarget = null" @done="onDone" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type Level, type SuggestRow } from '../api'
import AdjustDialog, { type AdjustTarget } from '../shared/AdjustDialog.vue'
import EffectPanel from '../shared/EffectPanel.vue'
import LevelPill from '../shared/LevelPill.vue'
import ModeNotice from '../shared/ModeNotice.vue'
import Tabs from '../shared/Tabs.vue'
import ShareBar from '../shared/ShareBar.vue'
import { adjustText, fullTime, LEVEL_TEXT, PLATFORM_CLASS, platformName } from '../shared/format'
import { loadMeta, meta } from '../shared/meta'

const TABS = [{ k: 'suggest', n: '系统建议' }, { k: 'active', n: '生效中' }, { k: 'history', n: '调整历史' }]
const KIND: Record<string, string> = { keep: '正常', few: '数据不足', idle: '无流量' }
const route = useRoute()
const router = useRouter()
const app = useAppStore()
const tab = ref(String(route.query.tab ?? 'suggest'))
const logId = ref<number | null>(Number(route.query.log) || null)
const suggestions = ref<any[]>([])
const active = ref<any[]>([])
const logs = ref<any[]>([])
const adjustTarget = ref<AdjustTarget | null>(null)
const cfg = computed(() => meta.value?.config)

async function load() {
  try {
    void loadMeta(true)
    if (tab.value === 'suggest') suggestions.value = (await dispatchApi.suggestions()) ?? []
    if (tab.value === 'active') active.value = (await dispatchApi.adjustments()) ?? []
    if (tab.value === 'history') {
      logs.value = (await dispatchApi.adjustmentLogs()) ?? []
      if (!logId.value && logs.value.length) logId.value = logs.value[0].log.id
    }
  } catch (e) {
    app.showError(extractApiErrorMessage(e, '加载失败'))
  }
}
watch(() => route.query, (qq) => {
  if (qq.tab) tab.value = String(qq.tab)
  if (qq.log) logId.value = Number(qq.log)
})
watch(tab, (t) => {
  void router.replace({ query: { ...route.query, tab: t } })
  void load()
}, { immediate: true })

const names = () => Object.fromEntries((meta.value?.accounts ?? []).map((a) => [String(a.id), a.name]))

function openAdjust(s: any, r: SuggestRow) {
  adjustTarget.value = { groupId: s.group.id, groupName: s.group.name, accountId: r.account_id, accountName: s.names[r.account_id],
    current: s.adjustments[r.account_id] ?? null, suggest: r, oneLine: r.reasons.map((x) => `${x.dimension}：${x.evidence}`).join('；'), names: s.names }
}
function openAdjustActive(a: any) {
  adjustTarget.value = { groupId: a.adjustment.group_id, groupName: a.group, accountId: a.adjustment.account_id, accountName: a.account,
    current: a.adjustment, worst: a.worst, oneLine: a.one_line, names: names() }
}
async function restore(a: any) {
  try {
    await dispatchApi.applyAdjust({ group_id: a.adjustment.group_id, account_id: a.adjustment.account_id, action: 'normal', factor: 1,
      reason: a.restore_hint ? '依据已消失，恢复' : '人工恢复' })
    app.showSuccess('已恢复正常')
    await load()
  } catch (e) {
    app.showError(extractApiErrorMessage(e, '恢复失败'))
  }
}
function onDone(id: number) {
  adjustTarget.value = null
  logId.value = id
  tab.value = 'history'
  void load()
}
</script>
