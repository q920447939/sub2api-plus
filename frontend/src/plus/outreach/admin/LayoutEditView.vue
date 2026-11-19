<!-- [plus] 新建 / 编辑邮件外壳：按块配置，右侧用示例邮件实时预览（同一条渲染管线）。 -->
<template>
  <AppLayout>
    <div v-if="loadError" class="card p-16 text-center text-sm text-gray-500">{{ loadError }}</div>
    <div v-else class="space-y-6 pb-16">
      <div class="flex flex-wrap items-center gap-3">
        <button type="button" class="btn btn-secondary" @click="router.push('/admin/outreach/layouts')">← 返回</button>
        <h1 class="flex-1 text-lg font-semibold text-gray-900 dark:text-white">{{ isNew ? '新建外壳' : '编辑外壳' }}</h1>
        <button type="button" class="btn btn-primary" :disabled="busy" @click="save">保存</button>
      </div>
      <div v-if="usedBy > 0" class="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-800/50 dark:bg-amber-900/20 dark:text-amber-200">
        有 {{ usedBy }} 个模板或活动在用这个外壳，修改会影响它们之后生成的批次和直发邮件；已生成的批次不受影响。
      </div>

      <div class="grid items-start gap-6 xl:grid-cols-2">
        <section class="card space-y-5 p-6">
          <div>
            <label class="input-label">外壳名称</label>
            <input v-model="form.name" class="input" maxlength="60" />
          </div>
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label class="input-label">页头</label>
              <Select v-model="form.config.header" :options="headerOptions" />
            </div>
            <div>
              <label class="input-label">主题色</label>
              <div class="flex items-center gap-2">
                <input v-model="form.config.theme_color" type="color" class="h-10 w-12 rounded border border-gray-200 dark:border-dark-700" />
                <input v-model="form.config.theme_color" class="input flex-1 font-mono" maxlength="7" />
              </div>
              <p class="input-hint">按钮、链接、卡片顶边</p>
            </div>
            <div v-if="form.config.header === 'text'" class="sm:col-span-2">
              <label class="input-label">页头名称（可选）</label>
              <input v-model="form.config.header_name" class="input" maxlength="60" placeholder="留空使用系统设置里的站点名" />
            </div>
            <div v-if="form.config.header === 'logo'" class="sm:col-span-2">
              <label class="input-label">Logo 图片地址</label>
              <input v-model="form.config.logo_url" class="input" placeholder="https://..." />
            </div>
            <div v-if="form.config.header !== 'none'" class="sm:col-span-2">
              <label class="input-label">页头链接（可选）</label>
              <input v-model="form.config.header_link" class="input" placeholder="https://..." />
            </div>
            <div>
              <label class="input-label">版式</label>
              <Select v-model="form.config.style" :options="styleOptions" />
            </div>
            <div>
              <label class="input-label">内容宽度（px）</label>
              <input v-model.number="form.config.width" type="number" min="480" max="800" class="input" />
            </div>
          </div>
          <div>
            <label class="input-label">页脚品牌区（Markdown）</label>
            <textarea v-model="form.config.footer_md" rows="4" class="input font-mono text-sm" />
            <p class="input-hint">版权、联系方式、社媒链接等。只能使用 {{ siteVar }}。</p>
          </div>
          <div class="rounded-lg bg-gray-50 p-4 text-xs text-gray-500 dark:bg-dark-800 dark:text-dark-400">
            页脚下方的合规区由系统按邮件目的自动追加，外壳无法修改：营销邮件为「为什么收到 + 退订链接」，服务邮件为「这是与账号相关的服务邮件」。
          </div>
        </section>

        <div class="space-y-3 xl:sticky xl:top-4">
          <div class="flex items-center gap-2 text-xs text-gray-500">
            <span>预览示例：</span>
            <Select v-model="samplePurpose" :options="sampleOptions" class="w-40" />
          </div>
          <MailFrame :subject="rendered.subject" :html="rendered.html" />
        </div>
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
import { useMailRender } from '../useMailRender'
import { adminApi, defaultLayoutConfig, useOutreachMeta, varToken, type LayoutInput } from '../api'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const { meta } = useOutreachMeta()

const id = ref<number | null>(route.params.id ? Number(route.params.id) : null)
const isNew = computed(() => id.value === null)
const busy = ref(false)
const loadError = ref('')
const usedBy = ref(0)
const siteVar = varToken('site_name')
const form = reactive<LayoutInput>({ name: '', config: defaultLayoutConfig() })

const headerOptions = [
  { value: 'text', label: '文字名称' },
  { value: 'logo', label: 'Logo 图片' },
  { value: 'none', label: '无' }
]
const styleOptions = [
  { value: 'card', label: '卡片（灰底白卡）' },
  { value: 'plain', label: '通栏（白底纯文本）' }
]

// 预览：用示例正文 + 选定目的（看营销 / 服务两种合规区的差别）
const samplePurpose = ref('recall')
const sampleOptions = computed(() => (meta.value?.purposes ?? []).map((p) => ({ value: p.key, label: `${p.label}示例` })))
const SAMPLE_BODY = `# {{username}}，您好\n\n这是一封示例邮件，用来查看外壳效果。\n\n- 列表第一项\n- 列表第二项\n\n[[示例按钮]](${varToken('cta_url')})`
const rendered = useMailRender(() => ({ purpose: samplePurpose.value, subject: '外壳预览', body_md: SAMPLE_BODY, layout: form.config }))

async function save() {
  busy.value = true
  try {
    const input = JSON.parse(JSON.stringify(form)) as LayoutInput
    if (id.value === null) {
      const l = await adminApi.createLayout(input)
      id.value = l.id
      router.replace(`/admin/outreach/layouts/${l.id}`)
    } else {
      await adminApi.updateLayout(id.value, input)
    }
    appStore.showSuccess('已保存')
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '保存失败'))
  } finally {
    busy.value = false
  }
}

onMounted(async () => {
  if (id.value === null) return
  try {
    const l = await adminApi.getLayout(id.value)
    Object.assign(form, { name: l.name, config: { ...defaultLayoutConfig(), ...l.config } })
    usedBy.value = l.used_by
  } catch (e) {
    loadError.value = extractApiErrorMessage(e, '加载失败')
  }
})
</script>
