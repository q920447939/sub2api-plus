<!-- [plus] 新建 / 编辑邮件模板。目的决定可用变量和是否必须带按钮；目的创建后不可改。 -->
<template>
  <AppLayout>
    <div v-if="loadError" class="card p-16 text-center text-sm text-gray-500">{{ loadError }}</div>
    <div v-else class="space-y-6 pb-16">
      <div class="flex flex-wrap items-center gap-3">
        <button type="button" class="btn btn-secondary" @click="router.push('/admin/outreach/templates')">← 返回</button>
        <h1 class="flex-1 text-lg font-semibold text-gray-900 dark:text-white">
          {{ isNew ? '新建模板' : '编辑模板' }}
          <span v-if="builtin" class="badge badge-gray ml-2 align-middle">内置</span>
        </h1>
        <button type="button" class="btn btn-primary" :disabled="busy" @click="save">保存</button>
      </div>
      <div v-if="builtin" class="rounded-lg border border-blue-200 bg-blue-50 p-4 text-sm text-blue-800 dark:border-blue-800/50 dark:bg-blue-900/20 dark:text-blue-200">
        内置模板不能删除：「邮箱验证」页单发邀请等直发邮件使用这个目的的内置模板。修改后立即对之后的直发生效。
      </div>

      <div class="grid items-start gap-6 xl:grid-cols-2">
        <section class="card space-y-4 p-6">
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label class="input-label">模板名称</label>
              <input v-model="form.name" class="input" maxlength="60" />
            </div>
            <div>
              <label class="input-label">目的</label>
              <Select v-model="form.purpose" :options="purposeOptions" :disabled="!isNew" />
              <p class="input-hint">{{ isNew ? purpose?.description : '创建后不可修改（变量集合由目的决定）' }}</p>
            </div>
          </div>
          <div>
            <label class="input-label">外壳</label>
            <Select v-model="layoutValue" :options="layoutOptions" />
          </div>
          <div>
            <label class="input-label">邮件主题</label>
            <input v-model="form.subject" class="input" maxlength="200" />
          </div>
          <div>
            <label class="input-label">正文（Markdown）</label>
            <textarea v-model="form.body_md" rows="16" class="input font-mono text-sm" />
            <p v-if="purpose?.requires_cta && !form.body_md.includes(CTA)" class="mt-1 text-xs text-amber-600">「{{ purpose.label }}」的正文必须有按钮 {{ CTA }}，保存会被拒绝。</p>
          </div>
          <VariableChips :variables="purpose?.variables ?? []" />
          <p class="text-xs text-gray-400">权益变量（如赠送积分数）只在活动附带了对应权益时可用，请在活动里使用。</p>
        </section>
        <MailFrame class="xl:sticky xl:top-4" :subject="rendered.subject" :html="rendered.html" />
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import MailFrame from '../components/MailFrame.vue'
import VariableChips from '../components/VariableChips.vue'
import { useMailRender } from '../useMailRender'
import { adminApi, useOutreachMeta, CTA, type Layout, type TemplateInput } from '../api'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const { meta, purposeOf } = useOutreachMeta()

const id = ref<number | null>(route.params.id ? Number(route.params.id) : null)
const isNew = computed(() => id.value === null)
const builtin = ref(false)
const busy = ref(false)
const loadError = ref('')
const form = reactive<TemplateInput>({ name: '', purpose: String(route.query.purpose || 'news'), subject: '', body_md: '', layout_id: null })

const purpose = computed(() => purposeOf(form.purpose))
const purposeOptions = computed(() => (meta.value?.purposes ?? []).map((p) => ({ value: p.key, label: p.label })))
const layouts = ref<Layout[]>([])
const layoutValue = computed({ get: () => form.layout_id ?? 0, set: (v: number) => (form.layout_id = v || null) })
const layoutOptions = computed(() => [
  { value: 0, label: '默认外壳' },
  ...layouts.value.filter((l) => !l.is_default).map((l) => ({ value: l.id, label: l.name }))
])

const rendered = useMailRender(() => ({ purpose: form.purpose, subject: form.subject, body_md: form.body_md, layout_id: form.layout_id }))

async function save() {
  busy.value = true
  try {
    const input = { ...form }
    if (id.value === null) {
      const t = await adminApi.createTemplate(input)
      id.value = t.id
      router.replace(`/admin/outreach/templates/${t.id}`)
    } else {
      await adminApi.updateTemplate(id.value, input)
    }
    appStore.showSuccess('已保存')
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '保存失败'))
  } finally {
    busy.value = false
  }
}

onMounted(async () => {
  adminApi.layouts().then((l) => (layouts.value = l)).catch(() => {})
  const src = id.value ?? (route.query.from ? Number(route.query.from) : null)
  if (src === null) {
    form.subject = '【{{site_name}}】'
    return
  }
  try {
    const t = await adminApi.getTemplate(src)
    Object.assign(form, { name: t.name, purpose: t.purpose, subject: t.subject, body_md: t.body_md, layout_id: t.layout_id })
    if (id.value === null) form.name = `${t.name} (副本)`.slice(0, 60)
    else builtin.value = t.builtin
  } catch (e) {
    loadError.value = extractApiErrorMessage(e, '加载失败')
  }
})
</script>
