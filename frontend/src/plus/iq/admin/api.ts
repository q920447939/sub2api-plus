/**
 * [plus] 智商检测 · 管理端 API（只在 /admin 页面使用）
 */
import { apiClient } from '@/api/client'

export type Format = '' | 'anthropic' | 'openai_chat' | 'openai_responses' | 'gemini'
export type Thinking = 'off' | 'low' | 'medium' | 'high'
export type KeyMode = 'auto' | 'manual'
export type RunStatus = 'queued' | 'running' | 'success' | 'failed'
export type Trigger = 'cron' | 'manual'

export interface Config {
  enabled: boolean
  user_enabled: boolean
  menu_name: string
  user_samples_per_task: number
  default_interval: number
  default_retention: number
  concurrency: number
  timeout_seconds: number
  max_output_tokens: number
  endpoint: string
  key_prefix: string
  prompt: string
  prompt_version: number
}

export interface GroupInfo {
  id: number
  name: string
  platform: string
}

export interface AdminConfigView {
  config: Config
  version: number
  groups: GroupInfo[]
  default_prompt: string
}

export interface KeyOption {
  id: number
  name: string
  masked: string
  user_id: number
  user_email: string
  active: boolean
}

export interface HopView {
  account_id: number
  ok: boolean
  status_code?: number
  message?: string
  account_name: string
}

export interface RunView {
  id: number
  task_id: number
  group_id: number
  model: string
  trigger: Trigger
  triggered_by?: number
  status: RunStatus
  prompt_version: number
  api_key_id?: number
  client_request_id: string
  queued_at: string
  started_at?: string
  finished_at?: string
  duration_ms?: number
  input_tokens?: number
  output_tokens?: number
  has_html: boolean
  external_refs?: string[]
  error?: string
  raw_excerpt?: string
  account_chain: unknown[]
  attributed_at?: string
  group_name: string
  chain: HopView[]
  preview_url?: string
  key_name?: string
  key_masked?: string
}

export interface Task {
  id: number
  group_id: number
  model: string
  key_mode: KeyMode
  api_key_id: number
  format: Format
  thinking: Thinking
  interval_minutes: number
  retention: number
  enabled: boolean
  created_by: number
  next_run_at: string
  created_at: string
  updated_at: string
}

export interface TaskView extends Task {
  group_name: string
  platform: string
  key: KeyOption | null
  recent: RunView[]
  active: RunStatus | null
}

export interface OverviewStats {
  enabled_tasks: number
  total_tasks: number
  daily_runs: number
  success_rate_24h: number | null
  running: number
  queued: number
  concurrency: number
  scheduler_on: boolean
}

export interface TasksOverview {
  tasks: TaskView[]
  stats: OverviewStats
}

export interface TaskInput {
  group_id: number
  model: string
  key_mode: KeyMode
  api_key_id: number
  format: Format
  thinking: Thinking
  interval_minutes: number
  retention: number
  enabled: boolean
}

export interface KeyChoices {
  auto_name: string
  auto_key_id: number
  keys: KeyOption[]
}

export interface TestResult {
  ok: boolean
  latency_ms: number
  message: string
  reply: string
  client_request_id: string
}

export interface AdminGalleryTask {
  task_id: number
  model: string
  enabled: boolean
  interval_minutes: number
  next_run_at: string
  runs: RunView[]
}

export interface AdminGalleryGroup extends GroupInfo {
  account_count: number
  tasks: AdminGalleryTask[]
}

export interface RunFilter {
  group_id?: number
  status?: RunStatus | ''
  trigger?: Trigger | ''
  account_id?: number
  page?: number
  page_size?: number
}

export interface Paginated<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  pages: number
}

const BASE = '/plus/admin/iq'

export async function getConfig(): Promise<AdminConfigView> {
  return (await apiClient.get<AdminConfigView>(`${BASE}/config`)).data
}

export async function saveConfig(config: Config, version: number): Promise<number> {
  return (await apiClient.put<{ version: number }>(`${BASE}/config`, { config, version })).data.version
}

export async function getTasks(): Promise<TasksOverview> {
  return (await apiClient.get<TasksOverview>(`${BASE}/tasks`)).data
}

export async function createTask(input: TaskInput): Promise<number> {
  return (await apiClient.post<{ id: number }>(`${BASE}/tasks`, input)).data.id
}

export async function updateTask(id: number, input: TaskInput): Promise<void> {
  await apiClient.put(`${BASE}/tasks/${id}`, input)
}

export async function deleteTask(id: number): Promise<void> {
  await apiClient.delete(`${BASE}/tasks/${id}`)
}

export async function testTask(input: TaskInput): Promise<TestResult> {
  return (await apiClient.post<TestResult>(`${BASE}/tasks/test`, input, { timeout: 120_000 })).data
}

export async function listModels(input: TaskInput): Promise<string[]> {
  return (await apiClient.post<string[]>(`${BASE}/tasks/models`, input)).data
}

/** 手动检测：传 task_id 单个、group_id 整组，都不传为全部启用的任务 */
export async function runNow(target: { task_id?: number; group_id?: number }): Promise<{ queued: number; skipped: number }> {
  return (await apiClient.post<{ queued: number; skipped: number }>(`${BASE}/run`, target)).data
}

export async function getKeyChoices(groupId: number): Promise<KeyChoices> {
  return (await apiClient.get<KeyChoices>(`${BASE}/keys`, { params: { group_id: groupId } })).data
}

export async function listRuns(f: RunFilter): Promise<Paginated<RunView>> {
  const params: Record<string, string | number> = {}
  for (const [k, v] of Object.entries(f)) {
    if (v !== undefined && v !== '' && v !== 0) params[k] = v as string | number
  }
  return (await apiClient.get<Paginated<RunView>>(`${BASE}/runs`, { params })).data
}

export async function getRun(id: number): Promise<RunView> {
  return (await apiClient.get<RunView>(`${BASE}/runs/${id}`)).data
}

export async function getGallery(groupId?: number): Promise<AdminGalleryGroup[]> {
  return (await apiClient.get<AdminGalleryGroup[]>(`${BASE}/gallery`, { params: groupId ? { group_id: groupId } : undefined })).data
}
