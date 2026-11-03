<template>
  <AdminParkPage
    title="签到管理"
    :tabs="[{ key: 'data', label: '签到数据' }, { key: 'config', label: '签到配置' }]"
    :config-tabs="['config']"
    section="checkin"
    save-label="保存签到配置"
  >
    <template #data>
      <div v-if="stats" class="grid grid-cols-2 gap-4 lg:grid-cols-5" data-testid="checkin-admin-stats">
        <AdminStatTile label="今日签到人数" :value="stats.today.users" :sub="`发放 ${stats.today.points} 积分`" />
        <AdminStatTile label="昨日签到人数" :value="stats.yesterday.users" :sub="`发放 ${stats.yesterday.points} 积分`" />
        <AdminStatTile v-for="b in stats.streak_dist" :key="b.label" :label="`今日 · ${b.label}`" :value="b.users" />
      </div>

      <div v-if="stats" class="space-y-2">
        <div class="flex gap-2">
          <button
            v-for="d in [7, 30]"
            :key="d"
            type="button"
            class="btn btn-sm"
            :class="days === d ? 'btn-primary' : 'btn-secondary'"
            @click="days = d"
          >近 {{ d }} 天</button>
        </div>
        <AdminTrendChart
          title="每日签到人数"
          :labels="stats.trend.map((d) => d.date)"
          :series="[{ label: '签到人数', data: stats.trend.map((d) => d.users), color: '#10b981' }]"
        />
      </div>

      <div class="card p-5">
        <div class="mb-3 flex flex-wrap items-end gap-3">
          <p class="mr-auto font-medium text-gray-900 dark:text-white">签到记录</p>
          <label class="text-sm text-gray-500">
            日期
            <input v-model="filter.date" type="date" class="input ml-1 w-40" @change="reload" />
          </label>
          <input v-model="filter.q" class="input w-56" placeholder="邮箱 / 用户名 / 用户 ID" @keyup.enter="reload" />
          <button type="button" class="btn btn-secondary btn-sm" @click="reload">查询</button>
          <button type="button" class="btn btn-secondary btn-sm" @click="setToday">只看今天</button>
        </div>
        <div class="overflow-x-auto">
          <table class="w-full min-w-[640px] text-sm">
            <thead class="text-left text-xs text-gray-400">
              <tr>
                <th class="py-2">签到日期</th>
                <th class="py-2">用户</th>
                <th class="py-2 text-right">连签天数</th>
                <th class="py-2 text-right">获得积分</th>
                <th class="py-2 pl-4">签到时间</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="l in logs" :key="l.id">
                <td class="py-2">{{ l.checked_date }}</td>
                <td class="py-2">{{ l.email }} <span class="text-xs text-gray-400">#{{ l.user_id }}</span></td>
                <td class="py-2 text-right tabular-nums">{{ l.streak }}</td>
                <td class="py-2 text-right tabular-nums">+{{ l.points_granted }}</td>
                <td class="py-2 pl-4 text-gray-500">{{ fmtTime(l.created_at) }}</td>
              </tr>
              <tr v-if="!logs.length">
                <td colspan="5" class="py-8 text-center text-gray-400">{{ loading ? '加载中…' : '暂无签到记录' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <Pagination
          v-if="total > 0"
          class="mt-3"
          :total="total"
          :page="page"
          :page-size="pageSize"
          @update:page="(p) => loadLogs(p)"
          @update:page-size="(s) => { pageSize = s; loadLogs(1) }"
        />
      </div>
    </template>

    <template #config>
      <AdminCheckinTab />
    </template>
  </AdminParkPage>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import Pagination from '@/components/common/Pagination.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminCheckinLogs, adminCheckinStats, type AdminCheckinLog, type CheckinStats } from '../checkin/api'
import AdminCheckinTab from './AdminCheckinTab.vue'
import AdminParkPage from './AdminParkPage.vue'
import AdminStatTile from './AdminStatTile.vue'
import AdminTrendChart from './AdminTrendChart.vue'
import { fmtTime, todayCN } from './adminFormat'

const appStore = useAppStore()

const days = ref(7)
const stats = ref<CheckinStats | null>(null)

async function loadStats() {
  try {
    stats.value = await adminCheckinStats(days.value)
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '签到统计加载失败'))
  }
}

const filter = reactive({ date: '', q: '' })
const logs = ref<AdminCheckinLog[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)

async function loadLogs(p = page.value) {
  loading.value = true
  try {
    const res = await adminCheckinLogs({ page: p, page_size: pageSize.value, q: filter.q.trim(), date: filter.date })
    logs.value = res.items ?? []
    total.value = res.total
    page.value = res.page || p
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '签到记录加载失败'))
  } finally {
    loading.value = false
  }
}

const reload = () => loadLogs(1)
function setToday() {
  filter.date = todayCN()
  reload()
}

watch(days, loadStats)
onMounted(() => {
  void loadStats()
  void loadLogs(1)
})
</script>
