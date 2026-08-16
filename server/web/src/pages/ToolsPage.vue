<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Device, DeviceCommand } from '../types'
import PaginationBar from '../components/PaginationBar.vue'
import { statusClass } from '../utils/ui'

const props = defineProps<{
  devices: Device[]
  commands: DeviceCommand[]
  result: string
}>()

const deviceId = defineModel<string>('deviceId', { required: true })
const atCommand = defineModel<string>('atCommand', { required: true })
const activeCommandId = defineModel<string>('activeCommandId', { required: true })

const pingHost = ref('8.8.8.8')
const commandFilter = ref('all')
const commandPage = ref(1)
const commandPageSize = ref(10)
const copiedId = ref('')

const emit = defineEmits<{
  runCommand: [type: string, payload?: Record<string, unknown>]
  refresh: []
}>()

const selectedDevice = computed(() => props.devices.find((device) => device.id === deviceId.value) || null)
const deviceCommands = computed(() => props.commands
  .filter((command) => command.deviceId === deviceId.value)
  .sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()))
const filteredCommands = computed(() => commandFilter.value === 'all'
  ? deviceCommands.value
  : deviceCommands.value.filter((command) => statusGroup(command.status) === commandFilter.value))
const commandTotalPages = computed(() => Math.max(1, Math.ceil(filteredCommands.value.length / commandPageSize.value)))
const pagedCommands = computed(() => {
  const page = Math.min(commandPage.value, commandTotalPages.value)
  const start = (page - 1) * commandPageSize.value
  return filteredCommands.value.slice(start, start + commandPageSize.value)
})
const runningCount = computed(() => deviceCommands.value.filter((command) => ['pending', 'claimed', 'running'].includes(command.status)).length)
const failureCount = computed(() => deviceCommands.value.filter((command) => ['failed', 'error', 'timed_out'].includes(command.status)).length)
const activeCommand = computed(() => deviceCommands.value.find((command) => command.id === activeCommandId.value) || deviceCommands.value[0] || null)
const activeCommandRunning = computed(() => activeCommand.value && ['pending', 'claimed', 'running'].includes(activeCommand.value.status))
const offline = computed(() => selectedDevice.value?.status !== 'online')

watch(deviceId, () => {
  activeCommandId.value = deviceCommands.value[0]?.id || ''
  commandPage.value = 1
})

watch([commandFilter, commandPageSize], () => {
  commandPage.value = 1
})

watch(commandTotalPages, (totalPages) => {
  if (commandPage.value > totalPages) commandPage.value = totalPages
})

function statusGroup(status: string) {
  if (['succeeded', 'success'].includes(status)) return 'success'
  if (['failed', 'error', 'timed_out'].includes(status)) return 'failed'
  return 'pending'
}

function statusLabel(status: string) {
  const labels: Record<string, string> = { pending: '等待领取', claimed: '执行中', running: '执行中', succeeded: '成功', success: '成功', failed: '失败', error: '错误', timed_out: '超时' }
  return labels[status] || status
}

function commandLabel(type: string) {
  const labels: Record<string, string> = {
    at_command: 'AT 指令', query_signal: '查询信号', query_sim: '查询 SIM', query_network: '查询网络',
    ping: '网络 Ping', modem_airplane_toggle: '切换飞行模式', modem_hardreset: '模组硬重启'
  }
  return labels[type] || type.replaceAll('_', ' ')
}

function formatTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false
  }).format(date)
}

function formatPayload(command: DeviceCommand) {
  if (command.type === 'at_command') return String(command.payload.command || '-')
  if (command.type === 'ping') return String(command.payload.host || '-')
  const entries = Object.entries(command.payload || {})
  return entries.length ? entries.map(([key, value]) => `${key}=${String(value)}`).join(' · ') : '无参数'
}

function run(type: string, payload: Record<string, unknown> = {}) {
  if (!deviceId.value) return
  emit('runCommand', type, payload)
}

function runAT() {
  const command = atCommand.value.trim()
  if (!command) return
  run('at_command', { command })
}

function runPing() {
  const host = pingHost.value.trim()
  if (!host) return
  run('ping', { host })
}

function confirmDanger(type: string, message: string) {
  if (window.confirm(message)) run(type)
}

async function copyCommand(command: DeviceCommand) {
  const content = `${formatTime(command.createdAt)} [${command.status}] ${command.type}\n参数: ${formatPayload(command)}\n结果: ${command.result || '-'}`
  await navigator.clipboard.writeText(content)
  copiedId.value = command.id
  window.setTimeout(() => {
    if (copiedId.value === command.id) copiedId.value = ''
  }, 1500)
}
</script>

<template>
  <section class="page tools-page">
    <div class="page-head">
      <div><h1>诊断工具</h1><p>查询终端状态、执行网络测试和受控的模组命令。</p></div>
      <button class="btn" @click="emit('refresh')">刷新状态</button>
    </div>

    <div class="card tools-device-bar">
      <div class="tools-device-select"><label>目标终端</label><select v-model="deviceId" class="field"><option v-for="device in devices" :key="device.id" :value="device.id">{{ device.name }} / {{ device.phoneNumber || '号码未知' }} / {{ device.status === 'online' ? '在线' : '离线' }}</option></select></div>
      <template v-if="selectedDevice">
        <div class="tools-device-stat"><span>状态</span><b><span :class="['status', statusClass(selectedDevice.status)]">{{ selectedDevice.status === 'online' ? '在线' : '离线' }}</span></b></div>
        <div class="tools-device-stat"><span>当前号码</span><b class="mono">{{ selectedDevice.phoneNumber || '-' }}</b></div>
        <div class="tools-device-stat"><span>运营商</span><b>{{ selectedDevice.operator || '-' }}</b></div>
        <div class="tools-device-stat"><span>Wi-Fi 信号</span><b>{{ selectedDevice.rssi && selectedDevice.rssi < 0 ? `${selectedDevice.rssi} dBm` : '未知' }}</b></div>
        <div class="tools-device-stat"><span>蜂窝信号</span><b>{{ selectedDevice.cellularRssi && selectedDevice.cellularRssi < 0 ? `${selectedDevice.cellularRssi} dBm · CSQ ${selectedDevice.cellularCsq}` : '未知' }}</b></div>
        <div class="tools-device-stat"><span>ICCID</span><b class="mono">{{ selectedDevice.iccid || '-' }}</b></div>
        <div class="tools-device-stat"><span>执行中</span><b>{{ runningCount }}</b></div>
      </template>
    </div>

    <div v-if="offline" class="alert danger top-gap">终端当前离线，新命令将排队等待终端上线领取。</div>

    <section v-if="activeCommand" :class="['card', 'tools-active-result', `result-${statusGroup(activeCommand.status)}`]">
      <div class="tools-active-result-head">
        <div><span>{{ activeCommandRunning ? '本次命令正在执行' : '本次命令结果' }}</span><h2>{{ commandLabel(activeCommand.type) }}</h2><small class="mono">{{ formatPayload(activeCommand) }} · {{ activeCommand.id }}</small></div>
        <span :class="['status', statusClass(activeCommand.status)]">{{ statusLabel(activeCommand.status) }}</span>
      </div>
      <div class="tools-active-result-body">
        <div v-if="activeCommandRunning" class="tools-command-waiting"><span class="tools-pulse"></span><div><b>{{ activeCommand.status === 'pending' ? '等待终端领取命令' : '终端正在执行命令' }}</b><small>页面每 2 秒自动更新，执行结果会直接显示在这里。</small></div></div>
        <pre v-else-if="activeCommand.result">{{ activeCommand.result }}</pre>
        <p v-else>终端已返回状态，但没有附带结果内容。</p>
        <div class="tools-active-result-foot"><time>{{ formatTime(activeCommand.completedAt || activeCommand.createdAt) }}</time><button class="btn small" @click="copyCommand(activeCommand)">{{ copiedId === activeCommand.id ? '已复制' : '复制结果' }}</button></div>
      </div>
    </section>
    <div v-else-if="result" class="alert success top-gap">{{ result }}</div>

    <div class="grid tools-control-grid top-gap">
      <section class="card tools-panel">
        <div class="card-head"><div><b>常用诊断</b><small>只读查询与网络连通性测试</small></div></div>
        <div class="tools-actions-grid">
          <button class="tools-action" type="button" @click="run('at_command', { command: 'ATI' })"><b>模组信息</b><span>执行 ATI，读取厂商与型号</span></button>
          <button class="tools-action" type="button" @click="run('query_signal')"><b>蜂窝信号质量</b><span>通过 AT+CSQ 查询蜂窝 RSSI</span></button>
          <button class="tools-action" type="button" @click="run('query_sim')"><b>SIM 信息</b><span>查询 ICCID、卡状态与号码</span></button>
          <button class="tools-action" type="button" @click="run('query_network')"><b>网络状态</b><span>查询运营商与注册状态</span></button>
        </div>
        <div class="tools-ping"><div><label>Ping 主机</label><input v-model="pingHost" class="field mono" placeholder="8.8.8.8" @keydown.enter="runPing"></div><button class="btn primary" :disabled="!pingHost.trim()" @click="runPing">开始 Ping</button></div>
      </section>

      <section class="card tools-panel">
        <div class="card-head"><div><b>AT 控制台</b><small>直接向模组发送 AT 指令</small></div></div>
        <div class="tools-at-body"><label>AT 指令</label><div class="tools-at-input"><input v-model="atCommand" class="field mono" placeholder="AT+CSQ" @keydown.enter="runAT"><button class="btn primary" :disabled="!deviceId || !atCommand.trim()" @click="runAT">发送</button></div><div class="tools-at-presets"><button class="btn small" @click="atCommand = 'AT+CSQ'">AT+CSQ</button><button class="btn small" @click="atCommand = 'AT+COPS?'">AT+COPS?</button><button class="btn small" @click="atCommand = 'AT+CPIN?'">AT+CPIN?</button><button class="btn small" @click="atCommand = 'AT+CREG?'">AT+CREG?</button></div><p class="tools-safety-note">AT 指令会直接影响模组状态，请确认指令含义后发送。</p></div>
      </section>
    </div>

    <section class="card tools-danger-panel top-gap">
      <div class="card-head"><div><b>模组控制</b><small>可能中断当前网络或正在执行的任务</small></div><span class="status danger">高风险</span></div>
      <div class="tools-danger-actions"><div><b>切换飞行模式</b><span>模组将暂时断开蜂窝网络连接。</span><button class="btn" @click="confirmDanger('modem_airplane_toggle', '确认切换模组飞行模式？当前网络连接会中断。')">切换飞行模式</button></div><div><b>模组硬重启</b><span>模组会断电重启，当前命令和网络连接可能中断。</span><button class="btn danger" @click="confirmDanger('modem_hardreset', '确认硬重启模组？当前连接和正在执行的任务可能中断。')">硬重启</button></div></div>
    </section>

    <section class="card tools-history top-gap">
      <div class="card-head"><div><b>命令历史</b><small>当前终端最近的诊断与控制命令</small></div><div class="toolbar"><span v-if="failureCount" class="status danger">{{ failureCount }} 条失败</span><select v-model="commandFilter" class="select"><option value="all">全部状态</option><option value="pending">执行中</option><option value="success">成功</option><option value="failed">失败</option></select></div></div>
      <div v-if="filteredCommands.length" class="tools-command-list">
        <article v-for="command in pagedCommands" :key="command.id" :class="['tools-command-row', { selected: activeCommand?.id === command.id }]" @click="activeCommandId = command.id">
          <div class="tools-command-meta"><time>{{ formatTime(command.createdAt) }}</time><span :class="['status', statusClass(command.status)]">{{ statusLabel(command.status) }}</span></div>
          <div class="tools-command-main"><b>{{ commandLabel(command.type) }}</b><small class="mono">{{ formatPayload(command) }}</small><pre v-if="command.result">{{ command.result }}</pre></div>
          <button class="btn small" @click.stop="copyCommand(command)">{{ copiedId === command.id ? '已复制' : '复制' }}</button>
        </article>
      </div>
      <div v-else class="empty"><b>暂无匹配的命令记录</b><small>执行诊断命令后，状态和结果会显示在这里。</small></div>
      <PaginationBar v-if="filteredCommands.length" :page="commandPage" :page-size="commandPageSize" :total="filteredCommands.length" @change="commandPage = $event" @page-size-change="commandPageSize = $event" />
    </section>
  </section>
</template>
