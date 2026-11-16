// [plus] 管理端共享：全部场景（含已停用，历史数据要能显示名称）、积分是否可用
import { ref } from 'vue'
import { adminApi, type Config, type Scenario } from '../api'

export const allScenarios = ref<Scenario[]>([])
export const rewardEnabled = ref(false)
export const rewardRule = ref<Config['reward']>({ default: 500, step: 100 })
export const attachmentRule = ref<Config['attachments']>({ max_count: 3, max_mb: 5 })
let loaded: Promise<void> | null = null

export function loadAdminScenarios(force = false): Promise<void> {
  if (!loaded || force) {
    loaded = adminApi
      .config()
      .then((r) => {
        allScenarios.value = r.config.scenarios
        rewardEnabled.value = r.reward_enabled
        rewardRule.value = r.config.reward
        attachmentRule.value = r.config.attachments
      })
      .catch(() => {
        loaded = null
      })
  }
  return loaded
}

export const scenarioLabel = (key: string) => allScenarios.value.find((s) => s.key === key)?.label ?? key
export const scenarioIcon = (key: string) => allScenarios.value.find((s) => s.key === key)?.icon ?? '💬'
