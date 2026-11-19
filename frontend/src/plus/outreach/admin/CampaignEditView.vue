<!-- [plus] 新建 / 编辑活动：目的 → 人群 → 内容（模板 + 外壳）→ 按钮与权益。活动只是定义，不发信。 -->
<template>
  <AppLayout>
    <div v-if="loadError" class="card p-16 text-center text-sm text-gray-500">{{ loadError }}</div>
    <div v-else class="space-y-6 pb-16">
      <div class="flex flex-wrap items-center gap-3">
        <button type="button" class="btn btn-secondary" @click="back">← 返回</button>
        <h1 class="flex-1 text-lg font-semibold text-gray-900 dark:text-white">{{ isNew ? '新建活动' : '编辑活动' }}</h1>
        <button type="button" class="btn btn-secondary" :disabled="busy" @click="openTest">发送测试邮件</button>
        <button type="button" class="btn btn-primary" :disabled="busy" @click="save">保存</button>
      </div>

      <div v-if="batchCount > 0" class="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-800/50 dark:bg-amber-900/20 dark:text-amber-200">
        这个活动已经生成过 {{ batchCount }} 个批次。批次保存的是生成时的快照，<strong>这里的修改只影响之后生成的批次</strong>。
      </div>
      <div v-else class="rounded-lg border border-blue-200 bg-blue-50 p-4 text-sm text-blue-800 dark:border-blue-800/50 dark:bg-blue-900/20 dark:text-blue-200">
        活动只保存定义，保存后不会发出任何邮件。到活动详情「生成批次」，核对名单和每个人的邮件后点「开始执行」才会发送。
      </div>

      <div class="grid items-start gap-6 xl:grid-cols-2">
        <div class="space-y-6">
          <!-- 1. 目的 -->
          <section class="card space-y-4 p-6">
            <h2 class="text-sm font-medium text-gray-900 dark:text-white">1. 目的</h2>
            <div>
              <label class="input-label">活动名称（仅内部可见）</label>
              <input v-model="form.name" class="input" maxlength="120" placeholder="例如：11 月沉默老客召回" />
            </div>
            <div>
              <label class="input-label">邮件目的</label>
              <Select v-model="form.purpose" :options="purposeOptions" @update:model-value="onPurposeChange" />
              <p v-if="purpose" class="input-hint">{{ purpose.description }}</p>
            </div>
          </section>

          <!-- 2. 人群 -->
          <section class="card space-y-4 p-6">
            <div class="flex items-center gap-3">
              <h2 class="flex-1 text-sm font-medium text-gray-900 dark:text-white">2. 人群</h2>
              <button v-for="p in presets" :key="p.key" type="button" class="btn btn-secondary btn-sm" :title="p.hint" @click="applyPreset(p)">{{ p.label }}</button>
            </div>
            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label class="input-label">充值情况</label>
                <Select v-model="form.segment.recharged" :options="rechargedOptions" />
              </div>
              <div>
                <label class="input-label">不活跃天数 ≥</label>
                <input v-model.number="form.segment.inactive_days" type="number" min="0" class="input" />
                <p class="input-hint">按最近登录 / 调用时间；0 = 不限</p>
              </div>
              <div>
                <label class="input-label">账户余额 &gt;</label>
                <input v-model.number="form.segment.min_balance" type="number" min="0" step="0.01" class="input" />
              </div>
              <div>
                <label class="input-label">注册满天数 ≥</label>
                <input v-model.number="form.segment.registered_days" type="number" min="0" class="input" />
              </div>
            </div>
            <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-800">
              <div class="flex items-center gap-3">
                <button type="button" class="btn btn-secondary btn-sm" :disabled="previewing" @click="loadAudience">{{ previewing ? '计算中…' : '预估人数' }}</button>
                <span v-if="audience" class="text-sm text-gray-700 dark:text-dark-200">
                  目标人群 <strong class="text-primary-600">{{ audience.eligible }}</strong> 人
                  <span class="text-gray-400">（命中运营条件 {{ audience.matched }} 人）</span>
                </span>
              </div>
              <template v-if="audience">
                <p class="mt-3 text-xs font-medium text-gray-600 dark:text-dark-300">完整规则</p>
                <ul class="mt-1 space-y-0.5 text-xs text-gray-600 dark:text-dark-300">
                  <li v-for="r in audience.rules" :key="r">· {{ r }}</li>
                </ul>
                <ul v-if="excludedLines.length" class="mt-3 space-y-0.5 text-xs text-gray-500 dark:text-dark-400">
                  <li v-for="l in excludedLines" :key="l">· {{ l }}</li>
                </ul>
                <ul v-if="noteLines.length" class="mt-2 space-y-0.5 text-xs text-blue-600 dark:text-blue-400">
                  <li v-for="l in noteLines" :key="l">· {{ l }}</li>
                </ul>
                <p v-if="audience.samples.length" class="mt-3 break-all text-xs text-gray-500">抽样：{{ audience.samples.map((s) => s.email).join('，') }}</p>
              </template>
            </div>
          </section>

          <!-- 3. 内容 -->
          <section class="card space-y-4 p-6">
            <h2 class="text-sm font-medium text-gray-900 dark:text-white">3. 内容</h2>
            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label class="input-label">从模板开始</label>
                <div class="flex gap-2">
                  <Select v-model="pickTemplate" :options="templateOptions" class="flex-1" />
                  <button type="button" class="btn btn-secondary" :disabled="!pickTemplate" @click="applyTemplate">套用</button>
                </div>
                <p class="input-hint">
                  套用会覆盖下面的主题、正文和外壳；之后改模板不影响本活动。
                  <router-link to="/admin/outreach/templates" class="text-primary-600 hover:underline">管理模板</router-link>
                </p>
              </div>
              <div>
                <label class="input-label">外壳</label>
                <Select v-model="layoutValue" :options="layoutOptions" />
                <p class="input-hint">页头、版式、页脚品牌区。<router-link to="/admin/outreach/layouts" class="text-primary-600 hover:underline">管理外壳</router-link></p>
              </div>
            </div>
            <div>
              <label class="input-label">邮件主题</label>
              <input v-model="form.subject" class="input" maxlength="200" />
            </div>
            <div>
              <label class="input-label">正文（Markdown）</label>
              <textarea v-model="form.body_md" rows="14" class="input font-mono text-sm" />
              <p class="input-hint">支持：# 标题、- 列表、**粗体**、[文字](链接)、&gt; 引用。</p>
            </div>
            <VariableChips :variables="variables" />
          </section>

          <!-- 4. 按钮与权益 -->
          <section class="card space-y-4 p-6">
            <h2 class="text-sm font-medium text-gray-900 dark:text-white">4. 按钮与权益</h2>
            <p class="text-xs text-gray-500 dark:text-dark-400">{{ actionText }}</p>
            <div v-if="needsCTA && !hasCTA" class="flex items-center gap-3 rounded-lg bg-amber-50 p-3 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">
              <span class="flex-1">正文里还没有按钮，保存会被拒绝。</span>
              <button type="button" class="btn btn-secondary btn-sm" @click="insertCTA">插入按钮</button>
            </div>
            <div v-if="purpose?.allows_landing">
              <label class="input-label">落地页（可选）</label>
              <input v-model="form.landing_url" class="input" maxlength="500" placeholder="/pricing 或 https://本站域名/..." />
              <p class="input-hint">点按钮后到达的页面，留空为站点首页。只能是本站或「触达链接域名」下的地址。</p>
            </div>

            <template v-if="purpose?.allows_benefits">
              <div class="flex items-center gap-3 border-t border-gray-100 pt-4 dark:border-dark-700">
                <div class="min-w-0 flex-1">
                  <p class="text-sm text-gray-900 dark:text-white">附带权益（可选，可多个）</p>
                  <p class="text-xs text-gray-500 dark:text-dark-400">用户点按钮、登录后先领取全部权益，再自动去落地页。仍然只有一个按钮。</p>
                </div>
                <Select v-if="addable.length" v-model="addKind" :options="addOptions" class="w-40" @update:model-value="addBenefit" />
              </div>
              <div v-for="(b, i) in form.benefits" :key="b.kind" class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
                <div class="mb-3 flex items-center">
                  <span class="flex-1 text-sm font-medium text-gray-900 dark:text-white">{{ benefitOf(b.kind)?.label ?? b.kind }}</span>
                  <button type="button" class="text-xs text-red-500 hover:underline" @click="form.benefits.splice(i, 1)">移除</button>
                </div>
                <div v-if="!benefitOf(b.kind)" class="text-xs text-amber-600">该权益当前不可用，保存会失败，请移除。</div>
                <div v-for="f in benefitOf(b.kind)?.fields ?? []" :key="f.key">
                  <label class="input-label">{{ f.label }}</label>
                  <input v-model.number="b.config[f.key]" type="number" :min="f.min" :max="f.max" class="input" />
                  <p v-if="f.hint" class="input-hint">{{ f.hint }}</p>
                </div>
              </div>
              <div v-if="form.benefits.length">
                <label class="input-label">领取有效期（天）</label>
                <input v-model.number="form.claim_days" type="number" min="1" max="365" class="input w-40" />
                <p class="input-hint">邮件发出后多少天内可以领取。生成批次时冻结，之后修改不影响已发出的链接。每人每批次限领一次。</p>
              </div>
            </template>
          </section>
        </div>

        <MailFrame class="xl:sticky xl:top-4" :subject="rendered.subject" :html="rendered.html" />
      </div>
    </div>

    <BaseDialog :show="testDialog" title="发送测试邮件" @close="testDialog = false">
      <div class="space-y-3">
        <p class="text-sm text-gray-600 dark:text-dark-300">测试邮件只发到你自己的邮箱，不建名单、不计入统计。会先自动保存活动。</p>
        <div>
          <label class="input-label">用某个用户的真实数据渲染（可选）</label>
          <input v-model="sampleEmail" class="input" placeholder="留空则用你自己的邮箱和示例余额" />
          <p class="input-hint">填一个用户邮箱，可以看到他实际会收到的内容（用户名、余额等）。</p>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="testDialog = false">取消</button>
          <button type="button" class="btn btn-primary" :disabled="busy" @click="sendTest">{{ busy ? '发送中…' : '发送' }}</button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import MailFrame from '../components/MailFrame.vue'
import VariableChips from '../components/VariableChips.vue'
import { useMailRender } from '../useMailRender'
import {
  adminApi, emptySegment, useOutreachMeta, CTA, PRESETS,
  type AudiencePreview, type CampaignInput, type Layout, type Template
} from '../api'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const { meta, purposeOf, benefitOf } = useOutreachMeta()

const id = ref<number | null>(route.params.id ? Number(route.params.id) : null)
const isNew = computed(() => id.value === null)

const form = reactive<CampaignInput>({
  name: '', purpose: 'recall', template_id: null, layout_id: null,
  subject: '', body_md: '', segment: emptySegment(), benefits: [], landing_url: '', claim_days: 30
})
const loadError = ref('')
const busy = ref(false)
const batchCount = ref(0)

const purpose = computed(() => purposeOf(form.purpose))
const purposeOptions = computed(() => (meta.value?.purposes ?? []).map((p) => ({ value: p.key, label: `${p.label}（${p.category === 'service' ? '服务' : '营销'}）` })))
const presets = computed(() => PRESETS.filter((p) => purposeOf(p.purpose)))
const rechargedOptions = [
  { value: '', label: '不限' },
  { value: 'yes', label: '充值过' },
  { value: 'no', label: '没充值过' }
]

function onPurposeChange() {
  const p = purpose.value
  if (p && !p.allows_benefits) form.benefits = []
  if (p && !p.allows_landing) form.landing_url = ''
  audience.value = null
  loadTemplates(true)
}

function applyPreset(p: (typeof PRESETS)[number]) {
  const changed = p.purpose !== form.purpose
  form.purpose = p.purpose
  form.segment = { ...p.segment }
  if (!form.name) form.name = p.label
  if (changed) onPurposeChange()
  audience.value = null
}

// ─── 模板 / 外壳 ─────────────────────────────────────────────────────────
const templates = ref<Template[]>([])
const layouts = ref<Layout[]>([])
const pickTemplate = ref<number | null>(null)
const templateOptions = computed(() => [
  { value: null, label: templates.value.length ? '选择模板…' : '这个目的还没有模板' },
  ...templates.value.map((t) => ({ value: t.id, label: t.builtin ? `${t.name}` : t.name }))
])
const layoutValue = computed({
  get: () => form.layout_id ?? 0,
  set: (v: number) => (form.layout_id = v || null)
})
const layoutOptions = computed(() => [
  { value: 0, label: `默认外壳${defaultLayoutName.value ? `（${defaultLayoutName.value}）` : ''}` },
  ...layouts.value.filter((l) => !l.is_default).map((l) => ({ value: l.id, label: l.name }))
])
const defaultLayoutName = computed(() => layouts.value.find((l) => l.is_default)?.name ?? '')

/** autoApply：新建活动、正文还空着时，直接套用该目的的内置模板 */
async function loadTemplates(autoApply = false) {
  try {
    templates.value = await adminApi.templates(form.purpose)
    pickTemplate.value = templates.value.find((t) => t.builtin)?.id ?? templates.value[0]?.id ?? null
    if (autoApply && isNew.value && pickTemplate.value) applyTemplate()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '加载模板失败'))
  }
}

function applyTemplate() {
  const t = templates.value.find((x) => x.id === pickTemplate.value)
  if (!t) return
  form.template_id = t.id
  form.subject = t.subject
  form.body_md = t.body_md
  form.layout_id = t.layout_id
}

// ─── 按钮 / 权益 ─────────────────────────────────────────────────────────
const hasCTA = computed(() => form.body_md.includes(CTA))
const needsCTA = computed(() => !!purpose.value?.requires_cta || form.benefits.length > 0)
const actionText = computed(() => {
  const p = purpose.value
  if (!p) return ''
  if (!p.allows_landing) return `「${p.label}」的按钮固定去一次性验证页，不需要配置去向。`
  const tail = form.benefits.length ? '先领取下面的权益，再去落地页。' : '直接去落地页。'
  return `正文里的按钮（${CTA}）点击后${tail}${p.requires_cta ? '' : '这个目的可以不放按钮。'}`
})

function insertCTA() {
  form.body_md = `${form.body_md.trimEnd()}\n\n[[${form.benefits.length ? '领取并查看' : '去看看'}]](${CTA})`
}

const addKind = ref<string | null>('')
const addable = computed(() => (meta.value?.benefits ?? []).filter((b) => !form.benefits.some((x) => x.kind === b.kind)))
const addOptions = computed(() => [{ value: '', label: '＋ 添加权益' }, ...addable.value.map((b) => ({ value: b.kind, label: b.label }))])

function addBenefit(kind: string | number | boolean | null) {
  const m = typeof kind === 'string' ? benefitOf(kind) : undefined
  if (m) {
    const config: Record<string, number> = {}
    for (const f of m.fields) config[f.key] = f.min
    form.benefits.push({ kind: m.kind, config })
  }
  addKind.value = ''
}

const variables = computed(() => [
  ...(purpose.value?.variables ?? []),
  ...form.benefits.flatMap((b) => benefitOf(b.kind)?.variables ?? [])
])

// ─── 人群预估 ─────────────────────────────────────────────────────────────
const audience = ref<AudiencePreview | null>(null)
const previewing = ref(false)
const excludedLines = computed(() => {
  const e = audience.value?.excluded
  if (!e) return []
  const out: string[] = []
  if (e.invalid) out.push(`邮箱已标记无效：${e.invalid} 人`)
  if (e.wrong_state) out.push(`邮箱状态不符（需要「${purpose.value?.state_text ?? ''}」）：${e.wrong_state} 人`)
  if (e.unsubscribed) out.push(`已退订此类邮件：${e.unsubscribed} 人`)
  return out.length ? ['不在目标人群（已扣除）：', ...out] : []
})
const noteLines = computed(() => {
  const n = audience.value?.notes
  if (!n) return []
  const out: string[] = []
  if (n.recent_mail) out.push(`${n.recent_mail} 人在频控期内：会进名单，发送时跳过`)
  if (n.reached) out.push(`${n.reached} 人已收到过本活动的邮件`)
  if (n.queued) out.push(`${n.queued} 人正在本活动的其他批次里排队`)
  return out
})

async function loadAudience() {
  previewing.value = true
  try {
    audience.value = await adminApi.preview(form.purpose, form.segment, id.value ?? 0)
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '预估失败'))
  } finally {
    previewing.value = false
  }
}

// ─── 预览（后端渲染，与实际发出同一条管线）──────────────────────────────
const rendered = useMailRender(() => ({
  purpose: form.purpose, subject: form.subject, body_md: form.body_md, layout_id: form.layout_id, benefits: form.benefits
}))

// ─── 保存 / 测试 ─────────────────────────────────────────────────────────
async function persist(): Promise<number | null> {
  busy.value = true
  try {
    const input = JSON.parse(JSON.stringify(form)) as CampaignInput
    if (id.value === null) {
      const c = await adminApi.create(input)
      id.value = c.id
      router.replace(`/admin/outreach/campaigns/${c.id}/edit`)
    } else {
      await adminApi.update(id.value, input)
    }
    return id.value
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '保存失败'))
    return null
  } finally {
    busy.value = false
  }
}

async function save() {
  const cid = await persist()
  if (cid === null) return
  appStore.showSuccess('已保存')
  router.push(`/admin/outreach/campaigns/${cid}`)
}

const testDialog = ref(false)
const sampleEmail = ref('')
const openTest = () => (testDialog.value = true)

async function sendTest() {
  if ((await persist()) === null || id.value === null) return
  busy.value = true
  try {
    const r = await adminApi.test(id.value, sampleEmail.value)
    testDialog.value = false
    appStore.showSuccess(`测试邮件已发到 ${r.sent_to}，请到邮箱确认效果（也看一下是否进了垃圾箱）`)
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '发送测试邮件失败'))
  } finally {
    busy.value = false
  }
}

function back() {
  router.push(id.value === null ? '/admin/outreach/campaigns' : `/admin/outreach/campaigns/${id.value}`)
}

onMounted(async () => {
  adminApi.layouts().then((l) => (layouts.value = l)).catch(() => {})
  if (id.value !== null) {
    try {
      const c = await adminApi.get(id.value)
      Object.assign(form, {
        name: c.name, purpose: c.purpose, template_id: c.template_id, layout_id: c.layout_id, subject: c.subject,
        body_md: c.body_md, segment: c.segment, benefits: c.benefits ?? [], landing_url: c.landing_url, claim_days: c.claim_days || 30
      })
      batchCount.value = c.batch_count
      loadTemplates()
    } catch (e) {
      loadError.value = extractApiErrorMessage(e, '加载失败')
    }
    return
  }
  // 从邮箱验证页跳来：?preset=pending_paid
  const p = PRESETS.find((x) => x.key === route.query.preset)
  if (p) {
    form.purpose = p.purpose
    form.segment = { ...p.segment }
    form.name = p.label
  }
  loadTemplates(true)
})
</script>
