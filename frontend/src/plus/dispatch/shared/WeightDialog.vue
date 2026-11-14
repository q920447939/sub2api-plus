<!-- [plus] 调整权重：账号（或它的某个模型）在每个所在分组承接多少。
     滑块的值就是它在这个分组承接的百分比（同组加起来 100%），右边是它和同组其他账号「现在 → 预计」。 -->
<template>
  <BaseDialog :show="show" title="调整权重" width="wide" @close="close">
    <div v-if="!sheet" class="py-12 text-center text-sm text-gray-400">{{ error || '加载中…' }}</div>
    <div v-else data-testid="dispatch-weight">
      <div class="flex flex-wrap items-center gap-3">
        <div class="text-sm text-gray-600 dark:text-dark-300">
          <b class="text-gray-900 dark:text-white">{{ sheet.name }}</b>
          <span v-if="sheet.model" class="ml-1 font-mono">· {{ [sheet.model, ...sheet.also].join('、') }}</span>
        </div>
        <div v-if="rows.length > 1" class="ml-auto inline-flex rounded-lg bg-gray-100 p-0.5 dark:bg-dark-700">
          <button v-for="a in ALL" :key="a.k" type="button" class="rounded-md px-3 py-1 text-xs text-gray-600 hover:bg-white dark:text-dark-200 dark:hover:bg-dark-900" @click="setAll(a.k)">{{ a.n }}</button>
        </div>
      </div>
      <div v-if="!rows.length" class="py-10 text-center text-sm text-gray-400">它在所在分组里近 24 小时没有流量，没有可调的</div>
      <template v-else>
        <div class="mt-4 grid grid-cols-[minmax(0,1.1fr)_minmax(0,1.5fr)_minmax(0,1.6fr)] gap-x-5 px-0.5 text-xs text-gray-400">
          <span>分组</span><span>承接多少</span><span>现在 → 预计</span>
        </div>
        <div v-for="g in sheet.groups" :key="g.group_id" class="grid grid-cols-[minmax(0,1.1fr)_minmax(0,1.5fr)_minmax(0,1.6fr)] items-center gap-x-5 gap-y-2 border-t border-gray-100 px-0.5 py-3 dark:border-dark-700">
          <div>
            <div class="text-sm font-semibold leading-tight">{{ g.name }}</div>
            <div class="text-xs text-gray-500">{{ modeText(draft(g.group_id)) }}</div>
          </div>
          <div class="flex items-center gap-2">
            <button type="button" class="rounded-md px-2 py-0.5 text-xs" :class="draft(g.group_id).mode === 'pause' ? 'bg-gray-900 font-semibold text-white dark:bg-white dark:text-gray-900' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-200'" @click="setMode(g.group_id, 'pause')">暂停</button>
            <span v-if="fixed(g)" class="flex-1 text-center text-xs text-gray-400">{{ g.only ? '只有它一个账号' : '只有它在跑' }}</span>
            <span v-else class="relative flex min-w-0 flex-1 items-center">
              <input
                type="range" min="0" max="100" step="1" class="w-full accent-primary-600" :class="draft(g.group_id).mode === 'share' ? '' : 'opacity-50 accent-gray-400'"
                :value="sliderVal(g)" @input="onSlide(g, $event)" @change="preview()"
              />
              <i v-if="minOf(g) > 0" class="absolute -bottom-1 left-0 h-[3px] rounded bg-gray-400" :style="{ width: minOf(g) + '%' }" :title="`这 ${minOf(g)}% 只有它能跑，挪不走`" />
            </span>
            <button type="button" class="rounded-md px-2 py-0.5 text-xs" :class="draft(g.group_id).mode === 'normal' ? 'bg-gray-900 font-semibold text-white dark:bg-white dark:text-gray-900' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-200'" @click="setMode(g.group_id, 'normal')">正常</button>
          </div>
          <div>
            <div class="text-sm font-semibold tabular-nums">{{ pct(g.now.pct) }} <span class="text-gray-400">→</span> <b>{{ pct(g.next.pct) }}</b></div>
            <div class="mt-0.5 flex flex-wrap gap-x-3 text-xs text-gray-500">
              <span v-for="o in others(g)" :key="o.id" class="whitespace-nowrap">{{ name(o.id) }} <span class="tabular-nums">{{ pct(o.now) }} → <b class="text-gray-800 dark:text-gray-200">{{ pct(o.next) }}</b></span></span>
            </div>
            <div v-if="g.next.dead.length" class="mt-1 text-xs font-semibold text-red-600">{{ g.only ? '这个分组只有它一个账号，停掉后请求会全部失败' : `${someNames(g.next.dead)} 在这个分组将没有账号可用` }}</div>
            <div v-else-if="draft(g.group_id).mode === 'pause' && g.next.kept.length" class="mt-1 text-xs text-gray-500">{{ g.only ? '这个分组只有它一个账号' : `${someNames(g.next.kept)} 只有它在跑` }}，这部分照常给它</div>
            <div v-else-if="draft(g.group_id).mode === 'share' && hint(g)" class="mt-1 text-xs text-gray-500">{{ hint(g) }}</div>
          </div>
          <div v-if="draft(g.group_id).mode === 'pause' && g.now.locked > 0.5" class="col-span-3 flex items-center justify-end gap-2">
            <span class="text-xs text-gray-500">只有它能跑的部分</span>
            <div class="inline-flex rounded-lg bg-gray-100 p-0.5 dark:bg-dark-700">
              <button v-for="h in [false, true]" :key="String(h)" type="button" class="rounded-md px-3 py-1 text-xs" :class="draft(g.group_id).hard === h ? 'bg-white font-semibold shadow-sm dark:bg-dark-900' : 'text-gray-500'" @click="setHard(g.group_id, h)">{{ h ? '一起停掉' : '保留' }}</button>
            </div>
          </div>
        </div>
      </template>
      <input v-model="note" class="input mt-4" placeholder="备注（可不填）" />
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="close">取消</button>
        <button type="button" class="btn" :class="sheet?.danger ? 'btn-danger' : 'btn-primary'" :disabled="!sheet?.changed || saving" data-testid="dispatch-weight-save" @click="trySave">
          {{ sheet?.danger ? '仍然停掉' : '保存' }}
        </button>
      </div>
    </template>
  </BaseDialog>
  <ConfirmDialog
    :show="confirming" title="确认停掉" danger confirm-text="停掉，让这些请求失败"
    :message="dangerText" @confirm="save" @cancel="confirming = false"
  />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type Sheet, type SheetRow } from '../api'
import { pct, someNames } from './format'

export interface WeightTarget {
  accountId: number
  model?: string
  also?: string[]
  note?: string
  pauseAll?: boolean // 打开时就把所有分组设成暂停（「把这些模型从它身上挪走」）
}

const props = defineProps<{ target: WeightTarget | null }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const appStore = useAppStore()

const ALL = [{ k: 'pause', n: '全部暂停' }, { k: 'half', n: '全部减半' }, { k: 'normal', n: '全部正常' }] as const
const show = computed(() => !!props.target)
const sheet = ref<Sheet | null>(null)
const rows = ref<SheetRow[]>([])
const note = ref('')
const error = ref('')
const saving = ref(false)
const confirming = ref(false)
let seq = 0

watch(
  () => props.target,
  async (t) => {
    sheet.value = null
    error.value = ''
    if (!t) return
    note.value = t.note ?? ''
    try {
      const s = await dispatchApi.sheet(t.accountId, t.model ?? '')
      rows.value = s.groups.map((g) => ({ ...g.row }))
      if (t.pauseAll || t.also?.length) {
        if (t.pauseAll) rows.value.forEach((r) => Object.assign(r, { mode: 'pause', hard: false }))
        sheet.value = s
        await preview()
      } else sheet.value = s
    } catch (e) {
      error.value = extractApiErrorMessage(e)
    }
  },
  { immediate: true }
)

const draft = (gid: number) => rows.value.find((r) => r.group_id === gid) ?? { group_id: gid, mode: 'normal' as const, pct: 0, hard: false }
const name = (id: number) => sheet.value?.names.accounts[String(id)] ?? `#${id}`
const minOf = (g: Sheet['groups'][number]) => Math.ceil(g.now.locked)
const maxOf = (g: Sheet['groups'][number]) => Math.floor(g.now.reach)
const fixed = (g: Sheet['groups'][number]) => maxOf(g) - minOf(g) < 1 // 它能接的都只有它在接：只能选停不停
const sliderVal = (g: Sheet['groups'][number]) => {
  const d = draft(g.group_id)
  return d.mode === 'pause' ? minOf(g) : Math.round(d.mode === 'normal' ? g.now.pct : d.pct)
}
const modeText = (d: SheetRow) => (d.mode === 'normal' ? '正常' : d.mode === 'pause' ? (d.hard ? '暂停（全停）' : '暂停') : `权重 ${Math.round(d.pct)}%`)

function others(g: Sheet['groups'][number]) {
  return g.next.rows
    .filter((r) => r.account_id !== sheet.value?.account_id)
    .map((r) => ({ id: r.account_id, now: g.now.rows.find((x) => x.account_id === r.account_id)?.pct ?? 0, next: r.pct }))
    .filter((r) => r.now > 0.5 || r.next > 0.5)
}

function hint(g: Sheet['groups'][number]): string {
  if (g.only) return '这个分组只有它一个账号，调不动'
  const bits: string[] = []
  if (g.now.locked > 0.5) bits.push(`${someNames(g.now.alone)} 只有它在跑，最低 ${pct(Math.ceil(g.now.locked))}`)
  if (g.now.reach < 99.5) bits.push(`有的模型它不提供，最多 ${pct(Math.floor(g.now.reach))}`)
  return bits.join('；')
}

const dangerText = computed(() => {
  const dead = (sheet.value?.groups ?? []).filter((g) => g.next.dead.length).map((g) => `「${g.name}」的 ${someNames(g.next.dead)}`)
  return `保存后 ${dead.join('、')} 将没有账号可用，用户的这些请求会直接失败。确定要停掉吗？`
})

function req() {
  const t = props.target!
  return { account_id: t.accountId, model: t.model ?? '', also: t.also ?? [], rows: rows.value, note: note.value }
}

async function preview() {
  const my = ++seq
  try {
    const s = await dispatchApi.sheetPreview(req())
    if (my === seq) sheet.value = s
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e))
  }
}

let timer: ReturnType<typeof setTimeout> | undefined
function onSlide(g: Sheet['groups'][number], ev: Event) {
  const el = ev.target as HTMLInputElement
  let v = Number(el.value)
  v = Math.max(minOf(g), Math.min(maxOf(g), v)) // 只有它能跑的部分挪不走；它不提供的模型给不到
  el.value = String(v)
  Object.assign(draft(g.group_id), { mode: 'share', pct: v, hard: false })
  clearTimeout(timer)
  timer = setTimeout(preview, 150)
}

function setMode(gid: number, mode: SheetRow['mode']) {
  Object.assign(draft(gid), { mode, hard: false })
  void preview()
}

function setHard(gid: number, hard: boolean) {
  draft(gid).hard = hard
  void preview()
}

function setAll(k: 'pause' | 'half' | 'normal') {
  for (const g of sheet.value?.groups ?? []) {
    const d = draft(g.group_id)
    if (k === 'half') {
      if (g.now.reach - g.now.locked > 0.5) Object.assign(d, { mode: 'share', pct: Math.max(g.now.locked, g.now.pct / 2) })
    } else Object.assign(d, { mode: k })
    d.hard = false
  }
  void preview()
}

function trySave() {
  if (sheet.value?.danger) confirming.value = true
  else void save()
}

async function save() {
  confirming.value = false
  saving.value = true
  try {
    await dispatchApi.sheetSave(req())
    appStore.showSuccess('已保存')
    emit('saved')
    emit('close')
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e))
  } finally {
    saving.value = false
  }
}

function close() {
  confirming.value = false
  emit('close')
}
</script>
