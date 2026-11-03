// [plus] 积分乐园管理端格式化工具

const p2 = (n: number) => String(n).padStart(2, '0')

/** ISO → "MM-DD HH:mm:ss"（浏览器本地时区） */
export function fmtTime(iso: string): string {
  const d = new Date(iso)
  return `${p2(d.getMonth() + 1)}-${p2(d.getDate())} ${p2(d.getHours())}:${p2(d.getMinutes())}:${p2(d.getSeconds())}`
}

/** ISO → "YYYY-MM-DD"（浏览器本地时区） */
export function fmtDate(iso: string): string {
  const d = new Date(iso)
  return `${d.getFullYear()}-${p2(d.getMonth() + 1)}-${p2(d.getDate())}`
}

/** 东八区今天 "YYYY-MM-DD"，与后端统计口径一致 */
export function todayCN(): string {
  const d = new Date(Date.now() + 8 * 3600_000)
  return `${d.getUTCFullYear()}-${p2(d.getUTCMonth() + 1)}-${p2(d.getUTCDate())}`
}
