<!-- [plus] 管理端「用户触达 · 发送设置」：只有投递规则（链接域名、限速、频控、熔断、批次保留），保存即时生效。
     统计口径由邮件目的决定，领取有效期在活动里配置，验证奖励在「邮箱验证」，页头页脚在「邮件外壳」。 -->
<template>
  <AppLayout>
    <div class="space-y-6 pb-16">
      <div v-if="!cfg" class="card p-16 text-center text-sm text-gray-400">{{ error || '正在加载…' }}</div>
      <template v-else>
        <div class="card p-4 text-sm text-gray-600 dark:text-dark-300">
          邮件通过「系统设置 → 邮件」里配置的 SMTP 发送。限速要小于服务商额度（例如 Brevo 免费档 300 封/天），并给验证码、密码重置等系统邮件留出余量。
          页头页脚在 <router-link to="/admin/outreach/layouts" class="text-primary-600 hover:underline">邮件外壳</router-link> 配置，验证奖励与验证链接有效期在
          <router-link to="/admin/email-verify" class="text-primary-600 hover:underline">邮箱验证</router-link> 配置；权益领取有效期在每个活动里配置；效果指标的统计口径由邮件目的决定，不在这里设置。
        </div>

        <div class="grid items-start gap-6 xl:grid-cols-2">
          <section v-for="g in groups" :key="g.title" class="card">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-base font-medium text-gray-900 dark:text-white">{{ g.title }}</h2>
              <p v-if="g.desc" class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ g.desc }}</p>
            </div>
            <div class="divide-y divide-gray-100 dark:divide-dark-700">
              <label v-for="f in g.fields" :key="f.key" class="flex items-center justify-between gap-4 px-6 py-4">
                <span class="min-w-0">
                  <span class="block text-sm text-gray-900 dark:text-white">{{ f.label }}</span>
                  <span v-if="f.hint" class="block text-xs text-gray-500 dark:text-dark-400">{{ f.hint }}</span>
                </span>
                <span class="flex shrink-0 items-center gap-2">
                  <input v-if="f.text" v-model="(cfg as Record<string, unknown>)[f.key]" type="text" class="input w-64" placeholder="https://" />
                  <input v-else v-model.number="(cfg as Record<string, unknown>)[f.key]" type="number" :min="f.min ?? 0" class="input w-28 text-right" />
                  <span class="w-8 text-xs text-gray-500">{{ f.unit }}</span>
                </span>
              </label>
            </div>
          </section>

          <!-- 频控：营销统一间隔 + 每个目的自己的间隔（目的列表来自后端注册表） -->
          <section class="card">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-base font-medium text-gray-900 dark:text-white">频控</h2>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">命中频控的人仍会进名单，发送时跳过并写明原因。单发验证邀请等直发邮件不受频控限制。</p>
            </div>
            <div class="divide-y divide-gray-100 dark:divide-dark-700">
              <label class="flex items-center justify-between gap-4 px-6 py-4">
                <span>
                  <span class="block text-sm text-gray-900 dark:text-white">任意两封营销邮件之间</span>
                  <span class="block text-xs text-gray-500">跨目的：召回、资讯、活动之间共用；0 = 不限</span>
                </span>
                <span class="flex items-center gap-2"><input v-model.number="cfg.marketing_gap_days" type="number" min="0" class="input w-28 text-right" /><span class="w-8 text-xs text-gray-500">天</span></span>
              </label>
              <label v-for="p in meta?.purposes ?? []" :key="p.key" class="flex items-center justify-between gap-4 px-6 py-4">
                <span>
                  <span class="block text-sm text-gray-900 dark:text-white">两封「{{ p.label }}」之间</span>
                  <span class="block text-xs text-gray-500">默认 {{ p.default_gap_days }} 天；0 = 不限</span>
                </span>
                <span class="flex items-center gap-2">
                  <input :value="gapOf(p.key, p.default_gap_days)" type="number" min="0" class="input w-28 text-right" @input="setGap(p.key, ($event.target as HTMLInputElement).value)" />
                  <span class="w-8 text-xs text-gray-500">天</span>
                </span>
              </label>
            </div>
          </section>
        </div>

        <div class="flex items-center gap-3">
          <button type="button" class="btn btn-primary" :disabled="saving" @click="save">{{ saving ? '保存中…' : '保存' }}</button>
          <span class="text-xs text-gray-500">保存后立即生效，不需要重启。</span>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminApi, useOutreachMeta, type MailSettings } from '../api'

type Field = { key: keyof MailSettings; label: string; hint?: string; unit: string; min?: number; text?: boolean }

const groups: { title: string; desc?: string; fields: Field[] }[] = [
  {
    title: '链接与批次保留',
    desc: '邮件里按钮、领取、退订链接使用的域名。',
    fields: [
      { key: 'link_base_url', label: '触达链接域名', hint: '例如 https://news.example.com，需指向本站；留空使用系统设置里的「站点地址」', unit: '', text: true },
      { key: 'batch_retention_days', label: '已删除批次保留', hint: '删除发过邮件的批次后，至少保留这么久才清理（不少于 30 天）；期间频控与领取链接仍有效', unit: '天', min: 30 }
    ]
  },
  {
    title: '发送限速',
    desc: '达到上限后，未发出的收件人留在队列里，额度恢复后自动续发，不会丢；这期间批次仍显示「执行中」。',
    fields: [
      { key: 'per_minute', label: '每分钟最多', unit: '封', min: 1 },
      { key: 'per_day', label: '每 24 小时最多', hint: '滚动窗口，按服务商每日额度留余量', unit: '封', min: 1 }
    ]
  },
  {
    title: '自动熔断',
    desc: '退信率达到阈值时批次自动暂停，需人工确认后恢复。只能统计发信时 SMTP 同步拒收的地址。',
    fields: [
      { key: 'breaker_percent', label: '退信率阈值', unit: '%', min: 1 },
      { key: 'breaker_min_samples', label: '最小样本数', hint: '已有结论的邮件不足这个数时不判断', unit: '封', min: 1 }
    ]
  }
]

const appStore = useAppStore()
const { meta } = useOutreachMeta()
const cfg = ref<MailSettings | null>(null)
const version = ref(0)
const error = ref('')
const saving = ref(false)

const gapOf = (key: string, def: number) => cfg.value?.purpose_gap_days?.[key] ?? def
function setGap(key: string, raw: string) {
  if (!cfg.value) return
  const n = Number(raw)
  cfg.value.purpose_gap_days = { ...(cfg.value.purpose_gap_days ?? {}), [key]: Number.isFinite(n) ? n : 0 }
}

async function load() {
  try {
    const r = await adminApi.getSettings()
    cfg.value = { ...r.config, purpose_gap_days: { ...(r.config.purpose_gap_days ?? {}) } }
    version.value = r.version
  } catch (e) {
    error.value = extractApiErrorMessage(e, '加载失败')
  }
}

async function save() {
  if (!cfg.value) return
  saving.value = true
  try {
    const r = await adminApi.saveSettings(cfg.value, version.value)
    version.value = r.version
    appStore.showSuccess('已保存')
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
