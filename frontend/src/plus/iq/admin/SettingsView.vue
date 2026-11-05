<!-- [plus] 管理端「智商检测设置」 -->
<template>
  <AppLayout>
    <div class="space-y-4 pb-16" data-testid="iq-admin-settings">
      <header class="flex flex-wrap items-end gap-3">
        <div>
          <h1 class="text-xl font-bold text-gray-900 dark:text-white">智商检测设置</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">全局配置。任务上的间隔、保留条数等设置优先于这里的默认值。</p>
        </div>
        <span class="flex-1" />
        <button type="button" class="btn btn-primary" :disabled="!cfg || saving" data-testid="iq-settings-save" @click="save">{{ saving ? '保存中…' : '保存' }}</button>
      </header>

      <div v-if="!cfg" class="card p-10 text-center text-gray-400">{{ error || '正在加载…' }}</div>
      <div v-else class="grid gap-4 lg:grid-cols-2">
        <section class="card space-y-1 p-5">
          <h3 class="mb-2 font-semibold">开关与可见性</h3>
          <Row title="启用定时检测" desc="关闭后调度器不再定时检测，已有数据保留，手动检测仍可用">
            <Toggle v-model="cfg.enabled" />
          </Row>
          <Row :title="`对用户开放「${menuName}」`" :desc="`关闭时所有人（包括管理员）的用户菜单里都不显示「${menuName}」；管理员走「检测画廊」`">
            <Toggle v-model="cfg.user_enabled" data-testid="iq-user-enabled" />
          </Row>
          <Row title="用户可见分组" desc="和用户登录后在「API 密钥」里能选择的分组一致，不单独配置" />
          <Row title="用户端菜单名称" desc="侧边栏、页面标题和浏览器标签页都用这个名字"><input v-model.trim="cfg.menu_name" class="input w-48" /></Row>
          <Row title="用户端每个模型显示条数" desc="只显示成功样本，不超过任务的保留数"><input v-model.number="cfg.user_samples_per_task" type="number" min="1" max="100" class="input w-24" /></Row>
        </section>

        <section class="card space-y-1 p-5">
          <h3 class="mb-2 font-semibold">默认值与调度</h3>
          <Row title="默认检测间隔（分钟）" desc="新建任务时预填"><input v-model.number="cfg.default_interval" type="number" min="5" class="input w-24" /></Row>
          <Row title="默认保留样本数" desc="新建任务时预填"><input v-model.number="cfg.default_retention" type="number" min="1" class="input w-24" /></Row>
          <Row title="全局并发上限" desc="同时进行的检测数，超出的排队"><input v-model.number="cfg.concurrency" type="number" min="1" max="10" class="input w-24" /></Row>
          <Row title="单次超时（秒）" desc="思考型模型可能要几分钟"><input v-model.number="cfg.timeout_seconds" type="number" min="30" class="input w-24" /></Row>
          <Row title="最大输出 Token" desc="防止异常输出导致高额费用（思考预算另算）"><input v-model.number="cfg.max_output_tokens" type="number" min="1000" class="input w-28" /></Row>
          <Row title="网关地址" desc="检测请求发往这里，一般是本站（容器内可用 http://127.0.0.1:8080）"><input v-model.trim="cfg.endpoint" class="input w-64 font-mono" /></Row>
        </section>

        <section class="card space-y-1 p-5">
          <h3 class="mb-2 font-semibold">探测 Key</h3>
          <Row title="自动生成 Key 名称前缀" desc="生成的名称是「前缀 + 分组名」，挂在创建任务的管理员名下，正常计费">
            <input v-model.trim="cfg.key_prefix" class="input w-32 font-mono" />
          </Row>
        </section>

        <section class="card p-5">
          <h3 class="mb-2 font-semibold">题目 <span class="ml-1 rounded-full bg-gray-100 px-2 text-xs font-normal text-gray-600">v{{ cfg.prompt_version }}</span></h3>
          <textarea v-model="cfg.prompt" rows="6" class="input font-mono text-xs" />
          <p class="mt-1 text-xs text-gray-500">题目来自 pelican-iq（已获作者授权）。修改并保存后版本号 +1，新样本记录新版本。</p>
          <button type="button" class="btn btn-secondary btn-sm mt-2" @click="cfg.prompt = defaultPrompt">恢复默认题目</button>
          <div class="mt-3 rounded-lg bg-blue-50 p-2.5 text-xs text-blue-800 dark:bg-blue-900/20 dark:text-blue-300">
            🔒 预览安全：模型输出的 HTML 放在 sandbox iframe（不带 allow-same-origin）里运行，响应头带 sandbox CSP 并禁止一切外部连接，读不到本站的 Cookie 和 Token。
          </div>
        </section>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { getConfig, saveConfig, type Config } from './api'
import { IQ_DEFAULT_MENU_NAME, refreshIQMeta } from '../user/navFlag'

// 一行设置：左侧标题 + 说明，右侧控件
const Row = defineComponent({
  props: { title: { type: String, required: true }, desc: { type: String, default: '' } },
  setup(props, { slots }) {
    return () =>
      h('div', { class: 'flex items-center gap-3 border-b border-dashed border-gray-200 py-2.5 last:border-b-0 dark:border-dark-700' }, [
        h('div', { class: 'flex-1' }, [
          h('div', { class: 'font-medium' }, props.title),
          props.desc ? h('div', { class: 'text-xs text-gray-500' }, props.desc) : null
        ]),
        slots.default?.()
      ])
  }
})

const appStore = useAppStore()
const cfg = ref<Config | null>(null)
const version = ref(0)
const defaultPrompt = ref('')
const error = ref('')
const saving = ref(false)
const menuName = computed(() => cfg.value?.menu_name || IQ_DEFAULT_MENU_NAME)

async function load() {
  try {
    const v = await getConfig()
    cfg.value = v.config
    version.value = v.version
    defaultPrompt.value = v.default_prompt
  } catch (e) {
    error.value = extractApiErrorMessage(e, '加载失败')
  }
}

async function save() {
  if (!cfg.value) return
  saving.value = true
  try {
    version.value = await saveConfig(cfg.value, version.value)
    appStore.showSuccess('已保存')
    void refreshIQMeta(true) // 侧边栏菜单名、开关立即生效
    await load()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
