<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Device, LogEntry } from '../types'
import PaginationBar from '../components/PaginationBar.vue'

const props = defineProps<{
  logs: LogEntry[]
  devices: Device[]
}>()

const query = ref('')
const deviceId = ref('')
const level = ref('all')
const paused = ref(false)
const snapshot = ref<LogEntry[]>([])
const page = ref(1)
const pageSize = ref(10)
const copiedId = ref('')

const sourceLogs = computed(() => paused.value ? snapshot.value : props.logs)
const levels = computed(() => Array.from(new Set(props.logs.map((row) => normalizeLevel(row.level))).values()).sort())
const filteredLogs = computed(() => {
  const keyword = query.value.trim().toLowerCase()
  return sourceLogs.value.filter((row) => {
    const matchesDevice = !deviceId.value || row.deviceId === deviceId.value
    const matchesLevel = level.value === 'all' || normalizeLevel(row.level) === level.value
    const matchesQuery = !keyword || `${row.deviceName} ${row.deviceId} ${row.level} ${row.message}`.toLowerCase().includes(keyword)
    return matchesDevice && matchesLevel && matchesQuery
  })
})
const errorCount = computed(() => props.logs.filter((row) => ['error', 'fatal'].includes(normalizeLevel(row.level))).length)
const warningCount = computed(() => props.logs.filter((row) => normalizeLevel(row.level) === 'warn').length)
const deviceCount = computed(() => new Set(props.logs.map((row) => row.deviceId).filter(Boolean)).size)
const totalPages = computed(() => Math.max(1, Math.ceil(filteredLogs.value.length / pageSize.value)))
const pagedLogs = computed(() => {
  const currentPage = Math.min(page.value, totalPages.value)
  const start = (currentPage - 1) * pageSize.value
  return filteredLogs.value.slice(start, start + pageSize.value)
})

watch([query, deviceId, level, pageSize], () => {
  page.value = 1
})

watch(totalPages, (value) => {
  if (page.value > value) page.value = value
})

watch(paused, (value) => {
  if (value) snapshot.value = [...props.logs]
})

function normalizeLevel(value: string) {
  const normalized = (value || 'info').toLowerCase()
  if (normalized === 'warning') return 'warn'
  if (normalized === 'err') return 'error'
  return normalized
}

function levelLabel(value: string) {
  const labels: Record<string, string> = { debug: '调试', info: '信息', warn: '警告', error: '错误', fatal: '严重' }
  return labels[normalizeLevel(value)] || value || '信息'
}

function formatLogTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false
  }).format(date)
}

async function copyLog(row: LogEntry) {
  await navigator.clipboard.writeText(`${formatLogTime(row.createdAt)} [${row.level}] [${row.deviceName}] ${row.message}`)
  copiedId.value = row.id
  window.setTimeout(() => {
    if (copiedId.value === row.id) copiedId.value = ''
  }, 1500)
}

function exportLogs() {
  const lines = filteredLogs.value.map((row) => `${formatLogTime(row.createdAt)} [${row.level.toUpperCase()}] [${row.deviceName}] ${row.message}`)
  const blob = new Blob([lines.join('\n')], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = 'sms-hub-logs.txt'
  link.click()
  URL.revokeObjectURL(url)
}

function clearFilters() {
  query.value = ''
  deviceId.value = ''
  level.value = 'all'
}
</script>

<template>
  <section class="page logs-page">
    <div class="page-head">
      <div><h1>日志</h1><p>筛选终端日志和服务端事件，定位连接、命令与模组异常。</p></div>
      <div class="toolbar"><button class="btn" @click="paused = !paused">{{ paused ? '继续更新' : '暂停更新' }}</button><button class="btn primary" :disabled="filteredLogs.length === 0" @click="exportLogs">导出当前结果</button></div>
    </div>

    <div class="grid cols-4 logs-metrics">
      <div class="card metric"><span>当前日志</span><b>{{ logs.length }}</b><small>{{ paused ? `已暂停，快照 ${snapshot.length} 条` : '每 2 秒自动更新' }}</small></div>
      <div class="card metric"><span>涉及终端</span><b>{{ deviceCount }}</b><small>当前返回结果</small></div>
      <div class="card metric"><span>警告</span><b>{{ warningCount }}</b><small>warn / warning</small></div>
      <div class="card metric"><span>错误</span><b>{{ errorCount }}</b><small>error / fatal</small></div>
    </div>

    <div class="card logs-workspace top-gap">
      <div class="logs-toolbar">
        <input v-model="query" class="field logs-search" placeholder="搜索日志内容、终端或级别">
        <select v-model="deviceId" class="select"><option value="">全部终端</option><option v-for="device in devices" :key="device.id" :value="device.id">{{ device.name }} / {{ device.phoneNumber || device.deviceId }}</option></select>
        <select v-model="level" class="select"><option value="all">全部级别</option><option v-for="item in levels" :key="item" :value="item">{{ levelLabel(item) }}</option></select>
        <button class="btn" :disabled="!query && !deviceId && level === 'all'" @click="clearFilters">清除筛选</button>
        <span class="logs-result-count">{{ filteredLogs.length }} 条</span>
      </div>

      <div v-if="paused" class="logs-paused-note">日志显示已暂停，新日志仍在后台接收。</div>

      <div v-if="filteredLogs.length" class="logs-list">
        <article v-for="row in pagedLogs" :key="row.id" :class="['log-row', `level-${normalizeLevel(row.level)}`]">
          <div class="log-row-meta">
            <time>{{ formatLogTime(row.createdAt) }}</time>
            <span :class="['log-level', `level-${normalizeLevel(row.level)}`]">{{ levelLabel(row.level) }}</span>
            <span class="log-device" :title="row.deviceId">{{ row.deviceName || row.deviceId || '服务端' }}</span>
          </div>
          <pre>{{ row.message }}</pre>
          <button class="btn small log-copy" type="button" :title="copiedId === row.id ? '已复制' : '复制日志'" @click="copyLog(row)">{{ copiedId === row.id ? '已复制' : '复制' }}</button>
        </article>
      </div>
      <div v-else class="empty logs-empty"><b>没有匹配的日志</b><small>调整关键词、终端或日志级别筛选。</small></div>
      <PaginationBar v-if="filteredLogs.length" :page="page" :page-size="pageSize" :total="filteredLogs.length" @change="page = $event" @page-size-change="pageSize = $event" />
    </div>
  </section>
</template>
