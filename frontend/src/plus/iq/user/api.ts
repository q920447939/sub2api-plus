/**
 * [plus] 模型画廊 · 用户端 API。
 * 菜单即身份：用户页只用这个文件，返回结构里没有账号、Key、错误等管理端字段。
 */
import { apiClient } from '@/api/client'

export interface UserMeta {
  enabled: boolean
  menu_name: string
}

export interface UserSample {
  id: number
  created_at: string
  preview_url: string
}

export interface UserTask {
  model: string
  samples: UserSample[]
}

export interface UserGroup {
  id: number
  name: string
  platform: string
  tasks: UserTask[]
}

export interface UserGallery {
  groups: UserGroup[]
}

export async function getMeta(): Promise<UserMeta> {
  const { data } = await apiClient.get<UserMeta>('/plus/iq/meta')
  return data
}

export async function getGallery(): Promise<UserGallery> {
  const { data } = await apiClient.get<UserGallery>('/plus/iq/gallery')
  return data
}
