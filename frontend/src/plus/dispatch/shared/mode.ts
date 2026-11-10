// [plus] 当前档位（各页面共享，切换档位后调用 refreshMode）
import { ref } from 'vue'
import { dispatchApi, type Mode } from '../api'

export const dispatchMode = ref<Mode | null>(null)

export async function refreshMode() {
  try {
    dispatchMode.value = (await dispatchApi.getConfig()).config.mode
  } catch {
    /* 没权限或模块未启用时不显示 */
  }
}
