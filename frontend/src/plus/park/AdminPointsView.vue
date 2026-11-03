<template>
  <AdminParkPage
    title="积分管理"
    :tabs="[
      { key: 'accounts', label: '积分账户' },
      { key: 'ledger', label: '积分流水' },
      { key: 'earn', label: '积分获取' },
      { key: 'rules', label: '积分规则' }
    ]"
    :config-tabs="['earn', 'rules']"
    section="points"
    save-label="保存积分配置"
  >
    <!-- 积分账户 -->
    <template #accounts>
      <div class="card p-5">
        <div class="mb-3 flex flex-wrap items-end gap-3">
          <p class="mr-auto font-medium text-gray-900 dark:text-white">用户积分账户</p>
          <select v-model="acc.sort" class="input w-36" @change="loadAccounts(1)">
            <option value="balance">按余额排序</option>
            <option value="last_active">按最近变动排序</option>
          </select>
          <input v-model="acc.q" class="input w-56" placeholder="邮箱 / 用户名 / 用户 ID" @keyup.enter="loadAccounts(1)" />
          <button type="button" class="btn btn-secondary btn-sm" @click="loadAccounts(1)">查询</button>
          <button type="button" class="btn btn-primary btn-sm" data-testid="points-adjust-open" @click="openAdjust()">手动调整积分</button>
        </div>
        <div class="overflow-x-auto">
          <table class="w-full min-w-[820px] text-sm">
            <thead class="text-left text-xs text-gray-400">
              <tr>
                <th class="py-2">用户</th>
                <th class="py-2 text-right">当前积分</th>
                <th class="py-2 text-right">累计获得</th>
                <th class="py-2 text-right">累计使用</th>
                <th class="py-2 pl-4">最近变动</th>
                <th class="py-2">清零时间</th>
                <th class="py-2 text-right">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="a in acc.items" :key="a.user_id">
                <td class="py-2">{{ a.email }} <span class="text-xs text-gray-400">#{{ a.user_id }}</span></td>
                <td class="py-2 text-right font-medium tabular-nums">{{ a.balance }}</td>
                <td class="py-2 text-right tabular-nums text-gray-500">{{ a.total_earned }}</td>
                <td class="py-2 text-right tabular-nums text-gray-500">{{ a.total_spent }}</td>
                <td class="py-2 pl-4 text-gray-500">{{ fmtTime(a.last_active_at) }}</td>
                <td class="py-2" :class="soon(a.expire_at) ? 'text-amber-600' : 'text-gray-500'">{{ a.expire_at ? fmtDate(a.expire_at) : '-' }}</td>
                <td class="py-2 text-right">
                  <button type="button" class="btn btn-secondary btn-sm" @click="openUserLedger(a.user_id, a.email)">流水</button>
                  <button type="button" class="btn btn-secondary btn-sm ml-2" @click="openAdjust(a.user_id, a.email, a.balance)">调整</button>
                </td>
              </tr>
              <tr v-if="!acc.items.length">
                <td colspan="7" class="py-8 text-center text-gray-400">{{ acc.loading ? '加载中…' : '暂无积分账户' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <Pagination
          v-if="acc.total > 0"
          class="mt-3"
          :total="acc.total"
          :page="acc.page"
          :page-size="acc.pageSize"
          @update:page="(p) => loadAccounts(p)"
          @update:page-size="(s) => { acc.pageSize = s; loadAccounts(1) }"
        />
      </div>
    </template>

    <!-- 全站积分流水 -->
    <template #ledger>
      <div class="card p-5">
        <div class="mb-3 flex flex-wrap items-end gap-3">
          <p class="mr-auto font-medium text-gray-900 dark:text-white">积分流水</p>
          <select v-model="led.reason" class="input w-40" @change="loadLedger(1)">
            <option value="">全部类型</option>
            <option v-for="(m, k) in REASON_META" :key="k" :value="k">{{ m.label }}</option>
          </select>
          <input v-model="led.q" class="input w-56" placeholder="邮箱 / 用户名 / 用户 ID" @keyup.enter="loadLedger(1)" />
          <button type="button" class="btn btn-secondary btn-sm" @click="loadLedger(1)">查询</button>
        </div>
        <LedgerTable :items="led.items" :loading="led.loading" show-user />
        <Pagination
          v-if="led.total > 0"
          class="mt-3"
          :total="led.total"
          :page="led.page"
          :page-size="led.pageSize"
          @update:page="(p) => loadLedger(p)"
          @update:page-size="(s) => { led.pageSize = s; loadLedger(1) }"
        />
      </div>
    </template>

    <template #earn>
      <AdminEarnTab />
    </template>
    <template #rules>
      <AdminPointsRulesTab />
    </template>
  </AdminParkPage>

  <!-- 单个用户流水 -->
  <BaseDialog :show="userLed.show" :title="`积分流水 · ${userLed.email}`" width="extra-wide" @close="userLed.show = false">
    <LedgerTable :items="userLed.items" :loading="userLed.loading" />
    <Pagination
      v-if="userLed.total > userLed.pageSize"
      class="mt-3"
      :total="userLed.total"
      :page="userLed.page"
      :page-size="userLed.pageSize"
      :show-page-size-selector="false"
      @update:page="(p) => loadUserLedger(p)"
    />
  </BaseDialog>

  <!-- 手动调整 -->
  <BaseDialog :show="adj.show" title="手动调整积分" width="narrow" @close="adj.show = false">
    <div class="space-y-4 text-sm">
      <label class="block">
        <span class="text-gray-600 dark:text-gray-400">用户 ID</span>
        <input v-model.number="adj.userId" type="number" min="1" class="input mt-1 w-full" :disabled="!!adj.email" data-testid="points-adjust-user" />
        <span v-if="adj.email" class="mt-1 block text-xs text-gray-400">{{ adj.email }} · 当前 {{ adj.balance }} 积分</span>
      </label>
      <div class="flex gap-3">
        <label class="block">
          <span class="text-gray-600 dark:text-gray-400">方向</span>
          <select v-model="adj.dir" class="input mt-1 w-24">
            <option value="add">增加</option>
            <option value="sub">扣减</option>
          </select>
        </label>
        <label class="block flex-1">
          <span class="text-gray-600 dark:text-gray-400">数量</span>
          <input v-model.number="adj.amount" type="number" min="1" class="input mt-1 w-full" data-testid="points-adjust-amount" />
        </label>
      </div>
      <label class="block">
        <span class="text-gray-600 dark:text-gray-400">原因（仅管理员可见）</span>
        <input v-model="adj.remark" maxlength="200" class="input mt-1 w-full" placeholder="如：活动补偿、误发回收" data-testid="points-adjust-remark" />
        <span class="mt-1 block text-xs text-gray-400">用户在积分流水里看到的是“积分调整”</span>
      </label>
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="adj.show = false">取消</button>
        <button type="button" class="btn btn-primary" :disabled="!adjValid || adj.saving" data-testid="points-adjust-submit" @click="submitAdjust">
          {{ adj.saving ? '提交中…' : '确认调整' }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  REASON_META,
  adminAdjustPoints,
  adminListAccounts,
  adminPointsLedger,
  type AdminAccount,
  type AdminLedgerItem
} from '../points/api'
import AdminEarnTab from './AdminEarnTab.vue'
import AdminParkPage from './AdminParkPage.vue'
import AdminPointsRulesTab from './AdminPointsRulesTab.vue'
import LedgerTable from './AdminLedgerTable.vue'
import { fmtDate, fmtTime } from './adminFormat'

const appStore = useAppStore()

// ─── 账户列表 ──────────────────────────────────────────────────────────
const acc = reactive({
  items: [] as AdminAccount[],
  total: 0,
  page: 1,
  pageSize: 20,
  q: '',
  sort: 'balance' as 'balance' | 'last_active',
  loading: false
})

async function loadAccounts(p = acc.page) {
  acc.loading = true
  try {
    const res = await adminListAccounts({ page: p, page_size: acc.pageSize, q: acc.q.trim(), sort: acc.sort })
    acc.items = res.items ?? []
    acc.total = res.total
    acc.page = res.page || p
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '积分账户加载失败'))
  } finally {
    acc.loading = false
  }
}

const soon = (iso?: string) => !!iso && new Date(iso).getTime() - Date.now() < 30 * 86400_000

// ─── 全站流水 ──────────────────────────────────────────────────────────
const led = reactive({ items: [] as AdminLedgerItem[], total: 0, page: 1, pageSize: 20, q: '', reason: '', loading: false })

async function loadLedger(p = led.page) {
  led.loading = true
  try {
    const res = await adminPointsLedger({ page: p, page_size: led.pageSize, q: led.q.trim(), reason: led.reason })
    led.items = res.items ?? []
    led.total = res.total
    led.page = res.page || p
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '积分流水加载失败'))
  } finally {
    led.loading = false
  }
}

// ─── 单用户流水弹窗 ────────────────────────────────────────────────────
const userLed = reactive({ show: false, userId: 0, email: '', items: [] as AdminLedgerItem[], total: 0, page: 1, pageSize: 10, loading: false })

async function loadUserLedger(p = 1) {
  userLed.loading = true
  try {
    const res = await adminPointsLedger({ page: p, page_size: userLed.pageSize, user_id: userLed.userId })
    userLed.items = res.items ?? []
    userLed.total = res.total
    userLed.page = res.page || p
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '积分流水加载失败'))
  } finally {
    userLed.loading = false
  }
}

function openUserLedger(userId: number, email: string) {
  Object.assign(userLed, { show: true, userId, email, items: [], total: 0 })
  void loadUserLedger(1)
}

// ─── 手动调整 ──────────────────────────────────────────────────────────
const adj = reactive({
  show: false,
  userId: 0 as number | '',
  email: '',
  balance: 0,
  dir: 'add' as 'add' | 'sub',
  amount: 0 as number | '',
  remark: '',
  saving: false
})
const adjValid = computed(() => Number(adj.userId) > 0 && Number(adj.amount) > 0 && adj.remark.trim().length > 0)

function openAdjust(userId?: number, email = '', balance = 0) {
  Object.assign(adj, { show: true, userId: userId ?? '', email, balance, dir: 'add', amount: '', remark: '' })
}

async function submitAdjust() {
  adj.saving = true
  try {
    const delta = Number(adj.amount) * (adj.dir === 'sub' ? -1 : 1)
    const bal = await adminAdjustPoints(Number(adj.userId), delta, adj.remark.trim())
    appStore.showSuccess(`已调整，当前余额 ${bal} 积分`)
    adj.show = false
    void loadAccounts()
    void loadLedger()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '调整失败'))
  } finally {
    adj.saving = false
  }
}

onMounted(() => {
  void loadAccounts(1)
  void loadLedger(1)
})
</script>
