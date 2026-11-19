<!-- [plus] 可用变量：点击复制。变量集合由后端按目的 + 权益给出。 -->
<template>
  <div>
    <p class="mb-1 text-xs font-medium text-gray-600 dark:text-dark-300">可用变量（主题和正文都支持，点击复制）</p>
    <div class="flex flex-wrap gap-1.5">
      <button
        v-for="v in variables"
        :key="v.name"
        type="button"
        class="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700 hover:bg-gray-200 dark:bg-dark-700 dark:text-dark-200"
        :title="v.desc"
        @click="copy(v.name)"
      >{{ varToken(v.name) }}</button>
    </div>
    <p class="mt-2 text-xs text-gray-400">
      独占一行的 <code>[[按钮文字]]({{ CTA }})</code> 渲染为按钮；按钮去向由系统按目的决定（验证页 / 先领权益再到落地页 / 落地页），一封邮件只需要一个。
    </p>
  </div>
</template>

<script setup lang="ts">
import { useAppStore } from '@/stores/app'
import { CTA, varToken, type VariableDoc } from '../api'

defineProps<{ variables: VariableDoc[] }>()
const appStore = useAppStore()

async function copy(name: string) {
  try {
    await navigator.clipboard.writeText(varToken(name))
    appStore.showSuccess(`已复制 ${varToken(name)}`)
  } catch {
    appStore.showError('复制失败，请手动输入')
  }
}
</script>
