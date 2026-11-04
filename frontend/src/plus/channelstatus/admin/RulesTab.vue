<template>
  <div v-if="r" class="space-y-5">
    <section class="card p-5">
      <h3 class="font-semibold text-gray-900 dark:text-white">状态分档</h3>
      <p class="mb-4 text-xs text-gray-400">
        请求数 &lt; 最小样本时没有健康分，显示「暂无数据」（灰色，不会显示为正常）。
        历史条每一格的时段更短、请求更少，因此单独设一个更低的门槛。
      </p>
      <div class="grid gap-4 md:grid-cols-4">
        <NumField v-model="r.up_min" label="正常：健康分 ≥" suffix="分" :min="1" :max="100" />
        <NumField v-model="r.warn_min" label="降级：健康分 ≥（以下为异常）" suffix="分" :min="1" :max="99" />
        <NumField v-model="r.min_sample" label="当前状态最小样本" suffix="次请求" :min="1" :max="10000" />
        <NumField v-model="r.history_min_sample" label="历史条每格最小样本" suffix="次请求" :min="1" :max="10000" />
      </div>
    </section>

    <section class="card p-5">
      <h3 class="font-semibold text-gray-900 dark:text-white">健康分口径</h3>
      <p class="mb-4 text-xs text-gray-400">
        健康分 = 三个维度各自 0–100 分后按权重加权；某维度样本不足时不参与，其余维度按权重重新归一化。默认值与上游 V2 一致。
      </p>
      <div class="grid gap-4 md:grid-cols-3">
        <NumField v-model="errorWeight" label="错误率权重" suffix="%" :min="0" :max="100" />
        <NumField v-model="ttftWeight" label="首字延迟权重" suffix="%" :min="0" :max="100" />
        <NumField v-model="cacheWeight" label="缓存命中率权重" suffix="%" :min="0" :max="100" />
      </div>
      <p class="mt-2 text-xs" :class="weightOk ? 'text-gray-400' : 'text-red-500'">当前合计 {{ weightSum }}%（必须为 100%）</p>

      <div class="mt-5 grid gap-4 md:grid-cols-3">
        <NumField v-model="warnErr" label="错误率告警线" suffix="%" :min="0" :max="100" :step="0.1" />
        <NumField v-model="critErr" label="错误率严重线（达到即 0 分）" suffix="%" :min="0" :max="100" :step="0.1" />
        <div />
        <NumField v-model="r.target_ttft_ms" label="TTFT 目标（不高于即满分）" suffix="ms" :min="1" />
        <NumField v-model="r.critical_ttft_ms" label="TTFT 严重线（达到即 0 分）" suffix="ms" :min="1" />
        <div />
        <NumField v-model="warnCache" label="缓存命中率告警线" suffix="%" :min="0" :max="100" />
        <NumField v-model="critCache" label="缓存命中率严重线" suffix="%" :min="0" :max="100" />
        <p class="self-end text-xs text-gray-400">两项都为 0 表示不因缓存未命中扣分（默认）</p>
      </div>
    </section>

    <section class="card p-5">
      <div class="flex items-center justify-between gap-4">
        <div>
          <h3 class="font-semibold text-gray-900 dark:text-white">排除用户侧错误</h3>
          <p class="text-xs text-gray-400">
            余额/配额不足、Key 无效或过期、用户并发超限等由用户自身导致的错误，不计入请求数和错误率（与运维监控 SLA 口径一致）。
            无可用账号、分组不支持该模型等平台侧失败仍计为错误。
          </p>
        </div>
        <Toggle v-model="r.exclude_user_side_errors" />
      </div>
    </section>

    <section class="card p-5">
      <h3 class="font-semibold text-gray-900 dark:text-white">忽略的错误分类</h3>
      <p class="mb-4 text-xs text-gray-400">勾选的分类不计入错误率（例如用户自己取消、上下文超限这类与渠道健康无关的错误）</p>
      <div class="grid grid-cols-2 gap-2 md:grid-cols-4">
        <label v-for="cat in settings.errorCategories" :key="cat" class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
          <input v-model="r.ignored_error_categories" type="checkbox" :value="cat" class="rounded border-gray-300" />
          {{ ERROR_CATEGORY_LABELS[cat] ?? cat }}
        </label>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, type WritableComputedRef } from 'vue'
import { ERROR_CATEGORY_LABELS } from '../status'
import type { Rules } from '../api'
import { settings } from './state'
import NumField from './NumField.vue'
import Toggle from '@/components/common/Toggle.vue'

const r = computed(() => settings.draft?.rules)

// 比例字段（0~1）在界面上按百分比编辑
type RatioKey = 'error_weight' | 'ttft_weight' | 'cache_weight' | 'warning_error_rate' | 'critical_error_rate' | 'warning_cache_rate' | 'critical_cache_rate'
function pct(key: RatioKey): WritableComputedRef<number> {
  return computed({
    get: () => Math.round((r.value?.[key] ?? 0) * 1000) / 10,
    set: (v: number) => {
      if (r.value) (r.value as Rules)[key] = Math.round(v * 10) / 1000
    }
  })
}
const errorWeight = pct('error_weight')
const ttftWeight = pct('ttft_weight')
const cacheWeight = pct('cache_weight')
const warnErr = pct('warning_error_rate')
const critErr = pct('critical_error_rate')
const warnCache = pct('warning_cache_rate')
const critCache = pct('critical_cache_rate')

const weightSum = computed(() => Math.round((errorWeight.value + ttftWeight.value + cacheWeight.value) * 10) / 10)
const weightOk = computed(() => Math.abs(weightSum.value - 100) < 0.1)
</script>
