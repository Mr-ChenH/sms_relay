import type { Page } from '../types'

export function formatTime(value: string) {
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

export function formatLogTime(value: string) {
  return new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(new Date(value))
}

export function statusClass(status: string) {
  if (['online', 'success', 'enabled', 'succeeded'].includes(status)) return 'ok'
  if (['retrying', 'running', 'pending', 'claimed'].includes(status)) return 'warn'
  if (['offline', 'failed', 'timed_out'].includes(status)) return 'danger'
  return 'gray'
}

export const navItems: Array<{ page: Page; icon: string; label: string; section?: string }> = [
  { page: 'overview', icon: '▣', label: '总览', section: '工作台' },
  { page: 'devices', icon: '▤', label: '终端' },
  { page: 'sms', icon: '✉', label: '历史短信' },
  { page: 'send', icon: '↗', label: '发送短信' },
  { page: 'routes', icon: '⌁', label: '消息分发', section: '管理' },
  { page: 'esim', icon: '▥', label: 'eSIM' },
  { page: 'esim-subscriptions', icon: '◴', label: '订阅保活' },
  { page: 'tools', icon: '⌘', label: '诊断工具' },
  { page: 'logs', icon: '≡', label: '日志' },
  { page: 'audit', icon: '◎', label: '审计' }
]
