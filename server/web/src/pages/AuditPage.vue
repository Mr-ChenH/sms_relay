<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { AuditLog } from '../types'
import PaginationBar from '../components/PaginationBar.vue'
import { statusClass } from '../utils/ui'

const props = defineProps<{ audit: AuditLog[] }>()

const query = ref('')
const actor = ref('all')
const result = ref('all')
const actionGroup = ref('all')
const selectedId = ref('')
const page = ref(1)
const pageSize = ref(10)
const copied = ref(false)

const actors = computed(() => Array.from(new Set(props.audit.map((row) => row.actor).filter(Boolean))).sort())
const results = computed(() => Array.from(new Set(props.audit.map((row) => row.result).filter(Boolean))).sort())
const filteredAudit = computed(() => {
  const keyword = query.value.trim().toLowerCase()
  return props.audit.filter((row) => {
    const matchesQuery = !keyword || `${row.actor} ${row.deviceName} ${row.action} ${row.parameterSummary} ${row.result}`.toLowerCase().includes(keyword)
    const matchesActor = actor.value === 'all' || row.actor === actor.value
    const matchesResult = result.value === 'all' || row.result === result.value
    const matchesGroup = actionGroup.value === 'all' || classifyAction(row.action) === actionGroup.value
    return matchesQuery && matchesActor && matchesResult && matchesGroup
  })
})
const selectedAudit = computed(() => pagedAudit.value.find((row) => row.id === selectedId.value) || pagedAudit.value[0] || null)
const pendingCount = computed(() => props.audit.filter((row) => ['pending', 'running', 'claimed'].includes(row.result)).length)
const failureCount = computed(() => props.audit.filter((row) => ['failed', 'error', 'timed_out'].includes(row.result)).length)
const sensitiveCount = computed(() => props.audit.filter((row) => ['sms', 'esim', 'device'].includes(classifyAction(row.action))).length)
const totalPages = computed(() => Math.max(1, Math.ceil(filteredAudit.value.length / pageSize.value)))
const pagedAudit = computed(() => {
  const currentPage = Math.min(page.value, totalPages.value)
  const start = (currentPage - 1) * pageSize.value
  return filteredAudit.value.slice(start, start + pageSize.value)
})

watch([query, actor, result, actionGroup, pageSize], () => {
  page.value = 1
})

watch(totalPages, (value) => {
  if (page.value > value) page.value = value
})

watch([pagedAudit, page], ([rows]) => {
  if (!rows.some((row) => row.id === selectedId.value)) selectedId.value = rows[0]?.id || ''
}, { immediate: true })

function classifyAction(action: string) {
  const value = action.toLowerCase()
  if (value.includes('sms')) return 'sms'
  if (value.includes('esim') || value.includes('profile') || value.includes('subscription')) return 'esim'
  if (value.includes('apprise') || value.includes('routing') || value.includes('notify')) return 'distribution'
  if (value.includes('at_') || value.includes('modem') || value.includes('ping') || value.includes('query_') || value.includes('restart')) return 'device'
  return 'system'
}

function actionGroupLabel(group: string) {
  const labels: Record<string, string> = { sms: '短信', esim: 'eSIM', distribution: '消息分发', device: '终端控制', system: '系统配置' }
  return labels[group] || group
}

function actionLabel(action: string) {
  const labels: Record<string, string> = {
    send_sms: '发送短信', esim_download_profile: '下载 eSIM Profile', esim_enable_profile: '启用 eSIM Profile',
    esim_delete_profile: '删除 eSIM Profile', create_esim_subscription: '新增订阅策略', update_esim_subscription: '更新订阅策略',
    create_apprise_service: '新增 Apprise 服务', update_apprise_service: '更新 Apprise 服务', delete_apprise_service: '删除 Apprise 服务',
    create_apprise_target: '新增通知 Target', update_apprise_target: '更新通知 Target', delete_apprise_target: '删除通知 Target',
    create_routing_rule: '新增路由规则', update_routing_rule: '更新路由规则', delete_routing_rule: '删除路由规则',
    at_command: '执行 AT 指令', modem_hardreset: '模组硬重启', modem_airplane_toggle: '切换飞行模式', ping: '网络 Ping'
  }
  return labels[action] || action.replaceAll('_', ' ')
}

function resultLabel(value: string) {
  const labels: Record<string, string> = { success: '成功', succeeded: '成功', pending: '等待执行', running: '执行中', claimed: '已领取', failed: '失败', error: '错误', timed_out: '超时' }
  return labels[value] || value
}

function formatAuditTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false
  }).format(date)
}

function clearFilters() {
  query.value = ''
  actor.value = 'all'
  result.value = 'all'
  actionGroup.value = 'all'
}

async function copySelected() {
  if (!selectedAudit.value) return
  const row = selectedAudit.value
  await navigator.clipboard.writeText(`时间: ${formatAuditTime(row.createdAt)}\n操作者: ${row.actor}\n终端: ${row.deviceName}\n动作: ${row.action}\n参数: ${row.parameterSummary}\n结果: ${row.result}\n审计 ID: ${row.id}`)
  copied.value = true
  window.setTimeout(() => { copied.value = false }, 1500)
}

function exportAudit() {
  const headers = ['id', 'createdAt', 'actor', 'deviceName', 'action', 'parameterSummary', 'result']
  const rows = filteredAudit.value.map((row) => [row.id, row.createdAt, row.actor, row.deviceName, row.action, row.parameterSummary, row.result])
  const csv = [headers, ...rows].map((row) => row.map((cell) => `"${String(cell).replaceAll('"', '""')}"`).join(',')).join('\n')
  const blob = new Blob(['\ufeff', csv], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = 'sms-hub-audit.csv'
  link.click()
  URL.revokeObjectURL(url)
}
</script>

<template>
  <section class="page audit-page">
    <div class="page-head">
      <div><h1>审计</h1><p>追踪短信、eSIM、消息分发和终端控制等敏感操作。</p></div>
      <button class="btn primary" :disabled="filteredAudit.length === 0" @click="exportAudit">导出当前结果</button>
    </div>

    <div class="grid cols-4 audit-metrics">
      <div class="card metric"><span>审计记录</span><b>{{ audit.length }}</b><small>当前返回结果</small></div>
      <div class="card metric"><span>敏感操作</span><b>{{ sensitiveCount }}</b><small>短信、eSIM 与终端控制</small></div>
      <div class="card metric"><span>等待执行</span><b>{{ pendingCount }}</b><small>pending / running / claimed</small></div>
      <div class="card metric"><span>失败或超时</span><b>{{ failureCount }}</b><small>需要进一步核查</small></div>
    </div>

    <div class="card audit-filter-bar top-gap">
      <input v-model="query" class="field audit-search" placeholder="搜索操作者、终端、动作或参数">
      <select v-model="actionGroup" class="select"><option value="all">全部操作类型</option><option value="sms">短信</option><option value="esim">eSIM</option><option value="distribution">消息分发</option><option value="device">终端控制</option><option value="system">系统配置</option></select>
      <select v-model="actor" class="select"><option value="all">全部操作者</option><option v-for="item in actors" :key="item" :value="item">{{ item }}</option></select>
      <select v-model="result" class="select"><option value="all">全部结果</option><option v-for="item in results" :key="item" :value="item">{{ resultLabel(item) }}</option></select>
      <button class="btn" :disabled="!query && actor === 'all' && result === 'all' && actionGroup === 'all'" @click="clearFilters">清除筛选</button>
      <span class="audit-result-count">{{ filteredAudit.length }} 条</span>
    </div>

    <div class="grid audit-workspace top-gap">
      <div class="card audit-list-panel">
        <div class="audit-table-wrap">
          <table>
            <thead><tr><th>时间</th><th>操作者</th><th>终端</th><th>操作</th><th>结果</th></tr></thead>
            <tbody>
              <tr v-for="row in pagedAudit" :key="row.id" :class="{ selected: selectedAudit?.id === row.id }" @click="selectedId = row.id">
                <td><time>{{ formatAuditTime(row.createdAt) }}</time></td>
                <td>{{ row.actor }}</td>
                <td>{{ row.deviceName || '-' }}</td>
                <td><b>{{ actionLabel(row.action) }}</b><small>{{ actionGroupLabel(classifyAction(row.action)) }}</small></td>
                <td><span :class="['status', statusClass(row.result)]">{{ resultLabel(row.result) }}</span></td>
              </tr>
              <tr v-if="filteredAudit.length === 0"><td colspan="5"><div class="empty audit-empty"><b>没有匹配的审计记录</b><small>调整关键词或筛选条件。</small></div></td></tr>
            </tbody>
          </table>
        </div>
        <PaginationBar v-if="filteredAudit.length" :page="page" :page-size="pageSize" :total="filteredAudit.length" @change="page = $event" @page-size-change="pageSize = $event" />
      </div>

      <aside class="card audit-detail">
        <div class="card-head"><div><b>记录详情</b><small>{{ selectedAudit?.id || '未选择记录' }}</small></div><button class="btn small" :disabled="!selectedAudit" @click="copySelected">{{ copied ? '已复制' : '复制' }}</button></div>
        <template v-if="selectedAudit">
          <dl>
            <dt>完整时间</dt><dd>{{ formatAuditTime(selectedAudit.createdAt) }}</dd>
            <dt>操作者</dt><dd>{{ selectedAudit.actor }}</dd>
            <dt>目标终端</dt><dd>{{ selectedAudit.deviceName || '-' }}</dd>
            <dt>操作类型</dt><dd><span class="audit-action-group">{{ actionGroupLabel(classifyAction(selectedAudit.action)) }}</span></dd>
            <dt>操作</dt><dd><b>{{ actionLabel(selectedAudit.action) }}</b><small class="mono">{{ selectedAudit.action }}</small></dd>
            <dt>执行结果</dt><dd><span :class="['status', statusClass(selectedAudit.result)]">{{ resultLabel(selectedAudit.result) }}</span></dd>
            <dt>参数摘要</dt><dd><pre>{{ selectedAudit.parameterSummary || '-' }}</pre></dd>
            <dt>审计 ID</dt><dd class="mono">{{ selectedAudit.id }}</dd>
          </dl>
        </template>
        <div v-else class="empty audit-empty"><b>未选择记录</b><small>从左侧列表选择一条审计记录查看详情。</small></div>
      </aside>
    </div>
  </section>
</template>
