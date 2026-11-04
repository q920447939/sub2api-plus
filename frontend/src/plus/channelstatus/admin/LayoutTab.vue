<template>
  <div v-if="c" class="space-y-4">
    <p class="card px-4 py-3 text-xs text-gray-500 dark:text-dark-300">
      拖动 ⠿ 调整平台 / 分组顺序；分组可拖到其他平台下。关闭的平台或分组不会出现在用户端。展示名留空则使用原名。
    </p>

    <VueDraggable v-model="c.layout" :animation="200" handle=".plat-handle" class="space-y-4">
      <section v-for="p in c.layout" :key="p.platform" class="card overflow-hidden p-0" data-testid="cs-layout-platform">
        <header class="flex items-center gap-3 bg-gray-50 px-4 py-3 dark:bg-dark-800">
          <span class="plat-handle cursor-grab select-none text-gray-300 hover:text-gray-500 active:cursor-grabbing">⠿</span>
          <span class="font-semibold text-gray-900 dark:text-white">{{ platformName(p.platform) }}</span>
          <span class="text-xs text-gray-400">{{ p.platform }} · {{ p.groups.length }} 个分组</span>
          <label class="ml-auto flex items-center gap-2 text-xs text-gray-500">
            Tab 名
            <input v-model.trim="p.tab_name" class="input w-36 py-1 text-sm" :placeholder="platformName(p.platform)" maxlength="32" />
          </label>
          <Toggle v-model="p.visible" />
        </header>

        <div class="grid grid-cols-[1.5rem_3.5rem_minmax(0,1.2fr)_minmax(0,1fr)_minmax(0,1fr)] gap-3 px-4 pt-2 text-xs text-gray-400">
          <span /><span>展示</span><span>sub2api 分组</span><span>展示名</span><span>副标题</span>
        </div>
        <VueDraggable
          v-model="p.groups"
          :animation="200"
          group="cs-groups"
          handle=".group-handle"
          class="min-h-[2.5rem] divide-y divide-gray-50 dark:divide-dark-700"
        >
          <div
            v-for="g in p.groups"
            :key="g.group_id"
            class="grid grid-cols-[1.5rem_3.5rem_minmax(0,1.2fr)_minmax(0,1fr)_minmax(0,1fr)] items-center gap-3 px-4 py-2"
            :class="{ 'opacity-50': !g.visible || !p.visible }"
          >
            <span class="group-handle cursor-grab select-none text-gray-300 hover:text-gray-500 active:cursor-grabbing">⠿</span>
            <Toggle v-model="g.visible" />
            <div class="min-w-0 truncate text-sm text-gray-800 dark:text-gray-100">
              {{ info(g.group_id)?.name ?? '(已删除)' }}
              <span class="text-xs text-gray-400">#{{ g.group_id }}</span>
              <span
                v-if="info(g.group_id) && info(g.group_id)!.platform !== p.platform"
                class="ml-1 rounded bg-gray-100 px-1 text-[10px] text-gray-500 dark:bg-dark-700"
              >{{ platformName(info(g.group_id)!.platform) }}</span>
              <span v-if="settings.newGroupIds.has(g.group_id)" class="ml-1 rounded bg-primary-50 px-1 text-[10px] text-primary-600">新</span>
            </div>
            <input v-model.trim="g.display_name" class="input py-1 text-sm" :placeholder="info(g.group_id)?.name" maxlength="64" />
            <input v-model.trim="g.subtitle" class="input py-1 text-sm" placeholder="如：官方直连 · 高稳定" maxlength="64" />
          </div>
        </VueDraggable>
      </section>
    </VueDraggable>

    <div v-if="!c.layout.length" class="card p-8 text-center text-gray-400">暂无启用的分组</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { VueDraggable } from 'vue-draggable-plus'
import Toggle from '@/components/common/Toggle.vue'
import { platformName } from '../status'
import { settings } from './state'

const c = computed(() => settings.draft)
const info = (id: number) => settings.groups[id]
</script>
